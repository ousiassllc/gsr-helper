package cmdtest

import (
	"errors"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/hostreq"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// 走らせた束から目当ての 1 件を拾う道具を集める。
//
// **run.go から分けてあるのは 1 ファイル 300 行の上限のためである**
// （atomic-design.md の「ファイルの行数」。Issue #150 で ChromeOf に締め切りを
// 通したときに超えた）。境界は責務に沿わせた——run.go が「Cmd をどう走らせ、
// どう待つか」を持つのに対し、こちらは「走らせた結果から何を拾うか」を持つ。
// 辿るキュー（run.go の bundle）は両方から使う。
//
// **「無い」と「戻らない」を必ず分けて返す。** 1 つの偽に丸めると、待ち時間切れを
// 呼び出し側が「そもそも発行されていない」と読み、実際には無い欠落を疑う失敗
// メッセージが出る（Issue #140 / #145 / #150）。

// ChromeOf は Cmd の束に含まれる最初の ChromeMsg を返す。
// 束に無ければ ErrNotFound、辿る途中で諦めて見つからなければ ErrCmdTimeout を返す。
//
// **束が入れ子になっていても辿る。** 以前は 1 段だけ展開して「ChromeMsg が奥から
// 出てくることは無い」としていたが、これは成り立たない——page が自分の ChromeMsg と
// 部品の返した Cmd をまとめて tea.Batch へ渡すと、内側の Batch がもう 1 段深くなる
// （絞り込みを始める `/` がその形になる。Issue #107）。
//
// **見つかった時点で打ち切る。** 束をすべて実行するわけではないので、絞り込みの
// カーソル点滅のような待つ Cmd は踏まない。page は ChromeMsg を束の先頭に置いており、
// 先頭で見つかればそれ以降は 1 つも実行しない。
//
// **踏んでしまった分は timeout で諦める。** ChromeMsg が先頭に無い束（購読を持つ
// Logs タブなど）では待つ Cmd を踏みうる。素で呼んでいたころはそこで止まり、壊れ方が
// 失敗ではなくハングになった（Issue #150）。1 本諦めても残りは辿る。
func ChromeOf(cmd tea.Cmd, timeout time.Duration) (page.ChromeMsg, error) {
	chrome, gaveUp, found := chromeOf(cmd, timeout)
	if found {
		return chrome, nil
	}
	if gaveUp > 0 {
		return page.ChromeMsg{}, fmt.Errorf("%w（%d 本が %s 以内に戻らなかった）", ErrCmdTimeout, gaveUp, timeout)
	}

	return page.ChromeMsg{}, ErrNotFound
}

// chromeOf は ChromeOf の本体。ChromeMsg と、諦めた本数と、見つかったかを返す。
func chromeOf(cmd tea.Cmd, timeout time.Duration) (page.ChromeMsg, int, bool) {
	if cmd == nil {
		return page.ChromeMsg{}, 0, false
	}

	msg, err := RunCmd(cmd, timeout)
	if err != nil {
		if errors.Is(err, ErrCmdTimeout) {
			return page.ChromeMsg{}, 1, false
		}

		return page.ChromeMsg{}, 0, false
	}
	if c, isChrome := msg.(page.ChromeMsg); isChrome {
		return c, 0, true
	}

	inner, isBundle := Cmds(msg)
	if !isBundle {
		return page.ChromeMsg{}, 0, false
	}

	gaveUp := 0
	for _, c := range inner {
		v, n, found := chromeOf(c, timeout)
		gaveUp += n
		if found {
			return v, gaveUp, true
		}
	}

	return page.ChromeMsg{}, gaveUp, false
}

// HostReqOf は Cmd の束に含まれる最初の起動時前提チェックの結果を返す。
// 束に無ければ ErrNotFound、束の展開が待ち時間内に戻らなければ ErrCmdTimeout を返す。
//
// 束は 1 段だけ展開して探す（Expand を通す）ので、束の中に束があってもそこまでは
// 潜らない。この結果を運ぶのは page.ChromeMsg ではなく素の Msg であり（ヘッダと
// 状態行の「ホスト前提 N 件」はタブの状態行とは別の値である）、親 Model と Doctor
// タブの両方がこの経路を検証する。
//
// **「無い」と「戻らない」を分けて返す。** 1 つの偽で返していると、待ち時間切れを
// 呼び出し側が「件数が親へ届いていない」と読み、実際には無い配送の欠落を疑う失敗
// メッセージが出る（FindMsg と同じ理由。Issue #140 / #145）。
//
// **束の先頭も中身も締め切りを通す。** 先頭だけに掛けても、戻らない Cmd が束の
// 2 本目以降に混じればそこで止まる（Issue #145）。1 本諦めても残りは辿る。
//
// **timeout は呼び出し側が決める**（ChromeOf と同じ形。Issue #156）。CmdTimeout を
// 内側で固定していたころは、諦める側を検証する 1 本がそのまま 30 秒かかり、
// `make check` が毎回それを払っていた。諦めるまでの時間がそのまま所要時間になる
// 検証はミリ秒の締め切りで書くこと。
func HostReqOf(cmd tea.Cmd, timeout time.Duration) (hostreq.Msg, error) {
	cmds, err := Expand(cmd, timeout)
	if err != nil {
		return hostreq.Msg{}, err
	}
	gaveUp := 0
	for _, c := range cmds {
		msg, err := RunCmd(c, timeout)
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
		return hostreq.Msg{}, fmt.Errorf("%w（%d 本が %s 以内に戻らなかった）", ErrCmdTimeout, gaveUp, timeout)
	}
	return hostreq.Msg{}, ErrNotFound
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
	b := newBundle(cmd, timeout)
	for {
		msg, ok := b.next()
		if !ok {
			break
		}
		if want(msg) {
			return msg, nil
		}
	}
	if b.gaveUp > 0 {
		return nil, fmt.Errorf("%w（%d 本が %s 以内に戻らなかった）", ErrCmdTimeout, b.gaveUp, timeout)
	}
	return nil, ErrNotFound
}
