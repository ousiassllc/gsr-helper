package disk

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/disk"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest/cmdtest"
)

// 検証は内部テスト（package disk）で行う。集計の Msg（usageMsg）と実行中の状態
// （scan / clean / plan）は非公開であり、外から組み立てられない。**外から見える
// 振る舞いだけで確かめられるものは振る舞いで確かめる**（削除が走ったかは
// exec.Fake の記録で見る）が、世代の突き合わせのように「古い結果を捨てた」ことを
// 表に出さない設計の部分は、注入して確かめるほかない。

// tabIndex は Disk タブのタブ番号（tabset の並びの添字。画面の [3] と 1 ずれる）。
const tabIndex = 2

// dfJSON は docker system df の応答。**prune -f が回収する種別（Containers /
// Build Cache）と回収しない種別（Images / Local Volumes）の両方を含める。** 回収する
// 種別だけを返すと、解放見込みが disk.PruneReclaimable でも「全 Reclaimable の素朴な
// 合計」でも同じ値になり、取り違えを検出できない。容量は docker の 10 進接頭辞で書く。
const dfJSON = `{"Type":"Images","Size":"9GB","Reclaimable":"8GB (88%)"}
{"Type":"Containers","Size":"300MB","Reclaimable":"300MB (100%)"}
{"Type":"Local Volumes","Size":"2GB","Reclaimable":"2GB (100%)"}
{"Type":"Build Cache","Size":"12GB","Reclaimable":"12GB (100%)"}`

// 行に出る文言の照合には**列幅に収まる長さの文字列**を使う（TARGET は 25 セル、
// PATH は最終列なので実効 24 セル。token.DiskColumns）。収まる文言はリテラルの全文で
// 照合する。被テスト側の定数を参照すると、文言が列幅を超えて伸びても照合が追随し、
// 中略に気付けない。
const (
	// dockerCacheLabel は dfJSON から作られる行の表示名（internal/disk の訳語）。
	dockerCacheLabel = "docker / ビルドキャッシュ"
	// dockerSkipReasonText は docker が使えないときの理由。
	dockerSkipReasonText = "docker が無く集計不可"
	// busyReasonPrefix はジョブ実行中で選べない理由（FR-31）。
	busyReasonPrefix = "ジョブ実行中で削除不可"
)

func press(k string) tea.KeyPressMsg { return pagetest.Press(k) }

// baseState は既定の共有状態と、そこに載せた Executor を返す。
//
// **runner 0 台・docker 無しを既定にする。** 集計を待つ Cmd はチャネルが閉じるまで
// 返らないため、集計対象がある状態を既定にすると、束の中の Cmd をすべて実行する
// 検証（pagetest.ScanKey / pump）がホストの走査を待つことになる。
func baseState() (page.StateMsg, *exec.Fake) {
	st := pagetest.State(80, 16)
	st.Caps.Docker = false
	fake := exec.NewFake()
	st.Deps.Exec = fake
	return st, fake
}

// dockerState は docker が使える共有状態を返す。docker system df は dfJSON を返す。
func dockerState() (page.StateMsg, *exec.Fake) {
	st, fake := baseState()
	st.Caps.Docker = true
	fake.SetFunc(func(_ string, args []string) (exec.Result, error) {
		if len(args) >= 2 && args[1] == "df" {
			return exec.Result{Stdout: []byte(dfJSON), Stderr: nil, ExitCode: 0}, nil
		}
		return exec.Result{Stdout: nil, Stderr: nil, ExitCode: 0}, nil
	})
	return st, fake
}

