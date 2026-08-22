package appconfig

import (
	"reflect"
	"slices"
	"testing"
)

// withConfig は既定値を土台に 1 か所だけ変えた Config を作る。
func withConfig(mod func(*Config)) Config {
	c := Default()
	mod(&c)
	return c
}

func TestNormalizeFillsAndValidates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      Config
		want    Config
		wantErr bool
	}{
		{name: "既定値はそのまま通る", in: Default(), want: Default()},
		{
			name: "ゼロ値の項目は既定値で埋まる",
			in:   Config{ScanRoots: nil, ScanDepth: 0, RefreshInterval: 0, DiskThresholds: DiskThresholds{}, AuditLog: "", Defaults: Defaults{}},
			want: Default(),
		},
		{
			name: "scan_depth 下限",
			in:   withConfig(func(c *Config) { c.ScanDepth = 1 }),
			want: withConfig(func(c *Config) { c.ScanDepth = 1 }),
		},
		{
			name: "scan_depth 上限",
			in:   withConfig(func(c *Config) { c.ScanDepth = 10 }),
			want: withConfig(func(c *Config) { c.ScanDepth = 10 }),
		},
		{name: "scan_depth 上限超過", in: withConfig(func(c *Config) { c.ScanDepth = 11 }), wantErr: true},
		{name: "scan_depth 負値", in: withConfig(func(c *Config) { c.ScanDepth = -1 }), wantErr: true},
		{
			name: "refresh_interval 上限",
			in:   withConfig(func(c *Config) { c.RefreshInterval = 3600 }),
			want: withConfig(func(c *Config) { c.RefreshInterval = 3600 }),
		},
		{name: "refresh_interval 上限超過", in: withConfig(func(c *Config) { c.RefreshInterval = 3601 }), wantErr: true},
		{name: "refresh_interval 負値", in: withConfig(func(c *Config) { c.RefreshInterval = -1 }), wantErr: true},
		{
			name:    "warn が critical 以上",
			in:      withConfig(func(c *Config) { c.DiskThresholds = DiskThresholds{Warn: 90, Critical: 90} }),
			wantErr: true,
		},
		{
			name:    "warn が範囲外",
			in:      withConfig(func(c *Config) { c.DiskThresholds = DiskThresholds{Warn: 100, Critical: 100} }),
			wantErr: true,
		},
		{
			name:    "critical が範囲外",
			in:      withConfig(func(c *Config) { c.DiskThresholds = DiskThresholds{Warn: 80, Critical: 101} }),
			wantErr: true,
		},
		{
			name: "scan_roots は空要素を除いて重複を落とす",
			in:   withConfig(func(c *Config) { c.ScanRoots = []string{" /opt/runners ", "", "/opt/runners/", "/data/r"} }),
			want: withConfig(func(c *Config) { c.ScanRoots = []string{"/opt/runners", "/data/r"} }),
		},
		{
			name:    "scan_roots の相対パス",
			in:      withConfig(func(c *Config) { c.ScanRoots = []string{"opt/runners"} }),
			wantErr: true,
		},
		{
			name:    "audit_log に .. を含む",
			in:      withConfig(func(c *Config) { c.AuditLog = "/var/log/../log/audit.jsonl" }),
			wantErr: true,
		},
		{
			name:    "install_base の相対パス",
			in:      withConfig(func(c *Config) { c.Defaults.InstallBase = "runners" }),
			wantErr: true,
		},
		{
			name: "labels は trim して重複と空要素を落とす",
			in:   withConfig(func(c *Config) { c.Defaults.Labels = []string{" gpu ", "gpu", "", "cuda"} }),
			want: withConfig(func(c *Config) { c.Defaults.Labels = []string{"gpu", "cuda"} }),
		},
		{
			name:    "labels の - 始まり",
			in:      withConfig(func(c *Config) { c.Defaults.Labels = []string{"--force"} }),
			wantErr: true,
		},
		{
			name:    "name_prefix の - 始まり",
			in:      withConfig(func(c *Config) { c.Defaults.NamePrefix = "-build" }),
			wantErr: true,
		},
		{
			name:    "name_prefix に空白",
			in:      withConfig(func(c *Config) { c.Defaults.NamePrefix = "build 01" }),
			wantErr: true,
		},
		{
			name: "name_prefix は前後の空白を落とす",
			in:   withConfig(func(c *Config) { c.Defaults.NamePrefix = "  build01  " }),
			want: withConfig(func(c *Config) { c.Defaults.NamePrefix = "build01" }),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := normalize(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalize() がエラーを返していない: %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalize() でエラー: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("normalize() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// MergeScanRoots は設定ファイルの scan_roots と --root の重複を入口をまたいで
// 落とす。同じルートが 2 度現れると、同じディレクトリを 2 度走査することになる。
func TestMergeScanRoots(t *testing.T) {
	tests := []struct {
		name       string
		cfg, extra []string
		want       []string
	}{
		{"どちらも空", nil, nil, nil},
		{"設定のみ", []string{"/opt/r"}, nil, []string{"/opt/r"}},
		{"フラグのみ", nil, []string{"/opt/r"}, []string{"/opt/r"}},
		{"設定が先、フラグが後", []string{"/a"}, []string{"/b"}, []string{"/a", "/b"}},
		{"入口をまたいだ重複を落とす", []string{"/a", "/b"}, []string{"/b", "/c"}, []string{"/a", "/b", "/c"}},
		{"同じ入口の重複も落とす", []string{"/a", "/a"}, []string{"/b", "/b"}, []string{"/a", "/b"}},
		{"空要素は落とす", []string{"", "/a"}, []string{""}, []string{"/a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MergeScanRoots(tt.cfg, tt.extra); !slices.Equal(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// 同じ設定について CLI フラグと設定ファイルの有効範囲が一致していること。
// 入口ごとに範囲が違うと、--refresh 86400 は通るのに refresh_interval: 86400 は
// 起動を止めるという食い違いになる。
func TestValidateRefreshRange(t *testing.T) {
	tests := []struct {
		sec     int
		wantErr bool
	}{{0, true}, {-1, true}, {1, false}, {3600, false}, {3601, true}, {86400, true}}
	for _, tt := range tests {
		err := ValidateRefresh("--refresh", tt.sec)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateRefresh(%d) = %v, wantErr %v", tt.sec, err, tt.wantErr)
		}
	}
	if err := ValidateRefresh("--refresh", 0); err == nil || err.Error() != "--refresh は 1〜3600 秒で指定してください: 0" {
		t.Errorf("項目名が文言に入っていない: %v", err)
	}
}

// --root と scan_roots が同じ検査を通ること（絶対パス・.. の禁止）。
func TestCleanScanRoot(t *testing.T) {
	if got, err := CleanScanRoot("--root", " /opt/runners/ "); err != nil || got != "/opt/runners" {
		t.Errorf(`CleanScanRoot(" /opt/runners/ ") = %q, %v, want "/opt/runners", nil`, got, err)
	}
	for _, in := range []string{"runners", "/opt/../etc", ""} {
		if _, err := CleanScanRoot("--root", in); err == nil {
			t.Errorf("CleanScanRoot(%q) が誤りを受け付けている", in)
		}
	}
}
