// Package edit は runner 1 台の設定を「読む → 変更を組む → 差分を出す → 書く」
// までを受け持つ（FR-35〜FR-40）。
//
// **Config タブから tea に依らない部分だけを切り出したものである。** 画面の
// 状態遷移とフォームの組み立ては page/config が持ち、こちらはファイルと GitHub
// API の側だけを持つ。分けている理由は 2 つある。
//
//   - 差分の組み立てと書き込みは端末を起動せずに検証できる。tea.Model の中に
//     置くと、ファイルが正しく書けたかを確かめるのにキー入力の再現が要る。
//   - 1 ディレクトリ 2000 行の上限に対し、Config タブは画面だけで手一杯である。
//
// 差分に出す内容と実際に書き込む内容は同じ値から作る（Change の doc）。
package edit

// Kind は編集できる設定項目の種類。並びは画面仕様の Config タブのモックに従う。
type Kind int

const (
	// KindEnv は .env（環境変数・プロキシ・job hooks）。
	KindEnv Kind = iota
	// KindPath は .path。
	KindPath
	// KindDropIn は systemd drop-in。
	KindDropIn
	// KindLabels はラベル（GitHub API で即時反映）。
	KindLabels
	// KindGroup は runner group（GitHub API で即時反映）。
	KindGroup
	// KindReregister は名前 / work dir / ephemeral。再登録が要るため編集できない。
	KindReregister
	// KindCopy は .env を他の runner へ複製する（FR-40）。
	KindCopy
	// KindSelf は本ツール自身の設定（FR-41〜FR-42）。一覧には出さず、
	// 対象の選択から直接開く。
	KindSelf
)

// EnvSpec は .env のうちフォームに項目を出すキー 1 つ分。
type EnvSpec struct {
	// Key は .env に書かれるキー。
	Key string
	// Title はフォームに出す見出し。
	Title string
	// Desc はフォームに出す補足。空なら出さない。
	Desc string
}

// EnvKeys はフォームに項目を出す .env のキー。
//
// 並びと分類はデータモデルの「.env の扱い」の表に従う（ジョブ環境 / プロキシ /
// job hooks）。**.env の全キーを出すことはしない。** 任意のキーを書ける形式なので
// フォームに列挙しきれず、列挙していないキーは EnvFile が行ごと保持したまま
// 素通しする（変更しない行は 1 バイトも変わらない）。
var EnvKeys = []EnvSpec{
	{Key: "PATH", Title: "PATH", Desc: "ジョブに渡す PATH"},
	{Key: "LANG", Title: "LANG", Desc: "例: ja_JP.UTF-8"},
	{Key: "ImageOS", Title: "ImageOS", Desc: "ランナーイメージの識別子"},
	{Key: "https_proxy", Title: "https_proxy", Desc: ""},
	{Key: "http_proxy", Title: "http_proxy", Desc: ""},
	{Key: "no_proxy", Title: "no_proxy", Desc: "カンマ区切り"},
	{
		Key: "ACTIONS_RUNNER_HOOK_JOB_STARTED", Title: "job hook（開始時）",
		Desc: "ジョブ開始時に実行するスクリプトのパス",
	},
	{
		Key: "ACTIONS_RUNNER_HOOK_JOB_COMPLETED", Title: "job hook（完了時）",
		Desc: "ジョブ完了時に実行するスクリプトのパス",
	},
}
