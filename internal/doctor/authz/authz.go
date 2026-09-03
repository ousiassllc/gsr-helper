// Package authz は環境診断（doctor）の「認証・権限」分類の項目を実装する
// （docs/requirements/functional.md のチェック項目一覧表）。
//
// 分類ごとにパッケージを分けているのは、項目の追加が他の分類のコードに触れずに
// 済むようにするためである。レジストリ（internal/doctor）はこの Checks() を
// 並べるだけでよく、個々の項目の型を知らない。
//
// **この分類の項目は秘密そのものを読まない。** .credentials の中身も
// GitHub トークンの値も扱わず、パーミッション・所有者・保有スコープという
// 「外形」だけで判定する（docs/architecture/security.md の「保持と出力」）。
package authz

import (
	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
)

// 実装が Check を満たすことを構築時に固定する。項目を足したときに
// Checks() へ並べる前の段階で取りこぼしに気付ける。
var (
	_ check.Check = perm{}
	_ check.Check = hidepid{}
	_ check.Check = scopes{}
)

// Checks は「認証・権限」の診断項目を表示順に返す。
//
// 項目の型を公開せず入口をここ 1 つに絞るのは、順序と顔ぶれの判断を
// このパッケージの中だけで完結させるためである。レジストリ側で個々の型を
// 名指しできる形にすると、並びの規定が 2 箇所に散る。
func Checks() []check.Check {
	return []check.Check{perm{}, hidepid{}, scopes{}}
}
