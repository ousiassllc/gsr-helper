package config_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/config"
	"github.com/ousiassllc/gsr-helper/internal/setup/valid"
)

// ラベルの検証（FR-36）。判定は internal/setup/valid に委ねているので、ここでは
// 「追加のフォームと同じ規則が通っていること」を固定する。規則が分岐すると、
// 追加では通るのに設定編集では弾かれる（またはその逆）状態が生まれる。
func TestValidateLabels(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in      []string
		want    []string
		wantErr error
	}{
		"通常":         {[]string{"gpu", "cuda12"}, []string{"gpu", "cuda12"}, nil},
		"前後の空白を落とす":  {[]string{"  gpu  "}, []string{"gpu"}, nil},
		"重複を除く":      {[]string{"gpu", "GPU"}, []string{"gpu"}, nil},
		"空要素は捨てる":    {[]string{"gpu", "", "  "}, []string{"gpu"}, nil},
		"空の並び":       {nil, []string{}, nil},
		"予約ラベル":      {[]string{"self-hosted"}, nil, valid.ErrReservedLabel},
		"予約ラベル（大文字）": {[]string{"Linux"}, nil, valid.ErrReservedLabel},
		"予約ラベル（x64）": {[]string{"x64"}, nil, valid.ErrReservedLabel},
		"使えない文字":     {[]string{"gpu/1"}, nil, valid.ErrBadLabelChar},
		"先頭がハイフン":    {[]string{"-gpu"}, nil, valid.ErrLeadingDash},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := config.ValidateLabels(tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateLabels(%v) のエラー = %v, want %v", tt.in, err, tt.wantErr)
			}
			if tt.wantErr == nil && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ValidateLabels(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// 絶対パスの検証（FR-36）。呼び出し側の欄の名前がエラー文言に載ること。
//
// **欄の名前を引数で受けるのが要点である。** 以前は work dir 専用の検証を監査ログの
// 欄から呼んでいたため、監査ログを直すよう促す文言に「work dir」と出ていた。
func TestValidateAbsPath(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		field   string
		path    string
		want    string
		wantErr error
	}{
		"通常":          {"監査ログ", "/var/log/gsr-helper/audit.log", "/var/log/gsr-helper/audit.log", nil},
		"前後の空白を落とす":   {"監査ログ", "  /var/log/audit.log  ", "/var/log/audit.log", nil},
		"重なった区切りを整える": {"監査ログ", "/var//log/audit.log", "/var/log/audit.log", nil},
		"空":           {"監査ログ", "", "", valid.ErrNotAbs},
		"相対パス":        {"監査ログ", "var/log/audit.log", "", valid.ErrNotAbs},
		"..を含む":       {"監査ログ", "/var/../etc/audit.log", "", valid.ErrHasDotDot},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := config.ValidateAbsPath(tt.field, tt.path)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateAbsPath(%q, %q) のエラー = %v, want %v", tt.field, tt.path, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ValidateAbsPath(%q, %q) = %q, want %q", tt.field, tt.path, got, tt.want)
			}
			if tt.wantErr != nil && !strings.Contains(err.Error(), tt.field) {
				t.Errorf("エラー文言 = %q, want %q を含む", err.Error(), tt.field)
			}
		})
	}
}

// 予約ラベルを除いた並びが得られること。GitHub の一覧 API は読み取り専用の
// ラベル（self-hosted / Linux / X64）を含む全量を返すが、そのままフォームへ
// 入れると自分の検証（ValidateLabels）に落ちて確定できなくなる。
func TestCustomLabels(t *testing.T) {
	t.Parallel()

	got := config.CustomLabels([]string{"self-hosted", "Linux", "X64", "gpu", "cuda12"})
	want := []string{"gpu", "cuda12"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CustomLabels() = %v, want %v", got, want)
	}
	if _, err := config.ValidateLabels(got); err != nil {
		t.Errorf("除いた並びが ValidateLabels を通らない: %v", err)
	}
}

// job hooks のスクリプトパスの検証。runner がジョブごとにシェルで実行する値
// なので、絶対パス・存在・実行権を書き込む前に確かめる。
func TestValidateHookPath(t *testing.T) {
	t.Parallel()

	statOf := func(mode fs.FileMode, err error) config.HookStat {
		return func(string) (fs.FileMode, error) { return mode, err }
	}
	missing := errors.New("見つからない")

	tests := map[string]struct {
		path    string
		stat    config.HookStat
		want    string
		wantErr error
	}{
		"実行できるスクリプト": {
			"/opt/hooks/cleanup.sh", statOf(0o755, nil), "/opt/hooks/cleanup.sh", nil,
		},
		"空は設定しない扱い":    {"", statOf(0o755, nil), "", nil},
		"空白だけも設定しない扱い": {"   ", statOf(0o755, nil), "", nil},
		"相対パス":         {"hooks/cleanup.sh", statOf(0o755, nil), "", valid.ErrNotAbs},
		"..を含む": {
			"/opt/../hooks/cleanup.sh", statOf(0o755, nil), "", valid.ErrHasDotDot,
		},
		"存在しない": {
			"/opt/hooks/cleanup.sh", statOf(0, missing), "", config.ErrHookNotFound,
		},
		"ディレクトリ": {
			"/opt/hooks", statOf(fs.ModeDir|0o755, nil), "", config.ErrHookNotFound,
		},
		"実行権が無い": {
			"/opt/hooks/cleanup.sh", statOf(0o644, nil), "", config.ErrHookNotExecutable,
		},
		"stat なしは形だけ": {
			"/opt/hooks/cleanup.sh", nil, "/opt/hooks/cleanup.sh", nil,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := config.ValidateHookPath(tt.path, tt.stat)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateHookPath(%q) のエラー = %v, want %v", tt.path, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ValidateHookPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// 既定の HookStat が実在するファイルのモードを返すこと。
func TestStatHook(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "hook.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("スクリプトの作成に失敗: %v", err)
	}

	got, err := config.ValidateHookPath(path, config.StatHook)
	if err != nil || got != path {
		t.Fatalf("ValidateHookPath() = %q, %v, want %q, nil", got, err, path)
	}

	if _, err := config.ValidateHookPath(filepath.Join(t.TempDir(), "none.sh"), config.StatHook); !errors.Is(err, config.ErrHookNotFound) {
		t.Errorf("存在しない場合のエラー = %v, want ErrHookNotFound", err)
	}
}
