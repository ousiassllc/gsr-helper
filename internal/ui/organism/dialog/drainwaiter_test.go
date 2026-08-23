package dialog_test

import (
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
)

// drainWidth / drainHeight はモーダルの中身に配られる領域の目安。
const (
	drainWidth  = 72
	drainHeight = 10
)

// noteLines は必ず描かれる制約の注記（screens.md のドレイン待機中のモック）。
//
// **文言をモックのまま固定する。** ドレイン停止は「今のジョブの完了を待つ」だけで
// あり、待っている間に次のジョブが割り当てられ得る（FR-07）。この 2 行が消えると、
// 利用者は「待てば必ず空く」と誤解して待ち続ける。Issue #5 の受け入れ条件である。
func noteLines() []string {
	return []string{
		"⚠ 待機中も新しいジョブを受け付ける可能性があります",
		"  （GitHub に受付停止の API がないため）",
	}
}

// newDrainWaiter は待機画面を組み立てて動かし始める。
func newDrainWaiter(in dialog.DrainInput) dialog.DrainWaiter {
	d := dialog.NewDrainWaiter(keymap.NewGlobal(), testStyles())
	d.SetSize(drainWidth, drainHeight)
	d.SetInput(in)
	d.Start()

	return d
}

// oneJob は 1 件だけ待っている入力（モックと同じ形）を返す。
func oneJob() dialog.DrainInput {
	return dialog.DrainInput{
		Runner: "build01-1",
		Jobs: []dialog.DrainJob{
			{Repository: "foo/bar", PID: 284193, Elapsed: 11*time.Minute + 2*time.Second},
		},
	}
}

// 制約の注記は状況によらず必ず描かれる（Issue #5 の受け入れ条件）。
//
// ジョブが 0 件のときと、高さが足りず本文が落ちるときも含めて確かめる。**枠の
// 切り詰めは末尾から行を落とすため、注記を末尾に置いたまま任せると真っ先に消える。**
func TestDrainWaiterAlwaysShowsConstraintNote(t *testing.T) {
	many := dialog.DrainInput{Runner: "build01-1", Jobs: []dialog.DrainJob{
		{Repository: "foo/bar", PID: 284193, Elapsed: time.Minute},
		{Repository: "foo/baz", PID: 284194, Elapsed: 2 * time.Minute},
		{Repository: "", PID: 0, Elapsed: 3 * time.Minute},
	}}

	tests := map[string]struct {
		in     dialog.DrainInput
		height int
	}{
		"ジョブ 1 件":     {oneJob(), drainHeight},
		"ジョブ 0 件":     {dialog.DrainInput{Runner: "build01-1", Jobs: nil}, drainHeight},
		"ジョブ 3 件":     {many, drainHeight},
		"高さが注記の分しかない": {many, 2},
		"高さが 1 行":     {many, 1},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			d := newDrainWaiter(tt.in)
			d.SetSize(drainWidth, tt.height)

			view := d.View()

			for _, line := range noteLines() {
				if !strings.Contains(view, line) {
					t.Errorf("制約の注記 %q が描かれていない:\n%s", line, view)
				}
			}
		})
	}
}

// 待機中のジョブは全件並ぶ。Runner.Workers は複数あり得る。
func TestDrainWaiterListsEveryJob(t *testing.T) {
	in := dialog.DrainInput{Runner: "build01-1", Jobs: []dialog.DrainJob{
		{Repository: "foo/bar", PID: 284193, Elapsed: 11*time.Minute + 2*time.Second},
		{Repository: "foo/baz", PID: 284194, Elapsed: 90 * time.Second},
	}}

	view := newDrainWaiter(in).View()

	want := []string{
		"対象ジョブ: foo/bar（Worker PID 284193、開始から 11m02s）",
		"対象ジョブ: foo/baz（Worker PID 284194、開始から 1m30s）",
	}
	for _, line := range want {
		if !strings.Contains(view, line) {
			t.Errorf("%q が描かれていない:\n%s", line, view)
		}
	}
}

// リポジトリ名と PID を取れていないジョブは記号で埋める。空欄のままにしない。
//
// Runner.Worker からリポジトリ名を取れない場合があるためである（screens.md の
// Jobs タブと同じ扱い）。空欄だと「名前の無いジョブ」と読めてしまう。
func TestDrainWaiterFillsUnknownJobFields(t *testing.T) {
	in := dialog.DrainInput{Runner: "build01-1", Jobs: []dialog.DrainJob{
		{Repository: "", PID: 0, Elapsed: 42 * time.Second},
	}}

	view := newDrainWaiter(in).View()

	if want := "対象ジョブ: -（Worker PID -、開始から 42s）"; !strings.Contains(view, want) {
		t.Errorf("%q が描かれていない:\n%s", want, view)
	}
}

