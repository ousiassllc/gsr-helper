// Package hostcfg は環境診断（doctor）の 依存コマンド / systemd / 構成整合 の
// 3 分類を実装する（functional.md のチェック項目一覧表）。
//
// 3 分類を 1 つのパッケージに置いたのは、いずれも「ホストの構成が runner の
// 想定どおりか」を見る項目であり、同じ道具（systemctl の読み取りと検出済みの
// runner 一覧の突き合わせ）を共有するためである。
//
// **internal/runner を変更しない。** systemd.State は Restart も Environment も
// 持たないが、それを足すのは runner の検出（3 経路の突き合わせ）の責務であって
// 診断の都合ではない。要る値は doctor 側から systemctl show を発行して読む。
//
// **本パッケージの項目はすべて Startup() == false である。** systemctl の
// 一覧取得はユニット数に比例して時間がかかり、FR-44 が求める「ホスト内の
// 読み取りと軽量なコマンド」を外れる。
package hostcfg

import (
	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
)

// 各項目が check.Check を満たすことをコンパイル時に確かめる。
// 型は非公開なので、実装漏れに気付けるのはここだけである。
var (
	_ check.Check = depsCheck{}
	_ check.Check = unitCheck{}
	_ check.Check = orphanCheck{}
	_ check.Check = duplicateCheck{}
)

// Checks は本パッケージの診断項目を一覧の順で返す。
//
// 型を公開せず入口をここ 1 つに絞るのは、顔ぶれと並びの判断をこのパッケージの
// 中だけで完結させるためである。レジストリ（internal/doctor）は個々の型を知らない。
func Checks() []check.Check {
	return []check.Check{
		depsCheck{},
		unitCheck{},
		orphanCheck{},
		duplicateCheck{},
	}
}

// one は結果 1 件をスライスにして返す。
func one(r check.Result) []check.Result { return []check.Result{r} }
