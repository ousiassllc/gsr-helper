package hostreq

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/doctor"
)

// State は起動時の前提チェック（FR-44）の進行状況と結果を持つ。親 Model はこれを
// 1 つ持ち、駆動の契機（検出に成功した周期）だけを決める。
//
// **親に int と bool を並べさせない。** 「1 度きり」と「その結果の件数」は前提
// チェックの側の不変条件であり、持ち主が離れると片方だけを更新する経路ができる
// （workscan.State / discovery.State と同じ判断）。
type State struct {
	// Checks は起動時に走らせる診断項目。空なら走らせない。**テストの
	// 差し替え口でもある**（本物は実ホストの sudo / docker / /etc/group を読む）。
	Checks []doctor.Check

	// bad は対処が要る項目の件数、done は 1 度発行したか。
	bad  int
	done bool
}

// Bad は対処が要る項目の件数（WARN + FAIL）を返す。親は chrome の入力へ写す。
func (s *State) Bad() int { return s.bad }

// StartOnce は起動時の前提チェック（FR-44）を発行する Cmd を返す。
//
// **runner を検出したあとに 1 度だけ走らせる。** パスワード不要 sudo と docker
// グループ所属は実行ユーザーごとに判定するので runner 一覧が要り、判定対象は
// ホストの構成なので秒単位では変わらない。3 秒ごとに走らせると、監査ログへ記録
// される `sudo -l -U` が他のレコードを押し流す。
//
// すでに発行済みなら nil を返し、発行できたときだけ「1 度きり」を使い切ったことに
// する（Start が nil を返す——Checks が空——ときは、まだ発行していない扱いのままに
// する。runner の検出が続けば、次の周期でまた試せるようにするため）。
func (s *State) StartOnce(in doctor.Input) tea.Cmd {
	if s.done {
		return nil
	}
	cmd := Start(in, s.Checks)
	if cmd == nil {
		return nil
	}
	s.done = true
	return cmd
}

// Apply は前提チェックの結果を取り込む。
//
// Doctor タブの再実行で届き直せば上書きする。据え置くと、sudo や docker グループを
// 直したあとも全て OK になったタブへ誘導し続けることになる。
func (s *State) Apply(msg Msg) { s.bad = msg.Bad }
