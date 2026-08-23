// Package netcheck は doctor の「ネットワーク」分類（functional.md のチェック
// 項目一覧表）を実装する。
//
// 分類ごとにパッケージを分けてあるのは、項目を足しても他の分類のコードに
// 触れずに済むようにするためである。ここが持つのは 2 項目だけで、レジストリ
// （internal/doctor）へ並べるのは Checks の戻りである。
//
// **この分類は起動時の自動判定（FR-44）に入れない。** 起動時に外部へ TCP を
// 張ると、回線の遅い環境やプロキシ配下で起動そのものが待たされる。到達性は
// 利用者が Doctor タブで明示的に起こすときだけ確かめる。
package netcheck

import (
	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
)

// 診断項目が check.Check を満たすことをコンパイル時に確かめる。
//
// レジストリへ並べるのは Checks の戻りだけなので、実装から取りこぼしても
// 型検査では気づけない。ここで名指ししておく。
var (
	_ check.Check = reachCheck{}
	_ check.Check = proxyCheck{}
)

// Checks はこの分類の診断項目を表示順に返す。
//
// 並びを固定するのは、到達性（外向きの通信そのもの）を先に、プロキシ設定
// （到達性が失敗したときの原因側）を後に読ませたいからである。
func Checks() []check.Check {
	return []check.Check{
		reachCheck{},
		proxyCheck{},
	}
}
