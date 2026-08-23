// Package hostreq は起動時のジョブ実行の前提チェック（FR-44）を発行する。
//
// 親 Model（ui）から切り出してあるのは 2 つの理由による。1 つは行数で、ui 直下は
// 1 ディレクトリ 2000 行の上限に対して余裕が無く、タブが要する起動時の値を足すたびに
// 押し上がる（atomic-design.md「一覧タブを 1 枚足せる余裕」）。もう 1 つは検証で、
// 発行そのものは親 Model の非公開な状態に触れずに確かめられる。
//
// **判定の中身は持たない。** 何を確かめるかは internal/doctor のレジストリが決め、
// ここは「Startup が真の項目を走らせ、対処が要る件数だけを返す」ことに徹する。
package hostreq

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/doctor"
)

// Budget は起動時の前提チェックに与える上限。
//
// 対象はホスト内の読み取りと軽量なコマンドだけ（`sudo -l -U` / `docker buildx
// version` / /etc/group と /proc の読み取り）なので、応答すれば数百ミリ秒で終わる。
// これを超えるのは sudo か docker が応答しない異常時であり、そのときは判定を
// 諦める。**警告が出ないだけで起動は妨げない。**
const Budget = 10 * time.Second

// Msg は起動時の前提チェックの結果。
//
// 件数だけを運ぶ。詳細は Doctor タブが自分で診断し直して出すので、親が結果の
// 一覧を抱える必要は無い（抱えると、タブが再実行した後も親の件数だけが古くなる）。
type Msg struct {
	// Bad は対処が要る項目の件数（WARN + FAIL）。
	Bad int
}

// Start は checks を走らせる Cmd を返す。checks が空なら nil を返す。
//
// nil を返す形にしてあるのは、呼び出し側が「発行しないときは Cmd を束ねない」
// 判断をできるようにするためである（束ねると共有状態の配布だけの Cmd の形が変わる）。
func Start(in doctor.Input, checks []doctor.Check) tea.Cmd {
	if len(checks) == 0 {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), Budget)
		defer cancel()
		return Msg{Bad: doctor.Count(doctor.Run(ctx, in, checks)).Bad()}
	}
}