// ジョブが 0 件でも壊れない。待機の行と注記だけが残る。
func TestDrainWaiterWithoutJobs(t *testing.T) {
	view := newDrainWaiter(dialog.DrainInput{Runner: "build01-1", Jobs: nil}).View()

	if strings.Contains(view, "対象ジョブ:") {
		t.Errorf("ジョブが無いのに対象ジョブの行が出ている:\n%s", view)
	}
	if !strings.Contains(view, "実行中のジョブの完了を待っています…") {
		t.Errorf("待機の行が描かれていない:\n%s", view)
	}
}

// スピナは動かしている間だけ回る。止めた後もコマを進めると Msg が流れ続ける。
func TestDrainWaiterSpinnerTicksWhileRunning(t *testing.T) {
	d := newDrainWaiter(oneJob())
	first := firstLine(d.View())

	d, _ = d.Update(spinner.TickMsg{Time: time.Now(), ID: 0})
	spun := firstLine(d.View())

	if spun == first {
		t.Errorf("Tick でスピナのコマが進んでいない（%q のまま）", first)
	}

	d.Stop()
	d, _ = d.Update(spinner.TickMsg{Time: time.Now(), ID: 0})

	if got := firstLine(d.View()); got != spun {
		t.Errorf("停止後にスピナが進んだ（%q → %q）", spun, got)
	}
}

// esc は待機のキャンセル。画面自身は閉じず、決定を Msg で返す。
func TestDrainWaiterCancel(t *testing.T) {
	_, cmd := newDrainWaiter(oneJob()).Update(press("esc"))

	if cmd == nil {
		t.Fatal("DrainCanceledMsg が発行されていない")
	}
	if _, ok := cmd().(dialog.DrainCanceledMsg); !ok {
		t.Fatalf("DrainCanceledMsg 以外の Msg が返った（%T）", cmd())
	}
}

// キャンセル以外のキーでは何も返さない。
func TestDrainWaiterIgnoresOtherKeys(t *testing.T) {
	for _, k := range []string{"j", "y", "enter", "q"} {
		t.Run(k, func(t *testing.T) {
			_, cmd := newDrainWaiter(oneJob()).Update(press(k))

			if cmd != nil {
				t.Errorf("%q で Msg が発行された（%T）", k, cmd())
			}
		})
	}
}

// 幅を超える行を出さない。長いリポジトリ名は中略する。
func TestDrainWaiterFitsWidth(t *testing.T) {
	in := dialog.DrainInput{Runner: "build01-1", Jobs: []dialog.DrainJob{
		{Repository: strings.Repeat("very-long-org/very-long-repo", 5), PID: 284193, Elapsed: time.Minute},
	}}

	wantNoWideLine(t, newDrainWaiter(in).View(), drainWidth)
}

// 見出しは枠（template.Modal）へ渡すために取り出せる。runner 名が無くても壊れない。
func TestDrainWaiterTitle(t *testing.T) {
	tests := map[string]struct {
		runner string
		want   string
	}{
		"runner 名あり": {"build01-1", "ドレイン停止中   build01-1"},
		"runner 名なし": {"", "ドレイン停止中"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			d := newDrainWaiter(dialog.DrainInput{Runner: tt.runner, Jobs: nil})

			if got := d.Title(); got != tt.want {
				t.Errorf("Title() = %q, want %q", got, tt.want)
			}
		})
	}
}

// フッタのキーヒントは待機のキャンセルだけ。本文には出さない。
func TestDrainWaiterHints(t *testing.T) {
	d := newDrainWaiter(oneJob())

	hints := d.Hints()
	if len(hints) != 1 {
		t.Fatalf("キーヒントの数 = %d, want 1（%v）", len(hints), hints)
	}
	if hints[0].Key != "esc" || hints[0].Desc != "待機をキャンセル" {
		t.Errorf("hints[0] = %q:%q, want %q:%q", hints[0].Key, hints[0].Desc, "esc", "待機をキャンセル")
	}
	if strings.Contains(d.View(), "待機をキャンセル") {
		t.Errorf("キーヒントが本文にも出ている:\n%s", d.View())
	}
}

// 配色とキー定義を差し替えても対象と計時は保つ。3 秒ごとの配り直しで作り直さない。
func TestDrainWaiterRestyleKeepsInput(t *testing.T) {
	d := newDrainWaiter(oneJob())

	d.Restyle(keymap.NewGlobal(), testStyles())

	if !strings.Contains(d.View(), "foo/bar") {
		t.Errorf("Restyle で対象が消えた:\n%s", d.View())
	}

	_, cmd := d.Update(press("esc"))
	if cmd == nil {
		t.Fatal("Restyle 後に esc が効かない")
	}
	if _, ok := cmd().(dialog.DrainCanceledMsg); !ok {
		t.Fatalf("DrainCanceledMsg 以外の Msg が返った（%T）", cmd())
	}
}

// firstLine は 1 行目を返す。
func firstLine(view string) string {
	return strings.Split(view, "\n")[0]
}
