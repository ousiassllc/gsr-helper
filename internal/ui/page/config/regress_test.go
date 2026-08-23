package config

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/config/apply"
	"github.com/ousiassllc/gsr-helper/internal/config/edit"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 承認は 1 度しか受けないこと（y の連打で 2 回書き込まない）。
//
// **2 回目の書き込みは FR-38 のバックアップを壊す。** 退避をやり直すと、
// 既に新しい内容になった .env が .bak へ写り、元の内容が失われる。
func TestApprovalIsAcceptedOnce(t *testing.T) {
	t.Parallel()

	const before = "PATH=/usr/bin\n"
	r := withTempDir(t, "build01-1", before)
	m := newPage(t, r)
	m, _ = send(t, m, page.EditConfigMsg{Runner: r})
	m = openEnvForm(t, m)

	m, cmd := send(t, m, page.ResultMsg{Kind: diffKind, Msg: dialog.DecidedMsg{Confirmed: true}})
	if _, ok := doneOf(cmd); !ok {
		t.Fatal("1 回目の書き込みが走っていない")
	}

	// 2 回目の決定（連打・貼り付け）は捨てる。
	_, again := send(t, m, page.ResultMsg{Kind: diffKind, Msg: dialog.DecidedMsg{Confirmed: true}})
	if _, ok := doneOf(again); ok {
		t.Error("同じ承認で 2 回書き込んだ")
	}
	if bak, err := readFile(r.Dir + "/.env.bak"); err != nil || bak != before {
		t.Errorf("バックアップ = %q, %v, want 元の内容 %q", bak, err, before)
	}
}

// 取得の最中に対象を切り替えたら、届いた結果を捨てること。
//
// 捨てないと A から取ったラベルを B のフォームへ入れ、そのまま確定すれば
// B のラベルが A の値で置き換わる。
func TestStaleFetchIsDropped(t *testing.T) {
	t.Parallel()

	a := withTempDir(t, "build01-1", "")
	b := withTempDir(t, "build01-2", "")
	m := newPage(t, a, b)
	m, _ = send(t, m, page.EditConfigMsg{Runner: a})

	// A の取得が飛んでいる間に B へ切り替える。
	m, _ = send(t, m, page.EditConfigMsg{Runner: b})
	m, _ = send(t, m, loadedMsg{
		kind: edit.KindLabels, dir: a.Dir, labels: []string{"gpu"}, groups: nil, err: nil,
	})

	if m.overlay.Active() || m.formShown {
		t.Errorf("切り替え後の対象にフォームが開いた:\n%s", view(m))
	}
	if m.vals.Labels != "" {
		t.Errorf("別の runner の値が入った: %q", m.vals.Labels)
	}
}

// 複製の反映は複製元ではなく複製先を再起動すること（FR-40）。
func TestCopyAppliesToTargetsNotSource(t *testing.T) {
	t.Parallel()

	src := withTempDir(t, "build01-1", "PATH=/usr/bin\n")
	dst := withTempDir(t, "build01-2", "PATH=/old\n")
	m := newPage(t, src, dst)
	m, _ = send(t, m, page.EditConfigMsg{Runner: src})

	m.vals.Kind = edit.KindCopy
	m.vals.CopyTo = []string{dst.Name()}
	m, _ = send(t, m, page.ResultMsg{Kind: formKind, Msg: dialog.FormDoneMsg{Form: nil}})
	m, _ = send(t, m, page.ResultMsg{Kind: diffKind, Msg: dialog.DecidedMsg{Confirmed: true}})
	m, _ = send(t, m, doneMsg{text: "書き込みました", err: nil})

	_, cmd := send(t, m, page.ResultMsg{
		Kind: applyKind, Msg: organism.ChosenMsg{ID: apply.Force.Label(), Key: ""},
	})
	if _, ok := doneOf(cmd); !ok {
		t.Fatal("反映が走っていない")
	}

	fake, ok := m.st.Exec.(*exec.Fake)
	if !ok {
		t.Fatalf("Fake 以外の Executor: %T", m.st.Exec)
	}
	var got string
	for _, c := range fake.Calls() {
		got += c.Name + " " + strings.Join(c.Args, " ") + "\n"
	}
	if !strings.Contains(got, dst.UnitName) {
		t.Errorf("複製先が再起動されていない:\n%s", got)
	}
	if strings.Contains(got, src.UnitName) {
		t.Errorf("複製元を再起動している:\n%s", got)
	}
}

