package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/config/edit"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/configmodal"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// tabIndex は Config タブの番号（[6]）。
const tabIndex = 5

// newPage は共有状態を配った Config タブを返す。
func newPage(t *testing.T, rs ...runner.Runner) Model {
	t.Helper()

	st := pagetest.State(80, 24, rs...)
	m := New(tabIndex, st)

	next, _ := m.Update(st)
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Model 以外が返った: %T", next)
	}
	return got
}

// withTempDir は一時ディレクトリに実体を持つ runner を返す。
func withTempDir(t *testing.T, name, env string) runner.Runner {
	t.Helper()

	dir := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("runner ディレクトリの作成に失敗: %v", err)
	}
	if env != "" {
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o600); err != nil {
			t.Fatalf(".env の作成に失敗: %v", err)
		}
	}

	return runner.Runner{
		Dir:      dir,
		Config:   runner.Config{AgentName: name},
		Scope:    scope.Scope{Kind: scope.Org, Owner: "foo"},
		UnitName: "actions.runner.foo." + name + ".service",
	}
}

// send は Msg を配り、Model を返す。
func send(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()

	next, cmd := m.Update(msg)
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Model 以外が返った: %T", next)
	}
	return got, cmd
}

// view は本文（モーダルがあればモーダル）を文字列で返す。
func view(m Model) string { return m.View().Content }

// contains は本文に文字列が含まれるかを返す。
func contains(m Model, s string) bool { return strings.Contains(view(m), s) }

// chromeOf は直近の Cmd から ChromeMsg を取り出す。
func chromeOf(t *testing.T, cmd tea.Cmd) page.ChromeMsg {
	t.Helper()

	got, ok := pagetest.ChromeOf(cmd)
	if !ok {
		t.Fatal("ChromeMsg が発行されていない")
	}
	return got
}

// readFile は path の内容を返す。
func readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	return string(b), err
}

// openEnvForm は .env のフォームに変更を入れて確定したところまで進める。
//
// huh のフォームを打鍵で埋める代わりに入力先へ直接書くのは、ここで見たいのが
// 「確定してから書き込むまで」の経路だからである。フォームそのものの動きは
// organism/dialog のテストが持つ。
func openEnvForm(t *testing.T, m Model) Model {
	t.Helper()

	m.vals.Kind = edit.KindEnv
	for i, spec := range edit.EnvKeys {
		if spec.Key == "PATH" {
			m.vals.Env[i], m.vals.EnvBefore[i] = "/opt/bin", "/usr/bin"
		}
	}

	m, _ = send(t, m, page.ResultMsg{Kind: configmodal.FormKind, Msg: dialog.FormDoneMsg{Form: nil}})
	if !m.overlay.Active() {
		t.Fatal("差分の承認が開いていない")
	}
	return m
}

// fileAsDir は「ディレクトリとしては使えないパス」を返す。
//
// 書き込みの失敗を作るために使う。通常のファイルを 1 つ置き、その下へ書かせると
// 親ディレクトリの作成が ENOTDIR で失敗する。権限に依らないので root でも再現する。
func fileAsDir(t *testing.T) string {
	t.Helper()

	path := t.TempDir() + "/notadir"
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("前提のファイルを作れない: %v", err)
	}
	return path
}

// errNotFound は束を最後まで辿っても目当ての Msg が無かったことを表す。
//
// ErrCmdTimeout と分けているのは、**呼び出し側の読み方が正反対だから**である。
// こちらは「Cmd が本当に出ていない」＝画面側の判断（差分が無いなど）を疑う合図で、
// 待ち時間切れは「出ているかどうか分からない」＝機械の混み具合を疑う合図になる
// （Issue #140）。
var errNotFound = errors.New("目当ての Msg が束に無い")

// savedOf は Cmd の束から親宛ての ConfigSavedMsg を取り出す（Issue #128）。
//
// doneOf と違って TabMsg を剥がさない。宛先は発行元のタブではなく親であり、
// page.Do で包まないことがこの Msg の要件だからである（page.ConfigSaved の doc）。
func savedOf(cmd tea.Cmd) (page.ConfigSavedMsg, error) {
	msg, err := findMsg(cmd, pagetest.CmdTimeout, func(m tea.Msg) bool {
		_, is := m.(page.ConfigSavedMsg)
		return is
	})
	if err != nil {
		return page.ConfigSavedMsg{}, err
	}
	return msg.(page.ConfigSavedMsg), nil
}

// doneOf は Cmd の束から書き込み・反映の結果を取り出す。
//
// 束（tea.Batch）で返るのは chrome の更新と処理本体が同時に流れるためで、
// 本体だけを取り出して結果を見る。
func doneOf(cmd tea.Cmd) (doneMsg, error) {
	msg, err := findMsg(cmd, pagetest.CmdTimeout, func(m tea.Msg) bool {
		if tab, wrapped := m.(page.TabMsg); wrapped {
			m = tab.Msg
		}
		_, is := m.(doneMsg)
		return is
	})
	if err != nil {
		return doneMsg{}, err
	}
	if tab, wrapped := msg.(page.TabMsg); wrapped {
		msg = tab.Msg
	}
	return msg.(doneMsg), nil
}

// findMsg は束の Cmd を順に走らせ、want を満たす最初の Msg を返す。
//
// **待ち時間切れで打ち切らない。** 束には戻らない Cmd が混じりうるので、1 本
// 諦めても残りを辿る。目当てが最後まで見つからなかったときだけ、諦めた本数が
// あれば ErrCmdTimeout を、無ければ errNotFound を返す——「見つからなかった」の
// 理由をここで確定させないと、呼び出し側の失敗メッセージが取り違える。
//
// **束の展開にも締め切りを掛ける。** pagetest.Expand は先頭の Cmd を締め切り
// 無しで走らせるので、束になっていない戻らない Cmd を渡すとそこで止まる。
// 待ち時間切れを区別して返すのに、区別する前に止まっては意味がない（Issue #140）。
//
// **戻らない Cmd を含む束には向かない。** 目当てが無いときは束を実行しきるので、
// 購読の待ち受けを持つタブでは諦めるだけで CmdTimeout ぶん掛かる（Config タブの
// Cmd は購読を持たない）。そちらは短い締め切りを決めて pagetest.Pump を使うこと。
func findMsg(cmd tea.Cmd, timeout time.Duration, want func(tea.Msg) bool) (tea.Msg, error) {
	gaveUp := 0
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]

		msg, err := pagetest.RunCmd(c, timeout)
		if errors.Is(err, pagetest.ErrCmdTimeout) {
			gaveUp++

			continue
		}
		if err != nil {
			continue
		}
		if inner, isBundle := pagetest.Cmds(msg); isBundle {
			queue = append(queue, inner...)

			continue
		}
		if want(msg) {
			return msg, nil
		}
	}
	if gaveUp > 0 {
		return nil, fmt.Errorf("%w（%d 本が %s 以内に戻らなかった）", pagetest.ErrCmdTimeout, gaveUp, timeout)
	}
	return nil, errNotFound
}
