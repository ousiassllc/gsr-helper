// Package check は環境診断（doctor）の 1 項目を表す型を置く。
//
// 診断項目の実装（下位パッケージ）とレジストリ（internal/doctor）の両方から参照
// されるため、両者のどちらでもない葉のパッケージに切り出してある。レジストリは
// 各項目を import し、各項目はこの型だけを import するので、import の循環が
// 構造として起きない。呼び出し側（UI）が名指しするのは internal/doctor の別名
// （doctor.Check / doctor.CheckResult）であり、このパッケージを直に import する
// のは診断項目の実装だけである。
package check

// 診断項目の分類。data-model.md の CheckResult.Category と一対一で対応する。
//
// 型を付けず素の文字列にしてあるのは、Check.Category() の戻り値が string だと
// components/overview.md が定めているためである。表示にそのまま使うので、値は
// 画面に出す表記そのものにする。
const (
	CatAuthz       = "認証・権限"
	CatNetwork     = "ネットワーク"
	CatTime        = "時刻"
	CatResource    = "リソース"
	CatHistory     = "障害履歴"
	CatDocker      = "docker"
	CatJobReq      = "ジョブ実行の前提"
	CatDeps        = "依存コマンド"
	CatSystemd     = "systemd"
	CatConsistency = "構成整合"
)

// categoryOrder は一覧に並べる順。functional.md のチェック項目一覧表の順に揃える。
//
// 分類ごとにまとめて出すのは、対処が分類単位で決まる（docker を入れる、sudoers を
// 直す）ためである。ここに無い分類は末尾へ回す。
var categoryOrder = []string{
	CatAuthz,
	CatNetwork,
	CatTime,
	CatResource,
	CatHistory,
	CatDocker,
	CatJobReq,
	CatDeps,
	CatSystemd,
	CatConsistency,
}

// CategoryRank は分類の並び順を返す。未知の分類は末尾（len(categoryOrder)）になる。
func CategoryRank(category string) int {
	for i, c := range categoryOrder {
		if c == category {
			return i
		}
	}
	return len(categoryOrder)
}

// Categories は一覧に並べる順の分類をすべて返す。
func Categories() []string {
	out := make([]string, len(categoryOrder))
	copy(out, categoryOrder)
	return out
}

// Status は 1 項目の判定。
//
// SKIP は「能力不足で実行できなかった」を表し、FAIL（実行したうえで不備が
// 見つかった）と区別する（FR-32）。docker が無い環境で docker のチェックを
// FAIL にすると、docker を使わない運用でも赤が残り続けて診断が読めなくなる。
type Status int

// Status の取り得る値。並び順は深刻度の昇順で、一覧の要約もこの順に出す。
const (
	OK   Status = iota // 正常
	Warn               // 注意。運用の判断に委ねる不備
	Fail               // 異常。放置するとジョブか runner が失敗する
	Skip               // 能力不足で実行できなかった
)

// String は画面と一覧に出す表記を返す。未定義の値は空文字を返す。
func (s Status) String() string {
	switch s {
	case OK:
		return "OK"
	case Warn:
		return "WARN"
	case Fail:
		return "FAIL"
	case Skip:
		return "SKIP"
	default:
		return ""
	}
}

// Bad は対処が要る判定かを返す（WARN / FAIL）。
//
// 起動時の自動判定（FR-44）が状態行に出す件数と、一覧の要約の強調がこの判定を
// 共有する。「OK でも SKIP でもない」を各所に書き下すと、状態を足したときに
// 数え方が食い違う。
func (s Status) Bad() bool { return s == Warn || s == Fail }

// Result は 1 項目の診断結果（data-model.md の CheckResult）。
//
// 1 つの Check が複数の Result を返すことがある。runner ごとに判定する項目
// （パーミッション・docker グループ所属）は runner の数だけ行が並ぶためである
// （screens.md の Doctor タブは TARGET 列に runner 名を出す）。
type Result struct {
	// ID はチェックの識別子。同じ Check が複数行を返す場合も同じ値になる。
	ID string
	// Category は分類。上記の Cat* のいずれか。
	Category string
	// Target は対象 runner の名前。ホスト全体のチェックでは空。
	Target string
	// Status は判定。
	Status Status
	// Summary は一覧の CHECK 列に出す 1 行の要約（「NTP 未同期（ずれ 42 秒）」）。
	Summary string
	// Detail は詳細画面の「検出内容」。判定の根拠（実測値）を書く。
	Detail string
	// Impact は詳細画面の「影響」。放置すると何が起きるかを書く。
	Impact string
	// Remedy は詳細画面の「推奨する対処」。**表示するだけで実行はしない**
	// （security.md のパスワード不要 sudo の要求への対応）。
	Remedy string
	// Startup は起動時の自動実行（FR-44）の対象か。Check.Startup() を写す。
	Startup bool
}
