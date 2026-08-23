// Package doctor は環境診断（FR-32〜FR-34 / FR-43〜FR-44）を行う。
//
// 診断項目そのものは分類ごとの下位パッケージにあり、ここはそれらを並べた
// レジストリ（Default）と並列実行（Run）だけを持つ。項目の型は葉のパッケージ
// （doctor/check）にあるので、レジストリが項目を import しても循環しない。
//
// 呼び出し側（UI）はこのパッケージだけを import すればよいよう、check の型を
// ここで再公開する。
package doctor

import (
	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
)

// Check は 1 つの診断項目。詳細は check.Check を参照。
type Check = check.Check

// CheckResult は 1 項目の診断結果。詳細は check.Result を参照。
type CheckResult = check.Result

// Input は全チェックに配る入力。詳細は check.Input を参照。
type Input = check.Input

// Status は 1 項目の判定。詳細は check.Status を参照。
type Status = check.Status

// Status の取り得る値。check 側の定数をそのまま再公開する。
const (
	OK   = check.OK
	Warn = check.Warn
	Fail = check.Fail
	Skip = check.Skip
)

// Categories は一覧に並べる順の分類をすべて返す。
func Categories() []string { return check.Categories() }
