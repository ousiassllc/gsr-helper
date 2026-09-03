package main

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
)

// opts は起動時のフラグの値。
type opts struct {
	roots   []string
	config  string
	refresh time.Duration // 0 なら未指定（設定ファイルの値を使う）
	noColor bool
	version bool
}

// parseArgs はコマンドライン引数を解釈する。
//
// 純粋関数にしてある（環境変数もファイルも読まず、出力もしない）。フラグの解釈と
// 検証を単体でテストできる状態に保つためである。使い方の表示は呼び出し側が
// usage で行う。
func parseArgs(args []string) (opts, error) {
	var o opts
	fs, roots, refresh := newFlagSet(&o)
	if err := fs.Parse(args); err != nil {
		// flag.ErrHelp（-h / --help）も含めてそのまま返す。使い方をどこへ出すかは
		// 呼び出し側が決める（正常終了か引数の誤りかで出力先が変わる）。
		return opts{}, fmt.Errorf("引数の解釈に失敗しました: %w", err)
	}
	if rest := fs.Args(); len(rest) > 0 {
		return opts{}, fmt.Errorf("不明な引数です: %s", strings.Join(rest, " "))
	}

	refreshDuration, err := refresh.duration()
	if err != nil {
		return opts{}, err
	}
	rootPaths, err := roots.paths()
	if err != nil {
		return opts{}, err
	}

	o.roots = rootPaths
	o.refresh = refreshDuration
	return o, nil
}

// newFlagSet はフラグの定義を組み立てる。
//
// 使い方の表示（usage）と解釈（parseArgs）で同じ定義を使うため、組み立てを 1 箇所に
// 置く。定義が 2 箇所に分かれると、フラグを足したときに説明文が片方だけになる。
func newFlagSet(o *opts) (*flag.FlagSet, *rootList, *seconds) {
	fs := flag.NewFlagSet(appName, flag.ContinueOnError)
	// 出力を捨てるのは、parseArgs を純粋関数に保つためである。
	fs.SetOutput(io.Discard)

	roots := &rootList{}
	refresh := &seconds{raw: ""}
	fs.Var(roots, "root", "追加の走査ルート（複数指定可）")
	fs.StringVar(&o.config, "config", "", "設定ファイルのパス")
	fs.Var(refresh, "refresh", "自動更新間隔（秒。1〜3600）")
	fs.BoolVar(&o.noColor, "no-color", false, "色を使わない（NO_COLOR も尊重する）")
	fs.BoolVar(&o.version, "version", false, "バージョンを表示して終了する")
	return fs, roots, refresh
}

// usage は使い方を w に出力する。
func usage(w io.Writer) {
	var o opts
	fs, _, _ := newFlagSet(&o)
	fs.SetOutput(w)
	_, _ = fmt.Fprintf(w, "使い方: %s [オプション]\n\n", appName)
	_, _ = fmt.Fprintf(w, "Linux + systemd の self-hosted runner ホストを管理する TUI。\n\n")
	_, _ = fmt.Fprintf(w, "オプション:\n")
	fs.PrintDefaults()
}

// rootList は --root の複数指定を集める。
type rootList []string

// String は現在の値を返す（flag.Value の実装）。
func (r *rootList) String() string {
	if r == nil {
		return ""
	}
	return strings.Join(*r, ",")
}

// Set は値を記録するだけで検証しない。検証は paths が行う。
// 検証を Parse の後へ回す理由は seconds.Set と同じ（英語の前置が混ざる）。
func (r *rootList) Set(v string) error {
	*r = append(*r, v)
	return nil
}

// paths は指定された走査ルートを検証して返す。
//
// 検証は設定ファイルの scan_roots と同じ appconfig.CleanScanRoot を通す。
// 絶対パスと .. の検査には安全上の根拠があり（[セキュリティ設計]の削除パスの検証）、
// --root から迂回できてはならない。
func (r *rootList) paths() ([]string, error) {
	if r == nil {
		return nil, nil
	}
	out := make([]string, 0, len(*r))
	for _, v := range *r {
		if strings.TrimSpace(v) == "" {
			return nil, fmt.Errorf("--root に空のパスは指定できません")
		}
		p, err := appconfig.CleanScanRoot("--root", v)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// seconds は --refresh に指定された文字列。
//
// 独自の型にするのは、未指定（設定ファイルの値を使う）と 0 秒の指定（誤り）を
// 区別するためである。flag.IntVar の既定値ではこの 2 つが同じ値になる。
type seconds struct {
	raw string // 指定された生の文字列。未指定なら空
}

// String は指定された文字列を返す（flag.Value の実装）。
func (s *seconds) String() string {
	if s == nil {
		return ""
	}
	return s.raw
}

// Set は値を記録するだけで検証しない。検証は duration が行う。
//
// ここで誤りを返すと flag パッケージが
// `invalid value "0" for flag -refresh: ...` と英語で包み直すため、利用者向けの
// 日本語の文言に英語の前置が混ざる。しかも包み直しは %v なので errors.As で
// 元のエラーを取り出せない。そこで検証を Parse の後（parseArgs）へ回す。
func (s *seconds) Set(v string) error {
	s.raw = v
	return nil
}

// duration は指定された値を検証して自動更新間隔を返す。未指定なら 0 を返す。
//
// 0 以下を認めないのは、間隔 0 が「検出を止める」でも「常時検出」でもなく、Tick が
// 無限に発火して UI を占有する値になるためである。上限を設けるのは、設定ファイルの
// refresh_interval と有効範囲を揃えるためである（appconfig.ValidateRefresh）。
func (s *seconds) duration() (time.Duration, error) {
	if s.raw == "" {
		return 0, nil
	}

	n, err := strconv.Atoi(strings.TrimSpace(s.raw))
	if err != nil {
		return 0, fmt.Errorf("--refresh には秒数を指定してください: %s", s.raw)
	}
	// 有効範囲は設定ファイルの refresh_interval と同じ 1 箇所を通す。入口ごとに
	// 範囲が違うと、--refresh では通る値が設定ファイルでは起動を止めることになる。
	if err := appconfig.ValidateRefresh("--refresh", n); err != nil {
		return 0, err
	}
	return time.Duration(n) * time.Second, nil
}
