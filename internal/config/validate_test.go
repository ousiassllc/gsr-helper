package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
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

// runner 名の検証（FR-36）。ホスト内の重複を弾くこと。
func TestValidateRunnerName(t *testing.T) {
	t.Parallel()

	existing := []string{"build01-1", "build01-2"}

	tests := map[string]struct {
		in      string
		wantErr error
	}{
		"通常":      {"build01-3", nil},
		"記号を許す":   {"build_01.3-x", nil},
		"空":       {"", valid.ErrEmptyName},
		"ホスト内で重複": {"build01-1", valid.ErrDuplicateName},
		"使えない文字":  {"build 01", valid.ErrBadNameChar},
		"先頭がハイフン": {"-build01", valid.ErrLeadingDash},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := config.ValidateRunnerName(tt.in, existing)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ValidateRunnerName(%q) のエラー = %v, want %v", tt.in, err, tt.wantErr)
			}
		})
	}
}

// work dir の検証（FR-36）。probe を差し替えて書き込み可否と残容量を確かめる。
func TestValidateWorkDir(t *testing.T) {
	t.Parallel()

	const giB = int64(1) << 30

	probeOf := func(writable bool, avail int64) config.WorkDirProbe {
		return func(string) (config.WorkDirInfo, error) {
			return config.WorkDirInfo{Writable: writable, AvailBytes: avail}, nil
		}
	}

	tests := map[string]struct {
		path    string
		minFree int64
		probe   config.WorkDirProbe
		want    string
		wantErr error
	}{
		"通常":           {"/data/_work", 0, probeOf(true, 10*giB), "/data/_work", nil},
		"末尾を整える":       {"/data//_work/", 0, probeOf(true, 10*giB), "/data/_work", nil},
		"相対パス":         {"data/_work", 0, probeOf(true, 10*giB), "", valid.ErrNotAbs},
		"..を含む":        {"/data/../_work", 0, probeOf(true, 10*giB), "", valid.ErrHasDotDot},
		"書き込めない":       {"/data/_work", 0, probeOf(false, 10*giB), "", config.ErrWorkDirNotWritable},
		"残容量が足りない":     {"/data/_work", 0, probeOf(true, 1<<20), "", config.ErrWorkDirLowSpace},
		"閾値を指定して足りる":   {"/data/_work", 1 << 20, probeOf(true, 2<<20), "/data/_work", nil},
		"probe なしは形だけ": {"/data/_work", 0, nil, "/data/_work", nil},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := config.ValidateWorkDir(tt.path, tt.minFree, tt.probe)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateWorkDir(%q) のエラー = %v, want %v", tt.path, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ValidateWorkDir(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// probe がエラーを返したら検証も失敗すること。判定できないことを「通った」に
// 丸めると、書けない場所を work dir に設定できてしまう。
func TestValidateWorkDirPropagatesProbeError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("statfs に失敗")
	probe := func(string) (config.WorkDirInfo, error) {
		return config.WorkDirInfo{Writable: false, AvailBytes: 0}, sentinel
	}

	if _, err := config.ValidateWorkDir("/data/_work", 0, probe); !errors.Is(err, sentinel) {
		t.Errorf("ValidateWorkDir() のエラー = %v, want %v を包んだもの", err, sentinel)
	}
}

// 既定の probe が実在するディレクトリを調べられること。まだ無い work dir では
// 存在する最も近い親を見る（追加時にはまだディレクトリが無いのが普通である）。
func TestProbeWorkDirUsesNearestExistingParent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	got, err := config.ProbeWorkDir(filepath.Join(dir, "not-yet", "_work"))
	if err != nil {
		t.Fatalf("ProbeWorkDir() でエラー: %v", err)
	}
	if !got.Writable {
		t.Error("一時ディレクトリ配下が書き込み不可と判定された")
	}
	if got.AvailBytes <= 0 {
		t.Errorf("残容量 = %d, want 正の値", got.AvailBytes)
	}
}

// 書き込めないディレクトリを書き込み可と判定しないこと。
func TestProbeWorkDirDetectsUnwritable(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 {
		t.Skip("root は書き込み権限の制限を受けないため、この経路は検証できない")
	}

	dir := filepath.Join(t.TempDir(), "ro")
	if err := os.Mkdir(dir, 0o500); err != nil {
		t.Fatalf("ディレクトリの作成に失敗: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	got, err := config.ProbeWorkDir(dir)
	if err != nil {
		t.Fatalf("ProbeWorkDir() でエラー: %v", err)
	}
	if got.Writable {
		t.Error("書き込めないディレクトリを書き込み可と判定した")
	}
}

// 検証を通した一時ファイルを残さないこと。
func TestProbeWorkDirLeavesNoFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if _, err := config.ProbeWorkDir(dir); err != nil {
		t.Fatalf("ProbeWorkDir() でエラー: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() でエラー: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("一時ファイルが残っている: %v", entries)
	}
}
