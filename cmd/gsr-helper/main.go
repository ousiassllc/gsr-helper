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

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/audit"
	"github.com/ousiassllc/gsr-helper/internal/exec/command"
	"github.com/ousiassllc/gsr-helper/internal/gh"
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

	// 設定ファイルが無い場合も appconfig.Load は既定値を返すため、戻り値からは
	// 初回起動を判別できない。Config タブが初回設定ウィザード（FR-41）を出せる
	// よう、ここで有無を確かめて渡す。確認そのものに失敗した場合（権限など）は
	// ウィザードを出さずに既定値で起動する。毎回ウィザードが立ち上がったうえで
	// 書き込みも失敗し続けるより、読み取り専用で使える方がよい。
	exists, err := appconfig.Exists(o.config)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		exists = true
	}
	confPath := configPath(o.config, stderr)

	// 監査記録の失敗は「コマンドの失敗」と混ぜず、専用の通知先で受けて終了後に
	// 報告する。通知先を渡さないと command と audit が stderr へ直接書き、
	// 代替スクリーンの表示を壊す（auditSink のコメント）。
	//
	// **監査ログより先に作る。** 外部コマンドを伴わない破壊的操作の記録
	// （audit.Logger.Report）も同じ受け皿へ流すため、Logger の生成時に渡す。
	sink := &auditSink{}
	defer func() { sink.report(stderr) }()

	lg := openAudit(cfg.AuditLog, stderr, sink.add)
	// クローズの失敗も報告する。バッファに残ったレコードが書けなかった場合が
	// 黙って消えると、監査ログのエラーのうちこれだけが利用者に見えない。
	defer func() { reportAuditClose(lg.Close(), stderr) }()

	// 秘密情報の提供元。runner の追加・削除で取得する短命トークンをここへ預け、
	// 監査ログと ExitError の値一致マスク（exec/mask の段 2）に効かせる。
	// 提供元は Run のたびに呼ばれるため、外部コマンドを起動しない実装である
	// 必要がある（gh.Secrets は保持済みの値を複製して返すだけ）。
	secrets := gh.NewSecrets()
	ex := command.New(secrets.Values, command.WithAudit(lg), command.WithAuditErrorFunc(sink.add))
	caps := appconfig.Detect(
		context.Background(), ex,
		appconfig.Options{HasToken: gh.HasToken, Timeout: 0},
	)

	app := ui.New(cfg, caps, ex, ui.Options{
		Color:      colorEnabled(o.noColor, os.Getenv, isTerminal(os.Stdout)),
		Refresh:    o.refresh,
		Roots:      appconfig.MergeScanRoots(cfg.ScanRoots, o.roots),
		Host:       hostname(),
		Secrets:    secrets,
		ConfigPath: confPath,
		FirstRun:   !exists,
		// 開けなかった場合も Discard が返るので nil にはならない（openAudit）。
		Audit: lg,
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

// configPath は設定ファイルの配置先を解決する。
//
// 空（--config 未指定）なら既定の配置先を引く。UI へ空文字を渡さないのは、
// 差分プレビューの見出しと初回ウィザードの書き込み先に実際のパスを出すためで
// ある。解決に失敗した場合は空のまま渡す。書き込み時に appconfig が同じ決定を
// やり直して同じ失敗を返すので、起動をここで止める理由は無い。
func configPath(configured string, stderr io.Writer) string {
	if configured != "" {
		return configured
	}

	path, err := appconfig.DefaultPath()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return ""
	}
	return path
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
//
// onErr は Report（外部コマンドを伴わない破壊的操作の記録）の書き込み失敗の
// 通知先である。**TUI では必ず渡すこと。** 渡さないと audit が os.Stderr へ
// 直接書き、bubbletea が代替スクリーンを握っている間に画面が壊れる。外部コマンドの
// 記録失敗（command.WithAuditErrorFunc）と同じ受け皿へ流すことで、記録の失敗が
// どちらの経路で起きても終了後に 1 か所でまとめて報告される。
func openAudit(path string, stderr io.Writer, onErr func(error)) *audit.Logger {
	id := audit.WithIdentity(os.Geteuid(), appconfig.SudoUser())
	lg, err := audit.Open(path, id, audit.WithErrorFunc(onErr))
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
