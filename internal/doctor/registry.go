package doctor

import (
	"github.com/ousiassllc/gsr-helper/internal/doctor/authz"
	"github.com/ousiassllc/gsr-helper/internal/doctor/hostcfg"
	"github.com/ousiassllc/gsr-helper/internal/doctor/hostres"
	"github.com/ousiassllc/gsr-helper/internal/doctor/jobreq"
	"github.com/ousiassllc/gsr-helper/internal/doctor/netcheck"
)

// Default はレジストリ。実行する診断項目をすべて返す。
//
// **項目を足す作業はこの関数に 1 行足すことに閉じる。** 分類ごとのパッケージが
// 自分の顔ぶれを Checks() で返し、ここはそれを連ねるだけである。既存の項目にも
// 実行（Run）にも触れずに項目を追加できる、という構造が components/overview.md の
// 「項目の追加が既存コードに影響しない」の実体である。
//
// 並びは functional.md のチェック項目一覧表の分類順に合わせてあるが、**表示順は
// これで決まらない。** 整列は Run が分類・識別子・対象で行う（並列実行の完了順に
// 引きずられないため）。ここの並びが効くのは、同じ分類・同じ識別子の中の相対順
// だけである。
func Default() []Check {
	groups := [][]Check{
		authz.Checks(),
		netcheck.Checks(),
		hostres.Checks(),
		jobreq.Checks(),
		hostcfg.Checks(),
	}

	var out []Check
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}
