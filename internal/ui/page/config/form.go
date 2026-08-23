package config

import (
	"errors"
	"strings"

	"charm.land/huh/v2"

	"github.com/ousiassllc/gsr-helper/internal/config"
)

// envKey は .env のうちフォームに項目を出すキー。
//
// 並びと分類はデータモデルの「.env の扱い」の表に従う（ジョブ環境 / プロキシ /
// job hooks）。**.env の全キーを出すことはしない。** 任意のキーを書ける形式なので
// フォームに列挙しきれず、列挙していないキーは EnvFile が行ごと保持したまま
// 素通しする（変更しない行は 1 バイトも変わらない）。
var envKeys = []struct {
	key   string
	title string
	desc  string
}{
	{"PATH", "PATH", "ジョブに渡す PATH"},
	{"LANG", "LANG", "例: ja_JP.UTF-8"},
	{"ImageOS", "ImageOS", "ランナーイメージの識別子"},
	{"https_proxy", "https_proxy", ""},
	{"http_proxy", "http_proxy", ""},
	{"no_proxy", "no_proxy", "カンマ区切り"},
	{"ACTIONS_RUNNER_HOOK_JOB_STARTED", "job hook（開始時）", "ジョブ開始時に実行するスクリプトのパス"},
	{"ACTIONS_RUNNER_HOOK_JOB_COMPLETED", "job hook（完了時）", "ジョブ完了時に実行するスクリプトのパス"},
}

// values はフォームの入力先。
//
// **実体を Model が持ち続ける。** huh はポインタで値を束縛するため、Update の
// たびに写しを作ると入力の書き込み先と読み出し先が別物になる（page/setup と同じ）。
type values struct {
	kind kind
	// env は envKeys と同じ添字で並ぶ入力欄の値。
	env []string
	// envBefore は開いた時点の値。空欄にした項目を「消す」と判断するために持つ。
	envBefore []string
	path      string
	restart   string
	memoryMax string
	labels    string
	group     string
	// groups は選べる runner group の名前。API から取る。
	groups []string
	// groupIDs は groups と同じ添字で並ぶ runner group の ID。
	// API は名前ではなく ID を取るため、選ばれた名前から引き当てるために持つ。
	groupIDs []int64
	// copyTo は複製先に選ばれた runner 名（FR-40）。
	copyTo []string
	// copyFrom は複製元の一覧に出す runner 名。
	copyCandidates []string
	// self は本ツール自身の設定の入力先（FR-41〜FR-42）。
	self selfValues
}

// newValues は空の入力の受け皿を作る。
func newValues() *values {
	return &values{
		kind: kindEnv, env: make([]string, len(envKeys)), envBefore: make([]string, len(envKeys)),
		path: "", restart: "", memoryMax: "", labels: "", group: "",
		groups: nil, groupIDs: nil, copyTo: nil, copyCandidates: nil,
		self: selfValues{scanRoots: "", refresh: "", warn: "", critical: "", auditLog: ""},
	}
}

// restartChoices は Restart= に選べる値。systemd の取り得る値のうち runner で
// 意味のあるものだけを出す。空は「本体の設定のまま」を表す。
var restartChoices = []string{"", "always", "on-failure", "no"}

// build は種類に応じた huh のフォームを組み立てる。
func (v *values) build(theme huh.Theme) *huh.Form {
	return huh.NewForm(huh.NewGroup(v.fields()...)).WithTheme(theme).WithShowHelp(true)
}

// fields は種類ごとの入力欄を返す。
func (v *values) fields() []huh.Field {
	switch v.kind {
	case kindEnv:
		return v.envFields()
	case kindPath:
		return []huh.Field{
			huh.NewInput().Title(".path").
				Description("ジョブの PATH を上書きする 1 行。空なら .path を空にします").
				Value(&v.path).Validate(validateLine),
		}
	case kindDropIn:
		return v.dropInFields()
	case kindLabels:
		return []huh.Field{
			huh.NewInput().Title("ラベル").
				Description("カンマ区切り。self-hosted / linux / x64 は自動で付きます").
				Value(&v.labels).Validate(validateLabels),
		}
	case kindGroup:
		return []huh.Field{v.groupField()}
	case kindCopy:
		return []huh.Field{v.copyField()}
	case kindSelf:
		return v.selfFields()
	case kindReregister:
		return nil
	default:
		return nil
	}
}

