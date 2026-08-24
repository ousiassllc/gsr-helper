package cmdtest

import (
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
func RunAll(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	inner, ok := Cmds(cmd())
	if !ok {
		return
	}
	for _, ic := range inner {
		RunAll(ic)
	}
}

// Msgs は Cmd が返す Msg を平坦化して返す。並びは中身へ辿り、それ以外は 1 件として返す。
func Msgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	inner, ok := Cmds(msg)
	if !ok {
		return []tea.Msg{msg}
	}

	out := make([]tea.Msg, 0, len(inner))
	for _, c := range inner {
		out = append(out, Msgs(c)...)
	}
	return out
}