// busyRunner は実体のあるディレクトリを持つジョブ実行中の runner を返す
// （_work/bar に 1 ファイル）。
//
// pagetest の runner は実在しないパスを指すので、集計すると対象 0 件になる。
func busyRunner(t *testing.T) runner.Runner {
	t.Helper()

	dir := t.TempDir()
	work := filepath.Join(dir, "_work", "bar")
	if err := os.MkdirAll(work, 0o750); err != nil {
		t.Fatalf("作業ディレクトリを作れない: %v", err)
	}
	if err := os.WriteFile(filepath.Join(work, "a.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatalf("ファイルを作れない: %v", err)
	}

	r := pagetest.BusyRunner()
	r.Dir = dir
	r.WorkDir = filepath.Join(dir, "_work")
	return r
}

// newModel は共有状態を配った Disk タブを返す。
func newModel(t *testing.T, st page.StateMsg) Model {
	t.Helper()

	m, _ := send(t, New(tabIndex, st), st)
	return m
}

// activate は前面に出たことを知らせ、集計が終わるまで結果を配り直す。
func activate(t *testing.T, m Model) Model {
	t.Helper()

	next, _ := send(t, m, page.ActivateMsg{})
	return next
}

// send は Msg を 1 つ渡し、そのとき発行された ChromeMsg と、Cmd が返す Msg まで
// 反映した Model を返す。
func send(t *testing.T, m Model, msg tea.Msg) (Model, page.ChromeMsg) {
	t.Helper()

	next, cmd := m.Update(msg)
	c, ok := findChrome(cmd)
	if !ok {
		t.Fatal("ChromeMsg が発行されていない")
	}
	return pump(t, as(t, next), cmd), c
}

// update は Msg を 1 つ渡すだけで、返った Cmd は実行しない。
func update(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()

	next, _ := m.Update(msg)
	return as(t, next)
}

// pump は Cmd が返した page.TabMsg の中身をタブへ配り直す。
//
// 親 Model が「ドメイン呼び出しの結果を発行元のタブへ戻す」振る舞い（page.TabMsg の
// doc）を最小限に写したものである。写さないと、集計も削除も 1 段目の Cmd で止まり、
// 判明順の反映や確認の往復を検証できない。
func pump(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()

	for _, msg := range cmdtest.MustMsgs(cmd, cmdtest.CmdTimeout) {
		tm, ok := msg.(page.TabMsg)
		if !ok {
			continue
		}
		next, c := m.Update(tm.Msg)
		m = pump(t, as(t, next), c)
	}
	return m
}

// as は tea.Model を Disk タブへ戻す。
func as(t *testing.T, m tea.Model) Model {
	t.Helper()

	got, ok := m.(Model)
	if !ok {
		t.Fatalf("Update が %T を返した, want disk.Model", m)
	}
	return got
}

// findChrome は Cmd を辿って最初の ChromeMsg を返す。
//
// 見つかった時点で打ち切るのは、集計や絞り込みの Cmd を走らせないためである
// （page は ChromeMsg を束の先頭に置いている）。
func findChrome(cmd tea.Cmd) (page.ChromeMsg, bool) {
	if cmd == nil {
		return page.ChromeMsg{}, false
	}
	switch msg := cmd().(type) {
	case page.ChromeMsg:
		return msg, true
	case tea.BatchMsg:
		for _, c := range msg {
			if v, ok := findChrome(c); ok {
				return v, true
			}
		}
	}
	return page.ChromeMsg{}, false
}

// pruneCall は docker system prune の呼び出しを返す。
func pruneCall(f *exec.Fake) (exec.Call, bool) {
	for _, c := range f.Calls() {
		if c.Name == "docker" && len(c.Args) >= 2 && c.Args[0] == "system" && c.Args[1] == "prune" {
			return c, true
		}
	}
	return exec.Call{}, false
}

// fakeUsage は集計 1 件を作る。
func fakeUsage(label string, bytes int64) disk.Usage {
	return disk.Usage{
		Kind: disk.KindWork, Runner: "build01-1", Base: "/opt/runners/build01-1",
		Path: "/opt/runners/build01-1/" + label, Label: label,
		Bytes: bytes, Files: 3, Removable: true, Reason: "", Err: nil,
	}
}
