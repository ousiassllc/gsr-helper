package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/setup/tarball"
	"github.com/ousiassllc/gsr-helper/internal/svc"
)

// dirMode は作成する runner ディレクトリのパーミッション。
//
// 手作業での設置（mkdir → tar 展開）と同じ 0755 にする。実行ユーザーへの
// 引き渡しは svc.sh install [user] が行う。
const dirMode = 0o755

// 実行時のエラー。
var (
	// ErrTokenRequired は短命トークンを要する計画にトークンが渡されなかった場合のエラー。
	ErrTokenRequired = errors.New("この操作には短命トークンが必要です")
	// ErrTarballRequired は tarball を要する計画に展開元が渡されなかった場合のエラー。
	ErrTarballRequired = errors.New("この操作には runner の tarball が必要です")
	// ErrNoExecutor は Executor が渡されなかった場合のエラー。
	ErrNoExecutor = errors.New("コマンドの実行手段が設定されていません")
)

// Progress は 1 手順ごとの進捗。
type Progress struct {
	// Index は処理中の台の 0 始まりの番号。
	Index int
	// Total は台数。
	Total int
	// Name は処理中の runner 名。
	Name string
	// Phase は処理中のフェーズ名（「展開」「登録」など）。
	Phase string
	// Done はこの台が最後まで終わったか。
	Done bool
	// Err はこの台が失敗した理由。成功なら nil。
	Err error
}

// ApplyInput は Apply の入力。
type ApplyInput struct {
	// Exec は外部コマンドの実行手段。
	Exec exec.Executor
	// Plan は実行する計画。
	Plan Plan
	// Token は全台で共通の短命トークン。Plan.NeedsToken が真のときに必須。
	//
	// 追加はスコープが 1 つなので 1 本のトークンを台数分で使い回せる（FR-14）。
	Token string
	// TokenFor は台ごとの短命トークンを返す。設定されていれば Token より優先する。
	//
	// 削除の対象は複数のスコープにまたがりうる（screens.md の削除の画面例は
	// repo と org の runner を同時に並べている）。remove token はスコープごとに
	// 発行されるため、全台で 1 本を使い回すと別スコープの台で必ず失敗する。
	TokenFor func(ctx context.Context, u Unit) (string, error)
	// Tarball は展開元 tarball のパス。Plan.NeedsTarball が真のときに必須。
	Tarball string
	// Drain はドレイン停止の手段。nil なら svc.Drain を使う（テストで差し替える）。
	Drain func(ctx context.Context, ex exec.Executor, r runner.Runner) error
	// Progress は進捗の通知先。nil 可。
	Progress func(Progress)
}

// Result は実行結果（FR-15）。
type Result struct {
	// Succeeded は最後まで成功した runner 名。
	Succeeded []string
	// Failed は失敗した runner 名。成功なら空。
	Failed string
	// Phase は失敗したフェーズ名。
	Phase string
	// Err は失敗の理由。
	Err error
	// Remaining は着手しなかった runner 名。
	Remaining []string
}

// OK は 1 台も失敗しなかったかを返す。
func (r Result) OK() bool { return r.Failed == "" && r.Err == nil }

// Apply は計画を順に実行する。
//
// **途中で失敗した場合はその台で中止し、成功済みの runner は残す**（FR-15）。
// 何台目までが成功し、どこで何が失敗したかを Result に載せて返す。戻り値の
// error は Result.Err と同じもので、呼び出し側が errors.Is で扱えるようにしてある。
//
// ctx は台と手順の境界で見る（docs/architecture/security.md「context で
// キャンセルできる」）。打ち切った時点で着手していない台は、失敗のときと同じく
// Result.Remaining に載る。
func Apply(ctx context.Context, in ApplyInput) (Result, error) {
	if err := validateApply(in); err != nil {
		return Result{
			Succeeded: nil, Failed: "", Phase: "", Err: err, Remaining: in.Plan.Names(),
		}, err
	}

	units := in.Plan.Units
	done := make([]string, 0, len(units))

	for i, u := range units {
		// 着手前に打ち切るので、この台は「失敗」ではなく未着手として扱う。
		// Failed を空にしておかないと、手を付けていない台を壊したように見える。
		if err := ctx.Err(); err != nil {
			return Result{
				Succeeded: done, Failed: "", Phase: "", Err: err, Remaining: remaining(units, i),
			}, err
		}

		if err := applyUnit(ctx, in, u, i, len(units)); err != nil {
			return Result{
				Succeeded: done,
				Failed:    u.Name,
				Phase:     phaseOf(err),
				Err:       err,
				Remaining: remaining(units, i+1),
			}, err
		}
		done = append(done, u.Name)
		notify(in.Progress, Progress{
			Index: i, Total: len(units), Name: u.Name, Phase: "完了", Done: true, Err: nil,
		})
	}

	return Result{Succeeded: done, Failed: "", Phase: "", Err: nil, Remaining: nil}, nil
}

