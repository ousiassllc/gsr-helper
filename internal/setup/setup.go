// Package setup は runner の追加・削除・バージョン更新を担う。
//
// 計画（Plan）と実行（Apply）を分離する。これにより実行前プレビュー（FR-16）が
// 計画をそのまま表示するだけで実現でき、表示と実行の食い違いが起きない
// （docs/components/overview.md「internal/setup」）。
//
// 短命トークンは計画には載せない。Step.TokenIndex が「Args のどこにトークンが
// 入るか」だけを持ち、実際の値は Apply が実行の直前に差し込む。プレビューが
// 参照するのは常に *** が入ったままの Args なので、承認画面にトークンが載る
// 経路が構造として存在しない（docs/architecture/security.md）。
package setup

import (
	"github.com/ousiassllc/gsr-helper/internal/exec/mask"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// Kind は計画の種類。
type Kind int

const (
	// KindAdd は runner の追加（FR-10〜FR-16）。
	KindAdd Kind = iota
	// KindRemove は runner の削除（FR-17〜FR-19）。
	KindRemove
	// KindUpdate は runner のバージョン更新（FR-20〜FR-22）。
	KindUpdate
)

// String は計画の種類の表示名を返す。
func (k Kind) String() string {
	switch k {
	case KindAdd:
		return "追加"
	case KindRemove:
		return "削除"
	case KindUpdate:
		return "バージョン更新"
	default:
		return "不明"
	}
}

// StepKind は 1 手順の種類。
type StepKind int

const (
	// StepCommand は外部コマンドの実行。
	StepCommand StepKind = iota
	// StepMkdir は runner ディレクトリの作成。
	StepMkdir
	// StepExtract は tarball の展開。
	StepExtract
	// StepDrain はジョブ完了を待つドレイン停止（FR-22）。
	StepDrain
)

// NoToken は Step.TokenIndex の「トークンを差し込まない」を表す値。
const NoToken = -1

// Step は 1 台分の手順 1 つ。
type Step struct {
	// Kind は手順の種類。
	Kind StepKind
	// Phase は進捗表示に出すフェーズ名（「展開」「登録」など）。
	Phase string
	// Name は StepCommand のときの実行ファイル。
	Name string
	// Args は StepCommand のときの引数。トークンの位置には mask.Placeholder が入る。
	Args []string
	// Dir は作業ディレクトリ。監査ログの dir にもなる。
	Dir string
	// Action は監査ログの action（runner.add / runner.remove / runner.update）。
	Action string
	// TokenIndex は Args のうち短命トークンを差し込む位置。NoToken なら差し込まない。
	TokenIndex int
	// Keep は StepExtract のときに上書きしない名前（FR-21）。
	Keep []string
}

// CommandLine は表示用の 1 行を返す。StepCommand 以外は空文字を返す。
//
// トークンの位置には計画の時点から mask.Placeholder が入っているため、ここで
// 改めてマスクしなくても平文にはならない。それでも mask.Args を通すのは、
// ラベルや URL に秘密情報らしい形が紛れた場合を取りこぼさないためである。
func (s Step) CommandLine() string {
	if s.Kind != StepCommand {
		return ""
	}
	return joinArgs(s.Name, mask.Args(s.Args))
}

// Unit は 1 台分の計画。
type Unit struct {
	// Name は runner 名。
	Name string
	// Dir は runner ディレクトリの絶対パス。
	Dir string
	// Runner は対象の runner。追加ではゼロ値。
	Runner runner.Runner
	// WasRunning は更新前に起動していたか（FR-22 の復帰判定）。
	WasRunning bool
	// Steps は実行順の手順。
	Steps []Step
}

// CommandLines は 1 台分の実行コマンド全文を実行順に返す。
func (u Unit) CommandLines() []string {
	out := make([]string, 0, len(u.Steps))
	for _, s := range u.Steps {
		if line := s.CommandLine(); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// Plan は実行前に確定した計画。
//
// これをそのまま表示したものが実行前プレビューであり、Apply が実行するのも
// これである。表示と実行の出どころを 1 つに保つ。
type Plan struct {
	// Kind は計画の種類。
	Kind Kind
	// Units は台ごとの計画。実行順に並ぶ。
	Units []Unit
	// NeedsToken は短命トークンを要するか。
	NeedsToken bool
	// NeedsTarball は tarball を要するか。
	NeedsTarball bool
	// Version は展開する runner のバージョン（表示用）。
	Version string
	// Warnings は承認前に見せる警告（ジョブ実行中など）。
	Warnings []string
	// Notes は補足（ディレクトリを残すことなど）。
	Notes []string
}

// Names は対象 runner 名を実行順に返す。
func (p Plan) Names() []string {
	out := make([]string, 0, len(p.Units))
	for _, u := range p.Units {
		out = append(out, u.Name)
	}
	return out
}

// Dirs は対象 runner ディレクトリを実行順に返す。
func (p Plan) Dirs() []string {
	out := make([]string, 0, len(p.Units))
	for _, u := range p.Units {
		out = append(out, u.Dir)
	}
	return out
}

// joinArgs は表示用にコマンド名と引数を空白で連結する。
func joinArgs(name string, args []string) string {
	out := name
	for _, a := range args {
		out += " " + a
	}
	return out
}
