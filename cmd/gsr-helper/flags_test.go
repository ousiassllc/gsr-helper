package main

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"testing"
	"time"
)

// --root は複数指定でき、指定した順に積まれる。
func TestParseArgsCollectsRoots(t *testing.T) {
	o, err := parseArgs([]string{"--root", "/opt/runners", "--root", "/srv/runners", "-root", "/mnt/r"})
	if err != nil {
		t.Fatalf("解釈に失敗した: %v", err)
	}
	if want := []string{"/opt/runners", "/srv/runners", "/mnt/r"}; !slices.Equal(o.roots, want) {
		t.Errorf("走査ルート = %v, want %v", o.roots, want)
	}
}

// フラグを与えない場合は既定値（未指定）になる。
func TestParseArgsDefaults(t *testing.T) {
	o, err := parseArgs(nil)
	if err != nil {
		t.Fatalf("解釈に失敗した: %v", err)
	}
	if len(o.roots) != 0 || o.config != "" || o.refresh != 0 || o.noColor || o.version {
		t.Errorf("既定値が埋まっている: %+v", o)
	}
}

// --refresh は 1 以上の秒数のみを受け付ける。
func TestParseArgsRefresh(t *testing.T) {
	tests := map[string]struct {
		args    []string
		want    time.Duration
		wantErr bool
	}{
		"未指定は 0（設定ファイルの値を使う）": {nil, 0, false},
		"1 秒":     {[]string{"--refresh", "1"}, time.Second, false},
		"5 秒":     {[]string{"--refresh", "5"}, 5 * time.Second, false},
		"0 は誤り":   {[]string{"--refresh", "0"}, 0, true},
		"負値は誤り":   {[]string{"--refresh", "-3"}, 0, true},
		"秒数以外は誤り": {[]string{"--refresh", "3s"}, 0, true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			o, err := parseArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("誤りを受け付けている: %+v", o)
				}
				return
			}
			if err != nil {
				t.Fatalf("解釈に失敗した: %v", err)
			}
			if o.refresh != tt.want {
				t.Errorf("自動更新間隔 = %v, want %v", o.refresh, tt.want)
			}
		})
	}
}

// --refresh の誤りは flag パッケージの英語の前置を混ぜず、日本語の文言だけを出す。
//
// flag は Set が返したエラーを `invalid value "0" for flag -refresh: ...` と包み直し、
// しかも %v なので errors.As で元のエラーを取り出せない。検証を Parse の後に回して
// いることをここで固定する。
func TestParseArgsRefreshErrorIsJapaneseOnly(t *testing.T) {
	tests := map[string]string{
		"0":  "--refresh には 1 以上の秒数を指定してください: 0",
		"-3": "--refresh には 1 以上の秒数を指定してください: -3",
		"3s": "--refresh には秒数を指定してください: 3s",
	}
	for value, want := range tests {
		t.Run(value, func(t *testing.T) {
			_, err := parseArgs([]string{"--refresh", value})
			if err == nil {
				t.Fatal("誤りを受け付けている")
			}
			if got := err.Error(); got != want {
				t.Errorf("エラー = %q, want %q", got, want)
			}
		})
	}
}

// 不明なフラグ・空の --root・余分な引数は誤りとして扱う。
func TestParseArgsRejectsBadInput(t *testing.T) {
	for name, args := range map[string][]string{
		"不明なフラグ":      {"--nope"},
		"空の走査ルート":     {"--root", ""},
		"余分な位置引数":     {"runners"},
		"値の無い --root": {"--root"},
	} {
		t.Run(name, func(t *testing.T) {
			if o, err := parseArgs(args); err == nil {
				t.Errorf("誤りを受け付けている: %+v", o)
			}
		})
	}
}

// --help は flag.ErrHelp として返し、使い方の出力先は呼び出し側が決める。
func TestParseArgsHelp(t *testing.T) {
	for _, arg := range []string{"--help", "-h"} {
		_, err := parseArgs([]string{arg})
		if !errors.Is(err, flag.ErrHelp) {
			t.Errorf("%s のエラー = %v, want flag.ErrHelp", arg, err)
		}
	}
}

// --version と --no-color と --config は値を素通しする。
func TestParseArgsSimpleFlags(t *testing.T) {
	o, err := parseArgs([]string{"--version", "--no-color", "--config", "/etc/gsr.yaml"})
	if err != nil {
		t.Fatalf("解釈に失敗した: %v", err)
	}
	if !o.version || !o.noColor || o.config != "/etc/gsr.yaml" {
		t.Errorf("フラグが反映されていない: %+v", o)
	}
}

