// Package command は Executor の実プロセス実装。
//
// internal/exec から分離しているのは、プロセス起動・タイムアウト・監査記録という
// 実行の副作用を、契約（exec.Executor / exec.Result / exec.Options）とテスト用の
// Fake から切り離すためである。ドメイン層は契約だけを import すればよく、
// 実装を差し替える経路は cmd/gsr-helper だけになる。
package command

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/audit"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/exec/mask"
)

const (
	// defaultTimeout は WithTimeout が指定されないときのタイムアウト。
	// 応答しないコマンドで UI が固まらないための保険であり、明示指定を推奨する。
	defaultTimeout = 30 * time.Second
	// waitDelay は kill 後に I/O の完了を待つ猶予。子が孫を残した場合でも
	// Run が戻らなくなることを防ぐ。
	waitDelay = 5 * time.Second
)

// Command は実プロセスを起動する Executor。シェルを経由しない。
type Command struct {
	timeout   time.Duration
	auditLog  *audit.Logger
	secretsFn func() []string
	auditErr  func(error)
}

// Option は Command の生成時の設定。
type Option func(*Command)

// WithTimeout は 1 回の実行のタイムアウトを設定する。既定は 30 秒。
// 0 以下は無制限の実行を作ってしまうため無視し、既定を保つ。
func WithTimeout(d time.Duration) Option {
	return func(c *Command) {
		if d > 0 {
			c.timeout = d
		}
	}
}

// WithAudit は監査ログの記録先を設定する。既定は audit.Discard()。
// nil は無視して既定を保つ（記録先を nil にして書き込み時に落ちるのを避ける）。
func WithAudit(lg *audit.Logger) Option {
	return func(c *Command) {
		if lg != nil {
			c.auditLog = lg
		}
	}
}

// WithAuditErrorFunc は監査記録の失敗を通知する先を設定する。
// 渡されるのは *AuditError で、errors.As で基の書き込みエラーまで到達できる。
//
// TUI アプリは必ず設定すること。未設定のときの代替経路は os.Stderr への 1 行出力で、
// bubbletea が代替画面（alternate screen）を握っている間に標準エラー出力へ書くと
// 画面が壊れる。
func WithAuditErrorFunc(fn func(error)) Option {
	return func(c *Command) {
		c.auditErr = fn
	}
}

// NoSecrets は秘密情報を持たないことを明示する提供元。
//
// New の secrets を省略できない引数にしたうえでこれを用意することで、
// 「うっかり渡し忘れた」と「秘密情報が無いと判断した」を呼び出し側のコードで
// 区別できるようにする。
func NoSecrets() []string { return nil }