// validateApply は実行前に入力の前提を確かめる。
func validateApply(in ApplyInput) error {
	if in.Exec == nil {
		return ErrNoExecutor
	}
	if len(in.Plan.Units) == 0 {
		return ErrNoTargets
	}
	if in.Plan.NeedsToken && in.Token == "" && in.TokenFor == nil {
		return ErrTokenRequired
	}
	if in.Plan.NeedsTarball && in.Tarball == "" {
		return ErrTarballRequired
	}
	return nil
}

// applyUnit は 1 台分の手順を順に実行する。
//
// 手順の境界ごとに ctx を見る。着手済みの台での打ち切りは失敗と同じ StepError に
// 包む。こうしておくと Result.Failed / Result.Phase が「どこまで進んだ台か」を
// 失敗時と同じ経路で表せ、呼び出し側は errors.Is で context.Canceled を見分けられる。
func applyUnit(ctx context.Context, in ApplyInput, u Unit, index, total int) error {
	for _, s := range u.Steps {
		if err := ctx.Err(); err != nil {
			return stepFailed(in, u, s, index, total, err)
		}

		notify(in.Progress, Progress{
			Index: index, Total: total, Name: u.Name, Phase: s.Phase, Done: false, Err: nil,
		})

		if err := runStep(ctx, in, u, s); err != nil {
			return stepFailed(in, u, s, index, total, err)
		}
	}
	return nil
}

// stepFailed は手順の失敗（打ち切りを含む）を StepError に包み、進捗にも流す。
func stepFailed(in ApplyInput, u Unit, s Step, index, total int, err error) error {
	werr := &StepError{Unit: u.Name, Phase: s.Phase, Err: err}
	notify(in.Progress, Progress{
		Index: index, Total: total, Name: u.Name, Phase: s.Phase, Done: true, Err: werr,
	})
	return werr
}

// runStep は 1 手順を実行する。
func runStep(ctx context.Context, in ApplyInput, u Unit, s Step) error {
	switch s.Kind {
	case StepMkdir:
		return os.MkdirAll(s.Dir, dirMode)
	case StepExtract:
		return tarball.Extract(ctx, in.Tarball, s.Dir, s.Keep)
	case StepDrain:
		return drainWith(ctx, in, u.Runner)
	case StepCommand:
		return runCommand(ctx, in, u, s)
	default:
		return fmt.Errorf("未知の手順です: %d", s.Kind)
	}
}

// runCommand は外部コマンドを 1 本発行する。
//
// トークンは Args の複製に対してここで初めて差し込む。計画側の Args は
// mask.Placeholder のままなので、プレビューが参照する値は変わらない。
func runCommand(ctx context.Context, in ApplyInput, u Unit, s Step) error {
	args := slices.Clone(s.Args)
	if s.TokenIndex >= 0 && s.TokenIndex < len(args) {
		tok, err := tokenFor(ctx, in, u)
		if err != nil {
			return err
		}
		args[s.TokenIndex] = tok
	}

	ctx = exec.WithOptions(ctx, exec.Options{
		Action: s.Action, Runner: u.Name, Dir: s.Dir, Env: s.Env, SkipAudit: false,
	})

	_, err := in.Exec.Run(ctx, s.Name, args...)
	return err
}

// tokenFor はこの台に使う短命トークンを返す。
func tokenFor(ctx context.Context, in ApplyInput, u Unit) (string, error) {
	if in.TokenFor == nil {
		return in.Token, nil
	}

	tok, err := in.TokenFor(ctx, u)
	if err != nil {
		return "", err
	}
	if tok == "" {
		return "", ErrTokenRequired
	}
	return tok, nil
}

// drainWith はドレイン停止を行う。差し替えが無ければ svc.Drain を使う。
func drainWith(ctx context.Context, in ApplyInput, r runner.Runner) error {
	if in.Drain != nil {
		return in.Drain(ctx, in.Exec, r)
	}
	return svc.Drain(ctx, in.Exec, r, nil)
}

// StepError はどの台のどのフェーズで失敗したかを保つエラー。
type StepError struct {
	// Unit は失敗した runner 名。
	Unit string
	// Phase は失敗したフェーズ名。
	Phase string
	// Err は失敗の理由。
	Err error
}

// Error はエラー文言を返す。
func (e *StepError) Error() string {
	return e.Unit + " の" + e.Phase + "に失敗しました: " + e.Err.Error()
}

// Unwrap は基のエラーを返す。
func (e *StepError) Unwrap() error { return e.Err }

// phaseOf は StepError からフェーズ名を取り出す。
func phaseOf(err error) string {
	var se *StepError
	if errors.As(err, &se) {
		return se.Phase
	}
	return ""
}

// remaining は from 以降の runner 名を返す。
func remaining(units []Unit, from int) []string {
	if from >= len(units) {
		return nil
	}
	out := make([]string, 0, len(units)-from)
	for _, u := range units[from:] {
		out = append(out, u.Name)
	}
	return out
}

// notify は進捗を通知する。nil の場合は何もしない。
func notify(fn func(Progress), p Progress) {
	if fn != nil {
		fn(p)
	}
}