// envFields は .env の入力欄を返す。
func (v *values) envFields() []huh.Field {
	out := make([]huh.Field, 0, len(envKeys))
	for i, k := range envKeys {
		in := huh.NewInput().Title(k.title).Value(&v.env[i]).Validate(validateLine)
		if k.desc != "" {
			in = in.Description(k.desc)
		}
		out = append(out, in)
	}
	return out
}

// dropInFields は drop-in の入力欄を返す。
func (v *values) dropInFields() []huh.Field {
	opts := make([]huh.Option[string], 0, len(restartChoices))
	for _, c := range restartChoices {
		label := c
		if c == "" {
			label = "（本体の設定のまま）"
		}
		opts = append(opts, huh.NewOption(label, c))
	}

	return []huh.Field{
		huh.NewSelect[string]().Title("Restart").Options(opts...).Value(&v.restart),
		huh.NewInput().Title("MemoryMax").
			Description("例: 4G。空なら設定しません").
			Value(&v.memoryMax).Validate(validateLine),
	}
}

// groupField は runner group の選択欄を返す。
func (v *values) groupField() huh.Field {
	opts := make([]huh.Option[string], 0, len(v.groups))
	for _, g := range v.groups {
		opts = append(opts, huh.NewOption(g, g))
	}
	if len(opts) == 0 {
		return huh.NewInput().Title("runner group").
			Description("一覧を取得できませんでした。名前を直接入力します").
			Value(&v.group).Validate(validateLine)
	}

	return huh.NewSelect[string]().Title("runner group").Options(opts...).Value(&v.group)
}

// copyField は複製先の複数選択欄を返す（FR-40）。
func (v *values) copyField() huh.Field {
	opts := make([]huh.Option[string], 0, len(v.copyCandidates))
	for _, n := range v.copyCandidates {
		opts = append(opts, huh.NewOption(n, n))
	}

	return huh.NewMultiSelect[string]().
		Title("複製先の runner").
		Description("選んだ runner の .env を、この runner の .env で置き換えます").
		Options(opts...).Value(&v.copyTo)
}

// ErrEmptyCopyTarget は複製先を 1 つも選ばずに確定した場合のエラー。
var ErrEmptyCopyTarget = errors.New("複製先の runner を 1 つ以上選んでください")

// groupID は選ばれた runner group の名前から ID を引く。
// 一覧を取得できず名前を直接入力した場合は 0 と偽を返す。
func (v *values) groupID() (int64, bool) {
	for i, n := range v.groups {
		if n == v.group && i < len(v.groupIDs) {
			return v.groupIDs[i], true
		}
	}
	return 0, false
}

// validateLine は 1 行に収まる値かを見る。改行が入ると設定ファイルの行が壊れる。
func validateLine(s string) error {
	if strings.ContainsAny(s, "\r\n") {
		return errNewline
	}
	return nil
}

// 入力の形式に関するエラー。
var (
	// errNewline は改行を含む入力のエラー。
	errNewline = errors.New("改行は入力できません")
	// errBadNumber は数として読めない入力のエラー。
	errBadNumber = errors.New("数値を入力してください")
	// errPercentRange は割合が 1〜100 の外にある場合のエラー。
	errPercentRange = errors.New("1〜100 の範囲で入力してください")
)

// validateLabels はラベルの入力を検証する（FR-36）。
func validateLabels(s string) error {
	_, err := config.ValidateLabels(splitLabels(s))
	return err
}

// splitLabels はカンマ区切りのラベルを分ける。空要素の除去と trim は
// config.ValidateLabels に任せる。
func splitLabels(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Split(s, ",")
}
