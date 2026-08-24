package cmdtest

import (
	"errors"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// Cmd を実際に走らせて結果を取り出す道具を集める。
//
// こちらが持つのは走らせて待つ仕組みそのもの（RunCmd と、それを束へ広げる bundle）で
// あり、cmd.go はその上で束をどう平坦化するかを持つ側である。諦める判断は RunCmd の
// 1 箇所に集めてあるので、cmd.go は RunCmd を通すだけで自前の締め切りを持たない。
// 走らせた結果から目当ての 1 件を拾う側は find.go にある（そちらの doc）。
//
// **諦める判断は RunCmd に集める。** このパッケージが Cmd を走らせる道具は例外なく
// RunCmd を通す——締め切りの掛かっていない実行が 1 本でも残っていると、戻らない Cmd を
// 渡したテストが失敗ではなくハングになり、CI では `go test` の既定のタイムアウト
// （10 分）ぶんの原因の分からない停止に見える（Issue #145 / #150）。素で実行して
// いた RunAll / Msgs / ChromeOf と pagetest の IsQuit は Issue #150 で通し終えた。
//
// **`testing` を import しない。** このパッケージは通常のパッケージなので（pagetest.go の
// doc）、import するとテスト用のフラグが本番のバイナリ側の依存に現れる。合否の判定は
// 呼び出し側の _test.go に残し、ここは結果と成否だけを返す。

// CmdTimeout は「必ず戻るはずの Cmd」を待つ上限。
//
// **戻りの速い Cmd の速さには効かない。** 締め切りが効くのは戻らない Cmd を諦める
// までの時間だけなので、負荷の側へ大きく倒してよい。config の helper_test が同じ
// 用途に 3 秒を使っていたころ、`make check`（-race で全パッケージを同時に実行）では
// それを使い切ることがあり、自己設定テストが散発的に落ちた（Issue #140）。
//
// **何が 3 秒を使い切ったのかは特定していない。** 分かっているのは締め切りの側で
// 落ちていたこと（その定数を縮めると同じ行・同じメッセージで落ちる）までで、
// goroutine の遅れ・ファイル I/O・-race の負荷のどれが効いたかは切り分けていない。
//
// **購読の待ち受けにはこれを使わないこと。** 行が届くまで戻らない Cmd を諦めるのが
// 目的の待ちは、諦めるまでの時間がそのままテストの所要時間になる。そちらは呼び出し
// 側が短い値を決める（logs の waitTimeout）。
const CmdTimeout = 30 * time.Second

// ErrNoCmd は走らせる Cmd が無かったことを表す。
var ErrNoCmd = errors.New("走らせる Cmd が無い")

// ErrCmdTimeout は Cmd が待ち時間内に戻らなかったことを表す。
//
// **「Msg が出なかった」と区別できることが要件である。** 両方を 1 つの偽で返して
// いたころは、待ち時間切れを呼び出し側が「そもそも Cmd が出ていない」と読み、
// 実際には無い差分の判定を疑う失敗メッセージが出ていた（Issue #140）。
var ErrCmdTimeout = errors.New("待ち時間内に Cmd が戻らない")

// RunCmd は Cmd を 1 本実行して Msg を返す。timeout 内に戻らなければ ErrCmdTimeout を返す。
//
// 戻らない Cmd がありうるのが前提である。購読の待ち受け（行が届くまで戻らない Cmd）は
// **戻らないこと自体が正しい**ので、諦めて次へ進むための時間を呼び出し側が決める。
//
// チャネルに余裕を持たせるのは、諦めたあとに Cmd が戻ってきても送信で詰まらせない
// ためである（購読を畳めば必ず戻る）。
//
// **諦めた goroutine はそのまま走り続ける。** ここでできるのは待つのをやめることだけで、
// Cmd 自体は止められない。テストが後で読む状態を書き換える Cmd は待ち時間切れに
// させないこと——読み出しが放置した goroutine と競合する。
func RunCmd(cmd tea.Cmd, timeout time.Duration) (tea.Msg, error) {
	if cmd == nil {
		return nil, ErrNoCmd
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		return msg, nil
	case <-time.After(timeout):
		return nil, ErrCmdTimeout
	}
}

// bundle は束を幅優先で辿るキュー。Pump と FindMsg が共有する。
//
// **辿り方だけを共有する。** 1 本ずつ締め切り付きで走らせ、束（tea.Batch /
// tea.Sequence）なら中身をキューへ積み直し、そうでなければ Msg を 1 つ返す——ここまでが
// 2 つで同じ形であり、その先（Model を進めるか、述語で照合するか）は違う。**先まで
// 畳み込むと、進め方・条件・打ち切りを引数で渡し分けることになり、2 つの素朴なループ
// より読みにくくなる**ので寄せていない（Issue #147）。
type bundle struct {
	queue   []tea.Cmd
	timeout time.Duration
	// gaveUp は待ち時間内に戻らず諦めた本数。
	gaveUp int
}

// newBundle は cmd 1 本から辿り始めるキューを返す。
func newBundle(cmd tea.Cmd, timeout time.Duration) *bundle {
	return &bundle{queue: []tea.Cmd{cmd}, timeout: timeout, gaveUp: 0}
}

// push は辿る Cmd を末尾へ足す。
func (b *bundle) push(cmds ...tea.Cmd) { b.queue = append(b.queue, cmds...) }

// next は次の Msg を 1 つ返す。辿る Cmd が尽きたら ok が偽。
//
// **待ち時間切れで打ち切らない。** 束には戻らない Cmd が混じりうるので、1 本諦めても
// 残りを辿る（諦めた本数は gaveUp が数える）。
func (b *bundle) next() (tea.Msg, bool) {
	for len(b.queue) > 0 {
		c := b.queue[0]
		b.queue = b.queue[1:]

		msg, err := RunCmd(c, b.timeout)
		if errors.Is(err, ErrCmdTimeout) {
			b.gaveUp++

			continue
		}
		if err != nil {
			continue
		}
		if inner, isBundle := Cmds(msg); isBundle {
			b.push(inner...)

			continue
		}
		return msg, true
	}
	return nil, false
}

// Pump は Cmd を辿って Model を進め、cond が満たされた時点で止める。
//
// **bubbletea のランタイムの代わりである。** タブの検証は「発行した Cmd が次に何を運んで
// くるか」で合否が決まるため、Cmd を 1 本ずつ手で回すと辿り漏れがそのまま緑になる。
// Cmd の並び（tea.Batch）は中身へ辿り、page.Do の包み（page.TabMsg）は解いてから Model へ
// 渡す。戻らない Cmd は timeout で諦めて次へ進む（RunCmd）。
//
// Model の型を引数にしたのは、タブごとに具体型が違うためである。**進め方（step）を
// 呼び出し側から渡す**のは、Model を進めるついでに行う後始末（購読の追跡など）がタブに
// 固有だからで、ここに畳み込むと共有できる部分が無くなる。
//
// maxSteps で打ち切るのは、条件を満たさない Cmd の連鎖（購読の待ち受けが際限なく続く）で
// テストを止めないためである。**同じガードを FindMsg に付けていないのは、あちらの
// キューが増えないからである**——束の展開は有限で、Model を進めない FindMsg には
// 新しい Cmd を積む経路が無い。増えるのは step が Cmd を返すこちらだけである。
func Pump[M any](
	m M, tab int, cmd tea.Cmd, timeout time.Duration,
	step func(M, tea.Msg) (M, tea.Cmd), cond func(M) bool,
) (M, error) {
	const maxSteps = 200

	b := newBundle(cmd, timeout)
	for range maxSteps {
		if cond(m) {
			return m, nil
		}
		msg, ok := b.next()
		if !ok {
			return m, errors.New("辿れる Cmd が尽きたが条件を満たさなかった")
		}
		if tm, isTab := msg.(page.TabMsg); isTab {
			if tm.Tab != tab {
				return m, fmt.Errorf("宛先のタブ番号 = %d, want %d（page.TabMsg）", tm.Tab, tab)
			}
			msg = tm.Msg
		}
		if msg == nil {
			continue
		}
		var c tea.Cmd
		m, c = step(m, msg)
		b.push(c)
	}
	return m, fmt.Errorf("%d 手進めても条件を満たさなかった", maxSteps)
}

// Drained はチャネルが timeout 内に閉じたかを返す。
//
// 残っている値は読み捨てる。購読の後始末を確かめる側が知りたいのは「閉じたか」であり、
// 畳む直前に送出された値が残っていること自体は異常ではないためである。
func Drained[T any](ch <-chan T, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return true
			}
		case <-deadline:
			return false
		}
	}
}
