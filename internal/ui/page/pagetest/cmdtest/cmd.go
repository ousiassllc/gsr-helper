package cmdtest

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Cmd の並び（tea.Batch / tea.Sequence の結果）を展開する道具を集める。
//
// 親 Model もタブも「発行した Cmd に何が入っているか」で検証する。同じ展開を
// パッケージごとに書き直すと、Batch を辿り損ねたテストだけが緑になる。

// Cmds は Msg が Cmd の並び（tea.Batch / tea.Sequence の結果）ならその中身を返す。
//
// **Batch と Sequence は区別できない。** tea.Sequence が返す Msg の型は非公開で
// あり、ここでは「Cmd のスライスであること」だけを見るためである。順序そのもの
// （後始末が終了より前か）を検証する側は、この関数ではなく Msg の型で判別すること。
func Cmds(msg tea.Msg) ([]tea.Cmd, bool) {
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Slice || v.Type().Elem() != reflect.TypeOf(tea.Cmd(nil)) {
		return nil, false
	}
	out := make([]tea.Cmd, 0, v.Len())
	for i := range v.Len() {
		c, ok := v.Index(i).Interface().(tea.Cmd)
		if !ok {
			return nil, false
		}
		out = append(out, c)
	}
	return out, true
}

// Expand は Cmd を 1 度だけ実行し、並びならその中身を、そうでなければ Cmd 自身を返す。
// timeout 内に戻らなければ ErrCmdTimeout を返す。
//
// 中の Cmd は実行しない。実行すると Tick が自動更新の間隔だけ待ち、検出がホストを
// 走査してしまう。
//
// **先頭の Cmd にも締め切りを掛ける（RunCmd を通す）。** 素で呼んでいたころは、束に
// なっていない「戻らない Cmd」を渡すとこの段で止まり、呼び出し側が RunCmd へ締め切りを
// 渡していても効かなかった。壊れ方が失敗ではなくハングになるため、CI では原因の
// 分からない `go test` の既定のタイムアウト（10 分）ぶんの停止に見える（Issue #145）。
//
// **cmd == nil は誤りではない。** Cmd を 1 本も発行しない Update は正常なので、
// 空の並びを返す（RunCmd の ErrNoCmd とはここが違う）。
//
// **束でなければ cmd 自身を返すので、呼び出し側が走らせると 2 度目の実行になる。**
// その 2 度目に締め切りは掛からない（掛かるのはここで束かを見る 1 度目だけである）。
// 副作用のある Cmd や、2 度目に戻らなくなる Cmd を渡さないこと。
func Expand(cmd tea.Cmd, timeout time.Duration) ([]tea.Cmd, error) {
	if cmd == nil {
		return nil, nil
	}
	msg, err := RunCmd(cmd, timeout)
	if err != nil {
		return nil, err
	}
	if inner, ok := Cmds(msg); ok {
		return inner, nil
	}
	return []tea.Cmd{cmd}, nil
}

// RunAll は Cmd を実行し、結果が Cmd の並びならその中身も再帰的に実行する。
// 走らせるだけで Msg は見ない（副作用――購読の畳み・後始末の発火――を起こすのが目的である）。
//
// **1 本ごとに timeout で諦める。** 素で呼んでいたころは、束に戻らない Cmd が
// 1 本混じるとそこで止まり、壊れ方が失敗ではなくハングになった（Issue #150）。
// 諦めた本数があれば ErrCmdTimeout を包んで返す——黙って飛ばすと、走らせたつもりの
// 後始末が起きていないまま呼び出し側が緑になる。
func RunAll(cmd tea.Cmd, timeout time.Duration) error {
	_, err := Msgs(cmd, timeout)

	return err
}

// Msgs は Cmd が返す Msg を平坦化して返す。並びは中身へ辿り、それ以外は 1 件として返す。
//
// **1 本ごとに timeout で諦め、残りは辿る。** 束には戻らない Cmd が混じりうるので、
// 1 本で打ち切ると後ろの Msg が落ちる（FindMsg と同じ方針）。素で呼んでいたころは
// 諦めもせず、戻らない Cmd が 1 本混じるとハングした（Issue #150）。
//
// **諦めた本数は error で返す。** 平坦化した並びは「これで全部」として読まれる
// ——黙って欠けさせると、届かなかった Msg を呼び出し側が「発行されていない」と読み、
// 実際には無い配送の欠落を疑う失敗メッセージが出る（ErrNotFound と ErrCmdTimeout を
// 分ける理由と同じ。Issue #140 / #145）。Msg 自体は返すので、欠けを承知で error を
// 捨てるのは AdvanceQuick だけである（Advance は MustMsgs で止める）。
func Msgs(cmd tea.Cmd, timeout time.Duration) ([]tea.Msg, error) {
	msgs, gaveUp := collectMsgs(cmd, timeout)
	if gaveUp > 0 {
		return msgs, fmt.Errorf("%w（%d 本が %s 以内に戻らなかった）", ErrCmdTimeout, gaveUp, timeout)
	}

	return msgs, nil
}

// MustMsgs は Msgs と同じ平坦化を行い、待ち時間切れがあれば panic で止める。
//
// **束を辿り切れることが前提の呼び出し側のためにある。** Msg を配り直す道具や
// 打鍵の往復（pagetest の ScanKey / ApplyChrome、各タブの検証）は「渡す Cmd は
// すべて有限時間で戻る」を前提に組んでおり、破れたのは呼び出し側の組み立ての
// 誤りである。そこへ error を足すと、返しの増えた道具が呼び出しのたびに増える
// だけで、判断は結局「テストを止める」に集まる。
//
// **止めるのが要点である。** 黙って欠かすと、辿れなかった Msg を呼び出し側が
// 「発行されていない」と読む assertion がすべて満たされて静かに緑になる
// （Issue #150）。素で走らせていたころは止まりもせず、壊れ方がハングだった。
//
// 諦めるかどうかを呼び出し側が判断したい場合は Msgs を使うこと。
func MustMsgs(cmd tea.Cmd, timeout time.Duration) []tea.Msg {
	msgs, err := Msgs(cmd, timeout)
	if err != nil {
		panic(fmt.Sprintf("Cmd の束を辿れない: %v", err))
	}

	return msgs
}

// collectMsgs は Msgs の本体。平坦化した Msg と、待ち時間内に戻らず諦めた本数を返す。
//
// **cmd == nil は誤りではない。** Cmd を 1 本も発行しない Update は正常なので、
// 空で返す（RunCmd の ErrNoCmd とはここが違う）。
func collectMsgs(cmd tea.Cmd, timeout time.Duration) ([]tea.Msg, int) {
	if cmd == nil {
		return nil, 0
	}

	msg, err := RunCmd(cmd, timeout)
	if err != nil {
		if errors.Is(err, ErrCmdTimeout) {
			return nil, 1
		}

		return nil, 0
	}

	inner, isBundle := Cmds(msg)
	if !isBundle {
		return []tea.Msg{msg}, 0
	}

	out := make([]tea.Msg, 0, len(inner))
	gaveUp := 0
	for _, c := range inner {
		got, n := collectMsgs(c, timeout)
		out = append(out, got...)
		gaveUp += n
	}

	return out, gaveUp
}
