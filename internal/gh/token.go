package gh

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/appconfig/confpath"
	"github.com/ousiassllc/gsr-helper/internal/exec"
)

// EnvToken は最優先で読むトークンの環境変数。
const EnvToken = "GH_TOKEN"

// actionToken はトークン取得コマンドの監査ログ上の action。
const actionToken = "caps.token"

// Source はトークンの取得元。テストで環境と実効 UID を差し替えられるようにしてある。
//
// 取得の優先順は docs/architecture/security.md「取得の優先順」に従う。
//  1. 環境変数 GH_TOKEN
//  2. sudo -u $SUDO_USER gh auth token（SUDO_USER があり実効 UID が 0 のとき）
//  3. gh auth token
//
// 2 番目があるのは、sudo で起動した場合の gh auth token が root の gh 設定を
// 参照してしまい、通常ユーザーで済ませた認証が読めないためである。
type Source struct {
	// Exec は外部コマンドの実行手段。nil の場合は環境変数だけを見る。
	Exec exec.Executor
	// Getenv は環境変数の読み取り。nil なら os.Getenv。
	Getenv func(string) string
	// Geteuid は実効 UID の取得。nil なら os.Geteuid。
	Geteuid func() int
	// LookPath はコマンドの探索。nil なら exec.LookPath。
	//
	// 差し替えられるようにしてあるのは、テストがホストの PATH に依存しない
	// ようにするためである（CI は gh が入っている self-hosted runner なので、
	// 実物を見ると「gh が無い」経路を検証できない）。
	LookPath func(string) (string, error)
	// Timeout は 1 コマンドあたりの上限。0 なら期限を足さず exec の既定に委ねる。
	Timeout time.Duration
}

// Token は優先順に従ってトークンを取得する。
//
// 取得できない場合は ErrNoToken を返す。返した値は呼び出し側がメモリ上でのみ扱い、
// 画面・ログ・監査ログへ出さないこと（docs/architecture/security.md）。
func (s Source) Token(ctx context.Context) (string, error) {
	ctx = ctxOrBackground(ctx)

	if v := strings.TrimSpace(s.getenv(EnvToken)); v != "" {
		return v, nil
	}

	// gh が無いなら失敗すると分かっている実行をしない（無駄な監査ログを残さない）。
	if s.Exec == nil || !s.available("gh") {
		return "", ErrNoToken
	}

	// sudo -u の引数になるため文字種を検証済みの値だけを使う。不正ならこの経路を飛ばす。
	if su := confpath.SudoUserFrom(s.getenv); s.geteuid() == 0 && su != "" {
		if v := s.run(ctx, "sudo", "-u", su, "gh", "auth", "token"); v != "" {
			return v, nil
		}
	}

	if v := s.run(ctx, "gh", "auth", "token"); v != "" {
		return v, nil
	}
	return "", ErrNoToken
}

// getenv は差し替え可能な環境変数の読み取り。
func (s Source) getenv(key string) string {
	if s.Getenv == nil {
		return os.Getenv(key)
	}
	return s.Getenv(key)
}

// geteuid は差し替え可能な実効 UID の取得。
func (s Source) geteuid() int {
	if s.Geteuid == nil {
		return os.Geteuid()
	}
	return s.Geteuid()
}

// available は name が PATH 上にあるかを返す。
func (s Source) available(name string) bool {
	look := s.LookPath
	if look == nil {
		look = exec.LookPath
	}
	_, err := look(name)
	return err == nil
}

// run は 1 本のコマンドを実行し、標準出力を trim して返す。失敗時は空文字。
//
// 標準出力にはトークンそのものが載る。戻り値を呼び出し側がそのまま保持する以外の
// 経路（ログ・エラー文言）へ流さないこと。監査ログに残るのはコマンド名と引数だけである。
func (s Source) run(ctx context.Context, name string, args ...string) string {
	if s.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.Timeout)
		defer cancel()
	}
	ctx = exec.WithOptions(ctx, exec.Options{
		Action: actionToken, Runner: "", Dir: "", Env: nil, SkipAudit: false,
	})

	res, err := s.Exec.Run(ctx, name, args...)
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(string(res.Stdout))
}

// Token は既定の取得元でトークンを取得する。
func Token(ctx context.Context, ex exec.Executor) (string, error) {
	return Source{Exec: ex, Getenv: nil, Geteuid: nil, LookPath: nil, Timeout: 0}.Token(ctx)
}

// HasToken はトークンを取得できるかだけを返す。hostcaps.TokenFunc として使う。
//
// 値そのものは返さない。判定のためだけに取り出したトークンが Caps や画面に
// 載る経路を作らないためである（docs/architecture/security.md「保持と出力」）。
// timeout は 1 コマンドあたりの上限で、起動時の能力判定が予算内に収まるようにする。
func HasToken(ctx context.Context, ex exec.Executor, timeout time.Duration) bool {
	v, err := Source{Exec: ex, Getenv: nil, Geteuid: nil, LookPath: nil, Timeout: timeout}.Token(ctx)
	return err == nil && v != ""
}
