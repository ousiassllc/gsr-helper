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

// StartOnce は起動時の前提チェック（FR-44）を発行する Cmd を返す。
//
// **runner を検出したあとに 1 度だけ走らせる。** パスワード不要 sudo と docker
// グループ所属は実行ユーザーごとに判定するので runner 一覧が要り、判定対象は
// ホストの構成なので秒単位では変わらない。3 秒ごとに走らせると、監査ログへ記録
// される `sudo -l -U` が他のレコードを押し流す。
//
// 「1 度だけ」の状態は呼び出し側（親 Model）が持つ 1 個の bool フィールドを
// *done として渡す。すでに発行済みなら nil を返し、発行できたときだけ *done を
// 真にする（Start が nil を返す——checks が空——ときは、まだ発行していない
// 扱いのままにする。runner の検出が続けば、次の周期でまた試せるようにするため）。
func StartOnce(done *bool, in doctor.Input, checks []doctor.Check) tea.Cmd {
	if *done {
		return nil
	}
	cmd := Start(in, checks)
	if cmd == nil {
		return nil
	}
	*done = true
	return cmd
}

// CountStartup は起動時の前提チェック（FR-44）に当たる結果だけを数えて Msg を返す。
//
// Doctor タブが再実行した結果を親へ届ける入口である。**全項目の結果をそのまま
// 渡してよい。** タブは登録されている項目をすべて走らせるが、ヘッダと状態行の
// 「ホスト前提 N 件」が指すのは起動時に見る項目だけであり、全体の件数を渡すと
// 別のものを数えた値がその場所に出る。
//
// **絞り込みに CheckResult.Startup は使わない。** 現状どの項目もその印を結果へ
// 写しておらず（各 Check は Startup を偽のまま返す）、印で絞ると不備が残って
// いても常に 0 件——つまり再実行するたびに警告が消える——ことになる。起動時の
// 顔ぶれを決めるのはレジストリなので、そこから識別子を引く。
func CountStartup(results []doctor.CheckResult) Msg {
	ids := startupIDs()
	startup := make([]doctor.CheckResult, 0, len(results))
	for _, r := range results {
		if _, ok := ids[r.ID]; ok {
			startup = append(startup, r)
		}
	}
	return Msg{Bad: doctor.Count(startup).Bad()}
}

// startupIDs は起動時に走らせる項目の識別子を返す。
func startupIDs() map[string]struct{} {
	checks := doctor.Startup(doctor.Default())
	ids := make(map[string]struct{}, len(checks))
	for _, c := range checks {
		ids[c.ID()] = struct{}{}
	}
	return ids
}