// 現在のラベルと同じ内容で確定したら書き込まないこと。
func TestUnchangedLabelsAreNotWritten(t *testing.T) {
	t.Parallel()

	r := withTempDir(t, "build01-1", "")
	m := newPage(t, r)
	m, _ = send(t, m, page.EditConfigMsg{Runner: r})

	// 取得した現在値（予約ラベルを含む全量）でフォームを開く。
	m, cmd := send(t, m, loadedMsg{
		kind: edit.KindLabels, dir: r.Dir,
		labels: []string{"self-hosted", "Linux", "X64", "gpu"}, groups: nil, err: nil,
	})
	pagetest.RunAll(cmd)

	if m.vals.Labels != "gpu" {
		t.Fatalf("フォームの初期値 = %q, want gpu（予約ラベルは除く）", m.vals.Labels)
	}

	m, _ = send(t, m, page.ResultMsg{Kind: formKind, Msg: dialog.FormDoneMsg{Form: nil}})
	if !strings.Contains(m.status(), "変更はありません") {
		t.Errorf("状態行 = %q, want 変更はありません", m.status())
	}
}

// 自身の設定を保存したあと開き直すと、保存した値が初期値になること（FR-42）。
func TestSelfConfigReopensWithSavedValues(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	st := pagetest.State(80, 24)
	st.Config = page.ConfigDeps{Conf: st.Config.Conf, Path: dir + "/config.yaml", FirstRun: false}
	m := New(tabIndex, st)
	next, _ := m.Update(st)
	m = next.(Model)

	m, _ = send(t, m, organism.ChosenMsg{ID: selfID, Key: ""})
	m.vals.Self.Refresh = "42"
	m, _ = send(t, m, page.ResultMsg{Kind: formKind, Msg: dialog.FormDoneMsg{Form: nil}})

	m, cmd := send(t, m, page.ResultMsg{Kind: diffKind, Msg: dialog.DecidedMsg{Confirmed: true}})
	done, ok := doneOf(cmd)
	if !ok || done.err != nil {
		t.Fatalf("設定の書き込みに失敗: %v", done.err)
	}
	m, _ = send(t, m, done)

	m, _ = send(t, m, organism.ChosenMsg{ID: selfID, Key: ""})
	if m.vals.Self.Refresh != "42" {
		t.Errorf("開き直した初期値 = %q, want 42（保存した値）", m.vals.Self.Refresh)
	}
}

// 反映方法の esc が「反映しない」と同じ意味になること。
//
// 単に閉じると apply.Run を通らず、drop-in に要る daemon-reload まで飛ぶ。
func TestApplyModalBackChoosesNone(t *testing.T) {
	t.Parallel()

	r := withTempDir(t, "build01-1", "PATH=/usr/bin\n")
	m := newPage(t, r)
	m, _ = send(t, m, page.EditConfigMsg{Runner: r})
	m = openEnvForm(t, m)
	m, _ = send(t, m, page.ResultMsg{Kind: diffKind, Msg: dialog.DecidedMsg{Confirmed: true}})
	m, cmd := send(t, m, doneMsg{text: "書き込みました", err: nil})
	m = pagetest.Advance(m, cmd, 4).(Model)

	if !m.overlay.Active() {
		t.Fatal("反映方法の選択が開いていない")
	}
	m, cmd = send(t, m, pagetest.Press("esc"))
	m = pagetest.Advance(m, cmd, 4).(Model)

	if m.overlay.Active() {
		t.Errorf("esc で閉じていない:\n%s", view(m))
	}
	if !strings.Contains(m.status(), "反映しました") {
		t.Errorf("状態行 = %q, want 反映（apply.None）を通った旨", m.status())
	}
}

// 絞り込みの入力中はキーを親へ差し戻さないこと。
//
// 差し戻すと、絞り込みに打った q でアプリが終わり、数字でタブが切り替わる。
func TestFilteringSwallowsGlobalKeys(t *testing.T) {
	t.Parallel()

	r := withTempDir(t, "build01-1", "")
	m := newPage(t, r)
	m, _ = send(t, m, page.EditConfigMsg{Runner: r})

	m, _ = send(t, m, pagetest.Press("/"))
	if !m.filtering() {
		t.Fatal("絞り込みが始まっていない")
	}

	_, cmd := send(t, m, pagetest.Press("q"))
	for _, msg := range pagetest.Msgs(cmd) {
		if _, ok := msg.(page.GlobalKeyMsg); ok {
			t.Error("絞り込み中の打鍵が親へ差し戻された")
		}
	}
	if got := chromeOf(t, m.chrome()).Input; got != "絞り込み" {
		t.Errorf("状態行の入力中 = %q, want 絞り込み", got)
	}
}

// systemd ユニットが無い runner では drop-in の行を選べず、理由が備考に出ること。
func TestDropInRowDisabledWithoutUnit(t *testing.T) {
	t.Parallel()

	r := withTempDir(t, "build01-1", "")
	r.UnitName = ""
	m := newPage(t, r)
	m, _ = send(t, m, page.EditConfigMsg{Runner: r})

	if !contains(m, "systemd ユニット無し") {
		t.Errorf("選べない理由が備考に出ていない:\n%s", view(m))
	}
}
