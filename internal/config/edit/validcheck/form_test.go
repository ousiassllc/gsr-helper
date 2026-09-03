package validcheck

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/config"
	"github.com/ousiassllc/gsr-helper/internal/config/edit"
)

// huh の Validate へ渡る 4 本の入口を、通す値と弾く値で 1 本ずつ縛る（Issue #182）。
// 中身は薄いラッパだが、UI から呼ばれる入口そのものである。どの欄がどの入口に
// 繋がるかは page/configmodal の回帰テストが見る。
//
// ValidateHook の行は**改行を config.ValidateHookPath より先に見る**ことも縛って
// いる——実在しない絶対パスに改行を付けた値は、順序が正しければ ErrNewline、
// 逆なら stat まで進んで ErrHookNotFound になる。
func TestFormValidators(t *testing.T) {
	t.Parallel()

	hook := filepath.Join(t.TempDir(), "hook.sh")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("hook の作成に失敗: %v", err)
	}

	// errAny は「何かで弾くこと」だけを見る行に置く（誰の番号でもない番兵）。
	errAny := errors.New("弾くこと")
	tests := []struct {
		name string
		fn   func(string) error
		in   string
		want error
	}{
		{"line: 1 行は通る", edit.ValidateLine, "/usr/bin:/bin", nil},
		{"line: 改行は弾く", edit.ValidateLine, "a\nb", edit.ErrNewline},
		{"line: 復帰も弾く", edit.ValidateLine, "a\rb", edit.ErrNewline},
		{"hook: 空は「設定しない」", edit.ValidateHook, "", nil},
		{"hook: 実行できる絶対パスは通る", edit.ValidateHook, hook, nil},
		{"hook: 相対パスは弾く", edit.ValidateHook, "relative/hook.sh", errAny},
		{"hook: 実在しないパスは弾く", edit.ValidateHook, hook + ".none", config.ErrHookNotFound},
		{"hook: 改行を先に見る", edit.ValidateHook, "/opt/hooks/started.sh\n", edit.ErrNewline},
		{"label: カンマ区切りは通る", edit.ValidateLabelInput, "gpu, build", nil},
		{"label: 使えない文字は弾く", edit.ValidateLabelInput, "gpu!", errAny},
		{"roots: 空は通る", edit.ValidateRoots, "", nil},
		{"roots: 絶対パスは通る", edit.ValidateRoots, "/opt/runners, /srv/runners", nil},
		{"roots: 相対パスは弾く", edit.ValidateRoots, "relative", errAny},
		{"roots: .. を含むものは弾く", edit.ValidateRoots, "/opt/../etc", errAny},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.fn(tt.in)
			switch {
			case tt.want == nil:
				if err != nil {
					t.Errorf("%q を弾いた: %v", tt.in, err)
				}
			case errors.Is(tt.want, errAny):
				if err == nil {
					t.Errorf("%q を通した", tt.in)
				}
			default:
				if !errors.Is(err, tt.want) {
					t.Errorf("%q = %v, want %v", tt.in, err, tt.want)
				}
			}
		})
	}
}
