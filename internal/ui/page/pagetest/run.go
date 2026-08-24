package pagetest

import (
	"errors"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/hostreq"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// Cmd を実際に走らせて結果を取り出す道具を集める。
//
// cmd.go が「Cmd の束をどう展開するか」を持つのに対し、こちらは「実行して待つ」側で
// ある。分けているのは、束の展開が Msg の形を見るだけの操作なのに対し、こちらは
// 戻らない Cmd（長寿命の購読の待ち受け）を諦める判断を含むためである。
//
// **諦める判断は RunCmd に集める。** cmd.go の Expand と HostReqOf は Cmd をここへ
// 通す——締め切りの掛かっていない実行が 1 本でも残っていると、戻らない Cmd を渡した
// テストが失敗ではなくハングになる（Issue #145）。**まだ素で実行する道具が残っている**
// ——cmd.go の RunAll / Msgs、この下の ChromeOf、parent.go の IsQuit である。どれも
// 戻らない Cmd を渡さないことが呼び出し側の前提であり、締め切りを通す作業は Issue #150
// が引き取っている。
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

// ChromeOf は Cmd の束に含まれる最初の ChromeMsg を返す。
//
// **束が入れ子になっていても辿る。** 以前は 1 段だけ展開して「ChromeMsg が奥から
// 出てくることは無い」としていたが、これは成り立たない——page が自分の ChromeMsg と
// 部品の返した Cmd をまとめて tea.Batch へ渡すと、内側の Batch がもう 1 段深くなる
// （絞り込みを始める `/` がその形になる。Issue #107）。
//
// **見つかった時点で打ち切る。** 束をすべて実行するわけではないので、絞り込みの
// カーソル点滅のような待つ Cmd は踏まない。page は ChromeMsg を束の先頭に置いており、
// 先頭で見つかればそれ以降は 1 つも実行しない。
func ChromeOf(cmd tea.Cmd) (page.ChromeMsg, bool) {
	if cmd == nil {
		return page.ChromeMsg{}, false
	}

	msg := cmd()
	if c, ok := msg.(page.ChromeMsg); ok {
		return c, true
	}
	inner, ok := Cmds(msg)
	if !ok {
		return page.ChromeMsg{}, false
	}
	for _, c := range inner {
		if v, found := ChromeOf(c); found {
			return v, true
		}
	}
	return page.ChromeMsg{}, false
}

// HostReqOf は Cmd の束に含まれる最初の起動時前提チェックの結果を返す。
// 束に無ければ ErrNotFound、束の展開が待ち時間内に戻らなければ ErrCmdTimeout を返す。
//
// ChromeOf と同じく 1 段だけ展開して探す。この結果を運ぶのは page.ChromeMsg では
// なく素の Msg であり（ヘッダと状態行の「ホスト前提 N 件」はタブの状態行とは別の
// 値である）、親 Model と Doctor タブの両方がこの経路を検証する。
//
// **「無い」と「戻らない」を分けて返す。** 1 つの偽で返していると、待ち時間切れを
// 呼び出し側が「件数が親へ届いていない」と読み、実際には無い配送の欠落を疑う失敗
// メッセージが出る（FindMsg と同じ理由。Issue #140 / #145）。
//
// **束の先頭も中身も締め切りを通す。** 先頭だけに掛けても、戻らない Cmd が束の
// 2 本目以降に混じればそこで止まる（Issue #145）。1 本諦めても残りは辿る。
func HostReqOf(cmd tea.Cmd) (hostreq.Msg, error) {
	cmds, err := Expand(cmd, CmdTimeout)
	if err != nil {
		return hostreq.Msg{}, err
	}
	gaveUp := 0
	for _, c := range cmds {
		msg, err := RunCmd(c, CmdTimeout)
		if err != nil {
			if errors.Is(err, ErrCmdTimeout) {
				gaveUp++
			}

			continue
		}
		if v, ok := msg.(hostreq.Msg); ok {
			return v, nil
		}
	}
	if gaveUp > 0 {
		return hostreq.Msg{}, fmt.Errorf("%w（%d 本が %s 以内に戻らなかった）", ErrCmdTimeout, gaveUp, CmdTimeout)
	}
	return hostreq.Msg{}, ErrNotFound
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
// テストを止めないためである。
func Pump[M any](
	m M, tab int, cmd tea.Cmd, timeout time.Duration,
	step func(M, tea.Msg) (M, tea.Cmd), cond func(M) bool,
) (M, error) {
	const maxSteps = 200

	queue := []tea.Cmd{cmd}
	for range maxSteps {
		if cond(m) {
			return m, nil
		}
		if len(queue) == 0 {
			return m, errors.New("辿れる Cmd が尽きたが条件を満たさなかった")
		}
		next := queue[0]
		queue = queue[1:]

		msg, err := RunCmd(next, timeout)
		if err != nil {
			continue
		}
		if inner, isBatch := Cmds(msg); isBatch {
			queue = append(queue, inner...)
			continue
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
		queue = append(queue, c)
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

// ErrNotFound は束を最後まで辿っても want を満たす Msg が無かったことを表す。
//
// ErrCmdTimeout と分けているのは、**呼び出し側の読み方が正反対だから**である
// ——こちらは Msg を出す側の判断（差分が無いなど）を、待ち時間切れは機械の
// 混み具合を疑う合図になる（Issue #140）。
var ErrNotFound = errors.New("目当ての Msg が束に無い")

// FindMsg は束の Cmd を順に走らせ、want を満たす最初の Msg を返す。
//
// **待ち時間切れで打ち切らない。** 束には戻らない Cmd が混じりうるので、1 本
// 諦めても残りを辿る。目当てが最後まで見つからなかったときだけ、諦めた本数が
// あれば ErrCmdTimeout を、無ければ ErrNotFound を返す——「見つからなかった」の
// 理由をここで確定させないと、呼び出し側の失敗メッセージが取り違える。
//
// **戻らない Cmd を含む束には向かない。** 目当てが無いときは束を実行しきるので、
// 購読を持つタブでは諦めるだけで timeout ぶん掛かる。そちらは Pump を使うこと。
func FindMsg(cmd tea.Cmd, timeout time.Duration, want func(tea.Msg) bool) (tea.Msg, error) {
	gaveUp := 0
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]

		msg, err := RunCmd(c, timeout)
		if errors.Is(err, ErrCmdTimeout) {
			gaveUp++

			continue
		}
		if err != nil {
			continue
		}
		if inner, isBundle := Cmds(msg); isBundle {
			queue = append(queue, inner...)

			continue
		}
		if want(msg) {
			return msg, nil
		}
	}
	if gaveUp > 0 {
		return nil, fmt.Errorf("%w（%d 本が %s 以内に戻らなかった）", ErrCmdTimeout, gaveUp, timeout)
	}
	return nil, ErrNotFound
}
