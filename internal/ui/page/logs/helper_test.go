package logs

import (
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest/cmdtest"
)

// テストの道具を集める。共有状態の組み立ては pagetest から取り、ここでは Logs タブに
// 固有のもの（`_diag` を持つ runner、購読を辿る Cmd の実行）だけを持つ。

// testTab は検証で使うタブ番号（screens.md の [4]Logs は添字 3）。
const testTab = 3

// waitTimeout は購読の待ち受け（wait）を諦めるまでの上限。
//
// 購読の待ち受けは行が届くまで戻らない。**戻らないこと自体は正しい**ので、諦めて
// 次へ進むための時間である。行を待つ検証は pumpUntil の条件で止める。
//
// **必ず戻るはずの Cmd をこれで待たないこと。** 諦めるまでの時間がそのまま
// テストの所要時間になるので短くしてあり、`make check`（-race で全パッケージを
// 同時に実行）の負荷が掛かると、正しく戻る Cmd でも間に合わずに落ちうる。
// そちらは cmdtest.CmdTimeout（30 秒。速く戻る Cmd の速さには効かない）を使う
// ——1 つの定数を両方に使っていたころ、config 側で同じ形の散発的な失敗が出た
// （Issue #140 / #145）。
const waitTimeout = 2 * time.Second

// drainTimeout は畳んだ購読のチャネルが閉じるのを待つ上限。
//
// **これも「必ず終わるはずの待ち」だが、cmdtest.CmdTimeout（30 秒）ほど倒さない。**
// 畳み漏れの退行が入ると track の後始末はほぼ全テストに掛かるため、諦めるまでの時間が
// そのままパッケージの所要時間になる。負荷に耐えるだけの余裕（2 秒の 2.5 倍）を持たせ、
// 失敗したときの待ち時間は分単位にしない、という間を取った値である。
const drainTimeout = 5 * time.Second

// writeLog は runner の `_diag` にログを 1 つ作る（組み立ては pagetest.WriteDiagLog に任せる）。
func writeLog(t *testing.T, r runner.Runner, name, body string, mod time.Time) string {
	t.Helper()

	path, err := pagetest.WriteDiagLog(r.Dir, name, body, mod)
	if err != nil {
		t.Fatalf("ログを用意できない: %v", err)
	}
	return path
}

// newTab は最初の共有状態を配り終えた Logs タブを返す。
//
// New の直後ではなく StateMsg を 1 度渡した状態から始めるのは、親が必ずそうする
// ためである（登録の Cmd はここで流れる。runners.go の Init の doc）。
func newTab(t *testing.T, st page.StateMsg) Model {
	t.Helper()

	m, _ := step(t, New(testTab, st), st)
	return m
}

// step は Msg を 1 つ渡し、Model と Cmd を返す。渡した先の Model は track で覚える。
func step(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()

	next, cmd := m.Update(msg)
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update が %T を返した, want logs.Model", next)
	}
	track(t, got)
	return got, cmd
}

// tracked はテストごとに「step が最後に見た Model」を覚える登録簿。
//
// 購読はテストの途中で何度も張り直され（open / 前面への復帰）、そのたびに別の変数の
// Model へ移る。**後始末で畳むべき 1 本を指せるのはここだけである。** `defer` へ Model を
// 渡す形では引数が登録時点で評価され、既に畳んだ購読を畳み直すだけになり、張り直した
// ぶんが残る。テストが並列に走っても壊れないよう sync.Map で持つ。
var tracked sync.Map // *testing.T -> *Model

// track は最新の Model を覚え、テストごとに 1 度だけ後始末を登録する。
//
// step を通せば必ず畳まれる形にしてあるので、購読を張るテストは後始末を書かなくてよい。
// 畳んだあと goroutine が実際に戻るまで待つのは、待たずに抜けると Tail と fsnotify の
// 監視がテストの外へ残り、消えた一時ディレクトリを見続けるためである
// （internal/logs の startTail と同じ作法）。
func track(t *testing.T, m Model) {
	t.Helper()

	v, loaded := tracked.LoadOrStore(t, &m)
	last, _ := v.(*Model)
	if loaded {
		*last = m
		return
	}
	t.Cleanup(func() {
		tracked.Delete(t)
		lines := last.stream.lines
		last.stop()
		if lines != nil && !cmdtest.Drained(lines, drainTimeout) {
			t.Error("テストの終わりに購読が畳まれていない")
		}
	})
}

// pumpUntil は Cmd を辿って Model を進め、cond が満たされた時点で止める
// （辿り方は cmdtest.Pump に任せ、ここは Model の進め方と合否の判定だけを渡す）。
func pumpUntil(t *testing.T, m Model, cmd tea.Cmd, cond func(Model) bool) Model {
	t.Helper()

	got, err := cmdtest.Pump(m, testTab, cmd, waitTimeout,
		func(m Model, msg tea.Msg) (Model, tea.Cmd) { return step(t, m, msg) }, cond)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// chromeOf は Cmd に含まれる ChromeMsg を返す。無ければテストを止める。
func chromeOf(t *testing.T, cmd tea.Cmd) page.ChromeMsg {
	t.Helper()

	msg, ok := cmdtest.ChromeOf(cmd)
	if !ok {
		t.Fatal("ChromeMsg が返っていない")
	}
	return msg
}
