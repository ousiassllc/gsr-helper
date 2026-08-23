package setup

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/exec/mask"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/setup/valid"
)

// 監査ログの action。
const (
	// ActionAdd は runner の追加。
	ActionAdd = "runner.add"
	// ActionRemove は runner の削除。
	ActionRemove = "runner.remove"
	// ActionUpdate は runner のバージョン更新。
	ActionUpdate = "runner.update"
)

// defaultWorkDir は config.sh --work の既定値。
const defaultWorkDir = "_work"

// AddSpec は追加の入力（FR-10〜FR-16）。
type AddSpec struct {
	// URL は登録先。config.sh --url にそのまま渡す。
	URL string
	// Scope は URL から判定した登録先。短命トークンの取得に使う。
	Scope scope.Scope
	// NamePrefix は `<ホスト名>-<連番>` のホスト名部分（FR-11）。
	NamePrefix string
	// Count は追加する台数（FR-10）。
	Count int
	// StartIndex は連番の開始値。NextIndex の結果を渡す。
	StartIndex int
	// Names は runner 名を明示する場合の一覧。空なら NamePrefix と StartIndex から
	// 連番で作る（FR-11）。
	//
	// 1 台ずつのウィザード追加（FR-12）は利用者が入力した名前をそのまま使う。
	// 連番の規則を通すと `gpu-box` が `gpu-box-1` になり、指定した名前で登録
	// されない。指定があるときは Count もその件数に従う。
	Names []string
	// Labels は追加で付けるラベル。予約ラベルは含めない。
	Labels []string
	// WorkDir は config.sh --work の値。空なら _work。
	WorkDir string
	// RunnerGroup は runner group 名。空なら --runnergroup を渡さない。
	RunnerGroup string
	// Ephemeral はジョブ 1 件で登録解除される runner にするか。
	Ephemeral bool
	// DisableUpdate は runner の自動更新を無効にするか。
	DisableUpdate bool
	// InstallBase は runner ディレクトリを作るベースディレクトリ。
	InstallBase string
	// RunAsUser は svc.sh install に渡す実行ユーザー。空なら渡さない。
	RunAsUser string
	// Version は展開する runner のバージョン（表示用）。
	Version string
	// Existing はホスト内の既存 runner 名。名前の重複検査に使う。
	Existing []string
	// Busy はジョブ実行中の runner 名。警告の組み立てに使う。
	Busy []string
}

// PlanAdd は追加の計画を立てる（FR-16）。
//
// 実行前に作成するディレクトリとコマンド全文をすべて確定させてから返す。
// 検証に落ちた時点でエラーにし、途中まで作った計画は返さない。
func PlanAdd(spec AddSpec) (Plan, error) {
	base, err := prepareAdd(&spec)
	if err != nil {
		return Plan{}, err
	}

	names := planNames(spec)
	existing := append(append([]string(nil), spec.Existing...), names...)

	units := make([]Unit, 0, len(names))
	for i, name := range names {
		// 追加する名前どうしの重複も見るため、自分より後ろの分だけを既存として渡す。
		if verr := valid.Name(name, existing[:len(spec.Existing)+i]); verr != nil {
			return Plan{}, verr
		}
		units = append(units, addUnit(spec, name, filepath.Join(base, name)))
	}

	return Plan{
		Kind:         KindAdd,
		Units:        units,
		NeedsToken:   true,
		NeedsTarball: true,
		Version:      spec.Version,
		Warnings:     addWarnings(spec.Busy),
		Notes:        addNotes(spec.Version),
	}, nil
}

// planNames は計画に載せる runner 名を決める。
func planNames(spec AddSpec) []string {
	if len(spec.Names) > 0 {
		return spec.Names
	}
	return Names(spec.NamePrefix, spec.StartIndex, spec.Count)
}