// New は実行実装を組み立てる。
//
// secrets は値一致マスクに使う秘密情報の提供元。省略できない引数にしているのは、
// 渡し忘れると監査ログとエラー文の値一致マスク（docs/architecture/security.md の
// 段 2）が無言で効かなくなり、キー名ベースでは救えない経路（stderr にエコーされた
// トークン、URL 埋め込み）が漏れるためである。秘密情報を扱わない場合は NoSecrets
// を渡す。
//
// provider は複数の goroutine から呼ばれるため並行安全であること。また provider の
// 中で外部コマンドを実行してはならない（gh auth token の取得は Executor 経由なので
// 無限再帰になる）。メモリ上に保持済みの値だけを返すこと。
//
// nil は NoSecrets と同等に扱う。TUI アプリが読み込むライブラリが起動時に panic
// するのは望ましくないため、異常終了させずに NoSecrets を促す方を選んでいる。
//
// TUI アプリは WithAuditErrorFunc も必ず渡すこと。未設定だと監査記録の失敗が
// os.Stderr へ直接書かれ、bubbletea が代替画面を握っている間は画面が壊れる。
func New(secrets func() []string, opts ...Option) *Command {
	c := &Command{
		timeout:   defaultTimeout,
		auditLog:  audit.Discard(),
		secretsFn: secrets,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// 実装が interface を満たしていることをコンパイル時に確かめる。
var _ exec.Executor = (*Command)(nil)

// secrets は登録された提供元から secret を取り出す。
// New に nil が渡された場合は NoSecrets と同等に扱う。
func (c *Command) secrets() []string {
	if c.secretsFn == nil {
		return nil
	}
	return c.secretsFn()
}

// Run はコマンドを実行し、結果と監査ログを残す。
//
// 返り値のエラーは「コマンド自体の成否」だけを表す。非ゼロ終了では *ExitError を
// 返し（os/exec と同じ流儀）、Result は成否にかかわらず常に埋める。
//
// 監査記録の失敗は返り値には決して混ぜない。記録できなかっただけで、成功した操作を
// 失敗として報告してしまうためである。失敗は *AuditError として
// WithAuditErrorFunc の通知先へ渡し、未設定なら os.Stderr へ 1 行だけ出す
// （黙って消さないため）。
func (c *Command) Run(ctx context.Context, name string, args ...string) (exec.Result, error) {
	o := exec.OptionsFrom(ctx)
	// 値一致マスクに使う値は 1 回の Run につき 1 度だけ取る。provider は並行安全で
	// あることを求められるためロック取得が入り得るし、Run の途中で返り値が変わると
	// ExitError.Args と監査ログの command でマスク結果が食い違う。
	secrets := c.secrets()
	start := time.Now()
	res, err := c.execute(ctx, o, name, args, secrets)

	rec := audit.Record{
		Action:     o.Action,
		Runner:     o.Runner,
		Dir:        o.Dir,
		Command:    append([]string{name}, mask.Args(args, secrets...)...),
		ExitCode:   res.ExitCode,
		DurationMS: time.Since(start).Milliseconds(),
	}
	if err != nil {
		rec.Error = recordedError(err, secrets)
	}

	if werr := c.auditLog.Write(rec); werr != nil {
		c.reportAuditError(werr)
	}
	return res, err
}

// execute は検証とプロセス起動を行う。監査レコードの組み立ては Run が受け持つため、
// 失敗の経路がどれであっても記録が残る。
//
// secrets は Run が 1 度だけ取得した値を受け取る（Run 内でマスク結果を揃えるため）。
func (c *Command) execute(ctx context.Context, o exec.Options, name string, args, secrets []string) (exec.Result, error) {
	if err := validateName(name, o.Dir); err != nil {
		return exec.Result{ExitCode: -1}, err
	}
	if err := validateEnv(o.Env); err != nil {
		return exec.Result{ExitCode: -1}, err
	}

	// 既定のタイムアウトは「1 コマンドあたりの上限」という保険なので、呼び出し側の
	// deadline で上書きせず min を採る。呼び出し側の deadline がより近ければそれを
	// 尊重し、遠ければ既定まで詰める。操作全体に長い deadline を張った呼び出し側が
	// 個々のコマンドの上限まで緩めてしまわないようにするためである。
	if dl, ok := ctx.Deadline(); !ok || time.Until(dl) > c.timeout {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	// 取り込み量に上限を置くのは標準エラー出力だけ。標準出力は呼び出し側が解析する
	// （systemctl show / list-units）ため、切ると解析が黙って壊れる。
	var stdout bytes.Buffer
	stderr := &limitedBuffer{limit: maxStderrCaptureBytes}
	// シェルを経由せず実行ファイルと引数配列を直接渡すため、メタ文字によるコマンド
	// 注入は成立しない。- で始まる値によるオプションインジェクション対策は
	// 入力検証（internal/setup）の責務。
	//nolint:gosec // 上記の理由
	cmd := osexec.CommandContext(ctx, name, args...)
	cmd.Dir = o.Dir
	cmd.Env = append(os.Environ(), o.Env...)
	cmd.Stdout = &stdout
	cmd.Stderr = stderr
	// 標準入力を与えない。対話的なコマンドを待たせるのではなく即 EOF で失敗させる。
	cmd.Stdin = nil
	// 子を新しいプロセスグループのリーダにし、中断時はグループ全体を止める。
	// 既定の Cancel は直接の子しか kill しないため、孫（./config.sh が起動する
	// sudo や systemctl）が runner ディレクトリを掴んだまま孤児として残る。
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return killProcessGroup(cmd.Process) }
	cmd.WaitDelay = waitDelay

	runErr := cmd.Run()
	res := exec.Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if runErr == nil {
		return res, nil
	}

	// 終了コードは *osexec.ExitError からのみ得られる。起動できなかった場合は -1。
	var exitErr *osexec.ExitError
	if errors.As(runErr, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
	} else {
		res.ExitCode = -1
	}

	// タイムアウトとキャンセルは終了コードでは区別できないため ctx から判定する。
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return res, fmt.Errorf("%s の実行がタイムアウトしました: %w", name, runErr)
	case errors.Is(ctx.Err(), context.Canceled):
		return res, fmt.Errorf("%s の実行がキャンセルされました: %w", name, runErr)
	case exitErr != nil:
		return res, &ExitError{
			Name: name,
			Args: mask.Args(args, secrets...),
			Code: res.ExitCode,
			// 画面に出す抜粋は末尾だけ残す。原因は最後の数行に出るためである。
			Stderr: truncateHead(mask.String(stderr.String(), secrets), maxStderrExcerptBytes),
			Err:    runErr,
		}
	default:
		return res, fmt.Errorf("%s の起動に失敗しました: %w", name, runErr)
	}
}

// validateName はコマンド名を検証する。
//
// ./config.sh のような相対パス指定は作業ディレクトリを基準に解決されるため、
// Dir が空だとツール自身のカレントディレクトリにある別のバイナリを起動しかねない。
func validateName(name, dir string) error {
	switch {
	case name == "":
		return errors.New("コマンド名が空です")
	case strings.ContainsRune(name, filepath.Separator) && !filepath.IsAbs(name) && dir == "":
		return fmt.Errorf("相対パスのコマンド %s の実行には作業ディレクトリの指定が必要です", name)
	default:
		return nil
	}
}

// validateEnv は追加環境変数が KEY=VALUE 形式かを検証する。
//
// os/exec は形式の壊れた要素を黙って無視するため、指定した値が効かない事故を
// 起動前に落とす。メッセージに値を含めない（トークンを渡す用途があるため）。
func validateEnv(env []string) error {
	for i, e := range env {
		key, _, ok := strings.Cut(e, "=")
		if !ok || key == "" {
			return fmt.Errorf("%d 番目の環境変数の指定が KEY=VALUE 形式ではありません", i+1)
		}
	}
	return nil
}