// 色を使うかは --no-color / NO_COLOR / 非 TTY のいずれか 1 つでも該当すれば無効。
func TestColorEnabled(t *testing.T) {
	tests := map[string]struct {
		noColor bool
		env     string
		tty     bool
		want    bool
	}{
		"端末で何も指定しない":        {false, "", true, true},
		"--no-color で無効":    {true, "", true, false},
		"NO_COLOR で無効":      {false, "1", true, false},
		"NO_COLOR が空なら有効":   {false, "", true, true},
		"非 TTY で無効":         {false, "", false, false},
		"非 TTY かつ NO_COLOR": {false, "1", false, false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			getenv := func(k string) string {
				if k != envNoColor {
					t.Errorf("読んだ環境変数 = %q, want %q", k, envNoColor)
				}
				return tt.env
			}
			if got := colorEnabled(tt.noColor, getenv, tt.tty); got != tt.want {
				t.Errorf("色を使うか = %v, want %v", got, tt.want)
			}
		})
	}
}

// 通常ファイル（リダイレクト先）は端末ではない。
func TestIsTerminal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("一時ファイルを作れない: %v", err)
	}
	defer func() { _ = f.Close() }()

	if isTerminal(f) {
		t.Error("通常ファイルを端末と判定している")
	}
}

// バージョン表記は ldflags の値 → ビルド情報 → (unknown) の順に決まる。
func TestResolveVersion(t *testing.T) {
	tests := map[string]struct {
		embedded string
		info     *debug.BuildInfo
		want     string
	}{
		"ldflags の値を優先": {"1.2.3", &debug.BuildInfo{Main: debug.Module{Version: "v0.0.1"}}, "1.2.3"},
		"ビルド情報で補う":      {"", &debug.BuildInfo{Main: debug.Module{Version: "v0.0.1"}}, "v0.0.1"},
		"ビルド情報が無い":      {"", nil, unknownVersion},
		"ビルド情報が空文字":     {"", &debug.BuildInfo{Main: debug.Module{Version: ""}}, unknownVersion},
		// ローカルの go build は (devel) を付ける。バージョンとして意味を持たないため、
		// doc どおり (unknown) に倒す。
		"ローカルの go build": {"", &debug.BuildInfo{Main: debug.Module{Version: develVersion}}, unknownVersion},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := resolveVersion(tt.embedded, tt.info)
			if !strings.HasPrefix(got, appName+" ") {
				t.Errorf("バージョン表記 = %q, ツール名で始まっていない", got)
			}
			if want := appName + " " + tt.want; got != want {
				t.Errorf("バージョン表記 = %q, want %q", got, want)
			}
		})
	}
}

// --version と --help は端末を掌握せずに出力して正常終了する。
func TestRunPrintsVersionAndUsage(t *testing.T) {
	tests := map[string]struct {
		args []string
		want string
	}{
		"--version": {[]string{"--version"}, appName},
		"--help":    {[]string{"--help"}, "使い方"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var out, errOut strings.Builder
			if code := run(tt.args, &out, &errOut); code != exitOK {
				t.Fatalf("終了コード = %d, want %d", code, exitOK)
			}
			if !strings.Contains(out.String(), tt.want) {
				t.Errorf("標準出力 = %q, %q を含まない", out.String(), tt.want)
			}
			if errOut.Len() != 0 {
				t.Errorf("標準エラー出力 = %q, want 空", errOut.String())
			}
		})
	}
}

// 引数の誤りは使い方を添えて終了コード 2 で終わる。
func TestRunRejectsBadArgs(t *testing.T) {
	var out, errOut strings.Builder
	if code := run([]string{"--nope"}, &out, &errOut); code != exitUsage {
		t.Fatalf("終了コード = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut.String(), "使い方") {
		t.Errorf("標準エラー出力 = %q, 使い方を含まない", errOut.String())
	}
}

// 設定ファイルが壊れている場合は端末を掌握せずに終了する。
func TestRunRejectsBrokenConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("scan_depth: [壊れた値]\n"), 0o600); err != nil {
		t.Fatalf("設定ファイルを作れない: %v", err)
	}

	var out, errOut strings.Builder
	if code := run([]string{"--config", path}, &out, &errOut); code != exitError {
		t.Fatalf("終了コード = %d, want %d", code, exitError)
	}
	if errOut.Len() == 0 {
		t.Error("失敗の理由を出していない")
	}
}
