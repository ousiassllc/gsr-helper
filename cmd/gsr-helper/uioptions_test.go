package main

import (
	"slices"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/audit"
	"github.com/ousiassllc/gsr-helper/internal/gh"
)

// ui.Options.Roots は --root で渡された値だけで、設定ファイルの scan_roots は混ざらない。
//
// 合成を起動時に畳むと Config タブで保存しても再起動まで効かない（Issue #132）。
// 合成結果ではなく Roots の中身そのものを見るのは、appconfig.MergeScanRoots が冪等な
// ためである。cmd が合成を畳む旧コードへ巻き戻っても、走査に使うルートの集合
// （scanRoots の合成結果）は正しいままに見える。見えなくなるのは「設定ファイルから
// 消した scan_roots だけが再起動まで残る」形の再発であり、合成結果を突き合わせる
// テストでは検出できない。
func TestUIOptionsRootsExcludeConfigScanRoots(t *testing.T) {
	cfg := appconfig.Default()
	cfg.ScanRoots = []string{"/etc/from-config", "/srv/from-config"}

	tests := map[string][]string{
		"--root を渡した": {"/opt/from-flag"},
		"--root が無い":  nil,
	}
	for name, roots := range tests {
		t.Run(name, func(t *testing.T) {
			got := uiOptions(cfg, opts{roots: roots}, false, "", "", false, nil, nil)
			if !slices.Equal(got.Roots, roots) {
				t.Errorf("走査ルート = %v, want %v", got.Roots, roots)
			}
			for _, r := range cfg.ScanRoots {
				if slices.Contains(got.Roots, r) {
					t.Errorf("設定ファイルの scan_roots が混ざっている: %q（走査ルート = %v）", r, got.Roots)
				}
			}
		})
	}
}

// Roots 以外の値は受け取ったまま ui.Options へ載る。
func TestUIOptionsPassesThroughStartupValues(t *testing.T) {
	o := opts{roots: nil, config: "/etc/gsr.yaml", refresh: 5 * time.Second, noColor: false, version: false}
	secrets := gh.NewSecrets()
	lg := audit.Discard()

	got := uiOptions(appconfig.Default(), o, true, "runner-host", "/etc/gsr.yaml", true, secrets, lg)
	if !got.Color || got.Refresh != 5*time.Second || got.Host != "runner-host" {
		t.Errorf("色・更新間隔・ホスト名が反映されていない: %+v", got)
	}
	if got.ConfigPath != "/etc/gsr.yaml" || !got.FirstRun {
		t.Errorf("設定ファイルの配置先と初回起動が反映されていない: %+v", got)
	}
	if got.Secrets != secrets || got.Audit != lg {
		t.Error("秘密情報の提供元と監査ログの記録先が渡されていない")
	}
}
