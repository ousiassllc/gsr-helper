// Command gsr-helper は Linux + systemd の self-hosted runner ホストを管理する TUI。
//
// このパッケージはロジックを持たない。フラグの解釈・設定の読み込み・能力判定・
// 色を使うかの判定を行い、親 Model を組み立てて tea.Program を起動するだけである
// （components/overview.md の cmd/gsr-helper）。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/audit"
	"github.com/ousiassllc/gsr-helper/internal/exec/command"
	"github.com/ousiassllc/gsr-helper/internal/ui"
)

// 終了コード。
const (
	exitOK    = 0 // 正常終了
	exitError = 1 // 起動または実行の失敗
	exitUsage = 2 // 引数の誤り
	exitPanic = 3 // panic からの復帰（端末の復元は bubbletea が行う）
)

// envNoColor は色を使わないことを指示する環境変数（https://no-color.org）。
const envNoColor = "NO_COLOR"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run は起動処理の本体。終了コードを返す。
//
// os.Exit を main に閉じ込めるのは、defer（監査ログのクローズ）を確実に走らせる
// ためである。
func run(args []string, stdout, stderr io.Writer) int {
	o, err := parseArgs(args)
	switch {
	case errors.Is(err, flag.ErrHelp):
		usage(stdout)
		return exitOK
	case err != nil:
		_, _ = fmt.Fprintln(stderr, err)
		usage(stderr)
		return exitUsage
	case o.version:
		_, _ = fmt.Fprintln(stdout, versionString())
		return exitOK
	}

	cfg, err := appconfig.Load(o.config)
	if err != nil {
		// 設定ファイルの破損は致命的な起動失敗として扱う。端末を掌握する前に
		// メッセージを出して終わる（architecture/overview.md のエラーハンドリング）。
		_, _ = fmt.Fprintln(stderr, err)
		return exitError
	}

	// 設定ファイルが無い場合も appconfig.Load は既定値を返す。初回設定ウィザード
	// （FR-41）は別 Issue の担当なので、この版では既定値で静かに起動する。案内を
	// 状態行に出さないのは、既定値でも全機能が動くため異常ではなく、ウィザードを
	// 実装する Issue でその案内を消す変更が必要になるためである。
	lg := openAudit(cfg.AuditLog, stderr)
	defer func() { _ = lg.Close() }()

	// 秘密情報の提供元は NoSecrets。この版はトークンをメモリに保持しない
	// （GitHub API を使う機能の Issue で、トークンを保持する提供元に差し替える）。
	ex := command.New(command.NoSecrets, command.WithAudit(lg))
	caps := appconfig.Detect(context.Background(), ex, appconfig.Options{HasToken: nil, Timeout: 0})

	app := ui.New(cfg, caps, ex, ui.Options{
		Color:   colorEnabled(o.noColor, os.Getenv, isTerminal(os.Stdout)),
		Refresh: o.refresh,
		Roots:   append(slices.Clone(cfg.ScanRoots), o.roots...),
		Host:    hostname(),
	})
	return runProgram(app, stderr)
}

// runProgram は TUI を実行し、終了コードを返す。
//
// panic 時の端末復元とスタックトレースの出力は bubbletea が行う（Program.Run が
// recover して端末を戻し、debug.Stack を stderr に書いてから ErrProgramPanic を
// 返す）。自前の recover は二重復元になるため書かない。
func runProgram(app ui.App, stderr io.Writer) int {
	if _, err := tea.NewProgram(app).Run(); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		if errors.Is(err, tea.ErrProgramPanic) {
			return exitPanic
		}
		return exitError
	}
	return exitOK
}

// openAudit は監査ログを開く。失敗しても起動は続け、警告だけを出す。
//
// 監査ログが書けないことを致命的にしないのは、非 root で一覧を読むだけの使い方
// （/var/log に書けない）を塞がないためである。記録先が無い場合は Discard に
// 倒すので、呼び出し側は nil 判定を書かずに済む。
//
// uid と sudo_user は明示して渡す。audit の既定は SUDO_USER を検証せずに読むが、
// appconfig は文字種を検証した値を持っているためである（不正な値をそのまま
// 記録すると、監査ログの読み手が実行者を誤って特定しうる）。
func openAudit(path string, stderr io.Writer) *audit.Logger {
	id := audit.WithIdentity(os.Geteuid(), appconfig.SudoUser())
	lg, err := audit.Open(path, id)
	if err != nil {
		// 代替スクリーンへ入る前に出すため、終了後の画面にこの警告が残る。
		_, _ = fmt.Fprintf(stderr, "警告: 監査ログを開けませんでした（記録せずに続行します）: %v\n", err)
		// Discard は何も書かないため、識別子（uid / sudo_user）は渡さない。
		return audit.Discard()
	}
	return lg
}

// colorEnabled は色を使うかを 1 つの値に決める。
//
// 判定を cmd に集めるのは、--no-color と NO_COLOR の合流点を 1 箇所にするためである
// （atomic-design.md の背景の明暗と NO_COLOR）。lipgloss 自身もカラープロファイルを
// 判定するが、本ツールの表示可否はこの値だけで決める。
//
// NO_COLOR は「設定されていて空文字でない」場合に効く（https://no-color.org）。
func colorEnabled(noColor bool, getenv func(string) string, tty bool) bool {
	if noColor || !tty {
		return false
	}
	return getenv(envNoColor) == ""
}

// isTerminal は f が端末かを返す。
//
// golang.org/x/term を使わないのは、依存を増やさずに済むためである。
// 文字デバイスかどうかの判定で、パイプ・リダイレクト（通常ファイル）を区別できる。
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// hostname はヘッダに出すホスト名を返す。取得できない場合は空文字を返す。
func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	return h
}
