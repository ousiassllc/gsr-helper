package edit

import (
	"errors"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/config"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// 一覧に出す「現在値」の読み取りと、フォームの 1 欄ぶんの検証。どちらも画面を
// 組み立てる材料だが端末に依らないので、行数の逼迫している page/config ではなく
// こちらに置く（Kind の doc と同じ理由）。

// Summary は対象 runner の現在値をまとめたもの。
//
// 画面の組み立てに要る値だけを持ち、ドメインの型を一覧へ持ち込まない。
// 読み取りに失敗した項目は空文字にして「値なし」として描く（一覧が出せなく
// なるより、その項目だけ値が読めていないと分かる方がよい）。
//
// **ラベルと runner group の現在値は持たない。** どちらも GitHub 側の値であり、
// 一覧を組み直すたびに API を呼ぶことになる。3 秒ポーリングでは API を呼ばない
// 方針（docs/api/external-interfaces.md）に従い、項目を選んだ時点で取りに行く。
type Summary struct {
	// EnvCount は .env に書かれているキーの数。
	EnvCount int
	// Path は .path の現在値。
	Path string
	// DropIn は drop-in の現在値の要約（"Restart=always, ..."）。
	DropIn string
	// HasUnit は systemd ユニットがあるか。無ければ drop-in を編集できない。
	HasUnit bool
	// Editable は runner の実体があり編集できるか。
	Editable bool
}

// Summarize は runner の現在値を読み取って要約する。
//
// 読み取りはファイル 3 つと検出済みの値だけで、GitHub API は呼ばない（3 秒
// ポーリングで API を呼ばない方針に沿う。ラベルは検出結果が持っている）。
func Summarize(r runner.Runner, ld Loader) Summary {
	s := Summary{EnvCount: 0, Path: "", DropIn: "", HasUnit: r.UnitName != "", Editable: true}
	if r.Dir == "" {
		s.Editable = false
		return s
	}

	if env, err := ld.Env(r); err == nil {
		s.EnvCount = len(env.Keys())
	}
	if p, err := ld.PathFile(r); err == nil {
		s.Path = p.Value
	}
	s.DropIn = dropInValue(r, ld)

	return s
}

// dropInValue は drop-in の現在値の要約を返す。
func dropInValue(r runner.Runner, ld Loader) string {
	if r.UnitName == "" {
		return ""
	}

	d, err := ld.DropIn(r)
	if err != nil || len(d.Directives) == 0 {
		return ""
	}

	parts := make([]string, 0, len(d.Directives))
	for _, dir := range d.Directives {
		parts = append(parts, dir.Key+"="+dir.Value)
	}
	return strings.Join(parts, ", ")
}

// ErrNewline は改行を含む入力のエラー。設定ファイルの行が壊れる。
var ErrNewline = errors.New("改行は入力できません")

// ValidateLine は 1 行に収まる値かを見る。
func ValidateLine(s string) error {
	if strings.ContainsAny(s, "\r\n") {
		return ErrNewline
	}
	return nil
}

// ValidateHook は job hooks のスクリプトパスを検証する。
//
// 他のキーより強く見るのは、この値が runner にジョブごとシェルで実行される
// ためである（docs/architecture/security.md）。
func ValidateHook(s string) error {
	if err := ValidateLine(s); err != nil {
		return err
	}

	_, err := config.ValidateHookPath(s, config.StatHook)

	return err
}

// ValidateLabelInput はカンマ区切りのラベルの入力を検証する（FR-36）。
func ValidateLabelInput(s string) error {
	_, err := LabelList(s)
	return err
}

// LabelList は入力を検証して整えたラベルの並びを返す（FR-36）。
func LabelList(s string) ([]string, error) {
	return config.ValidateLabels(SplitLabels(s))
}

// SplitLabels はカンマ区切りのラベルを分ける。空要素の除去と trim は
// config.ValidateLabels に任せる。
func SplitLabels(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Split(s, ",")
}

// JoinLabels はラベルの並びをフォームの 1 行にする。
func JoinLabels(labels []string) string { return strings.Join(labels, ",") }

// Names は runner の名前を並べて返す。
func Names(rs []runner.Runner) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Name())
	}
	return out
}