// prepareAdd は AddSpec を検証し、正規化した値を書き戻してベースディレクトリを返す。
func prepareAdd(spec *AddSpec) (string, error) {
	if len(spec.Names) > 0 {
		// 名前を明示する場合は台数と連番の規則を使わない。件数だけを揃える。
		spec.Count = len(spec.Names)
		spec.StartIndex = 1
	}
	if err := valid.Count(spec.Count); err != nil {
		return "", err
	}
	if spec.StartIndex < 1 {
		return "", fmt.Errorf("連番の開始値 %d: 1 以上を指定してください", spec.StartIndex)
	}

	u, err := valid.URL(spec.URL)
	if err != nil {
		return "", err
	}
	spec.URL = u

	base, err := valid.Dir("インストール先", spec.InstallBase)
	if err != nil {
		return "", err
	}

	labels, err := valid.Labels(spec.Labels)
	if err != nil {
		return "", err
	}
	spec.Labels = labels

	if spec.NamePrefix == "" && len(spec.Names) == 0 {
		return "", valid.ErrEmptyName
	}
	if strings.HasPrefix(spec.NamePrefix, "-") {
		return "", fmt.Errorf("名前の接頭辞 %q: %w", spec.NamePrefix, valid.ErrLeadingDash)
	}
	if spec.RunnerGroup != "" && strings.HasPrefix(spec.RunnerGroup, "-") {
		return "", fmt.Errorf("runner group %q: %w", spec.RunnerGroup, valid.ErrLeadingDash)
	}
	if spec.RunAsUser != "" && strings.HasPrefix(spec.RunAsUser, "-") {
		return "", fmt.Errorf("実行ユーザー %q: %w", spec.RunAsUser, valid.ErrLeadingDash)
	}
	if spec.WorkDir == "" {
		spec.WorkDir = defaultWorkDir
	}
	if strings.HasPrefix(spec.WorkDir, "-") {
		return "", fmt.Errorf("work dir %q: %w", spec.WorkDir, valid.ErrLeadingDash)
	}
	return base, nil
}

// addUnit は 1 台分の手順を組み立てる。
func addUnit(spec AddSpec, name, dir string) Unit {
	args, tokenAt := configureArgs(spec, name)

	steps := []Step{
		{
			Kind: StepMkdir, Phase: "ディレクトリ作成", Name: "", Args: nil,
			Dir: dir, Action: ActionAdd, TokenIndex: NoToken, Keep: nil,
		},
		{
			Kind: StepExtract, Phase: "展開", Name: "", Args: nil,
			Dir: dir, Action: ActionAdd, TokenIndex: NoToken, Keep: nil,
		},
		{
			Kind: StepCommand, Phase: "登録", Name: "./config.sh", Args: args,
			Dir: dir, Action: ActionAdd, TokenIndex: tokenAt, Keep: nil,
		},
		{
			Kind: StepCommand, Phase: "サービス登録", Name: "./svc.sh", Args: installArgs(spec.RunAsUser),
			Dir: dir, Action: ActionAdd, TokenIndex: NoToken, Keep: nil,
		},
		{
			Kind: StepCommand, Phase: "起動", Name: "./svc.sh", Args: []string{"start"},
			Dir: dir, Action: ActionAdd, TokenIndex: NoToken, Keep: nil,
		},
	}

	return Unit{Name: name, Dir: dir, Runner: runner.Runner{}, WasRunning: false, Steps: steps}
}

// configureArgs は config.sh の引数と、トークンを差し込む位置を返す。
//
// 位置には mask.Placeholder を置いたままにする。プレビューはこの配列をそのまま
// 表示するため、承認画面に平文のトークンが載る経路が存在しない。
func configureArgs(spec AddSpec, name string) ([]string, int) {
	args := []string{"--url", spec.URL, "--token"}
	tokenAt := len(args)
	args = append(args, mask.Placeholder, "--name", name)

	if len(spec.Labels) > 0 {
		args = append(args, "--labels", strings.Join(spec.Labels, ","))
	}
	args = append(args, "--work", spec.WorkDir)
	if spec.RunnerGroup != "" {
		args = append(args, "--runnergroup", spec.RunnerGroup)
	}
	args = append(args, "--unattended")
	if spec.Ephemeral {
		args = append(args, "--ephemeral")
	}
	if spec.DisableUpdate {
		args = append(args, "--disableupdate")
	}
	return args, tokenAt
}

// installArgs は svc.sh install の引数を返す。
func installArgs(user string) []string {
	if user == "" {
		return []string{"install"}
	}
	return []string{"install", user}
}

// addWarnings はジョブ実行中の runner がある場合の警告を返す。
//
// 登録時トークンはプロセス引数として渡るため、任意のコードを実行しうるジョブが
// 動いている最中の登録が最も危険である
// （docs/architecture/security.md「プロセス引数からのトークン読み取り」）。
func addWarnings(busy []string) []string {
	if len(busy) == 0 {
		return nil
	}
	return []string{
		"⚠ 登録時、トークンはプロセス引数として渡ります。同一ホストの他プロセスから",
		"  読み取れる可能性があるため、ジョブ実行中の登録は避けてください。",
		"  現在ジョブ実行中の runner: " + strings.Join(busy, ", "),
	}
}

// addNotes は追加の補足を返す。
func addNotes(version string) []string {
	if version == "" {
		return []string{"tarball は SHA-256 を検証してから展開します。"}
	}
	return []string{"runner バージョン: " + version + "（SHA-256 を検証してから展開します）"}
}
