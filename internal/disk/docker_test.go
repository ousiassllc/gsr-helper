package disk

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/exec"
)

// dfOutput は docker system df --format {{json .}} の出力（1 行 1 JSON）。
const dfOutput = `{"Type":"Images","TotalCount":"12","Active":"3","Size":"5.2GB","Reclaimable":"3.1GB (59%)"}
{"Type":"Containers","TotalCount":"4","Active":"1","Size":"0B","Reclaimable":"0B"}
{"Type":"Local Volumes","TotalCount":"2","Active":"0","Size":"1.5MiB","Reclaimable":"1.5MiB (100%)"}
{"Type":"Build Cache","TotalCount":"30","Active":"0","Size":"12.4GB","Reclaimable":"12.4GB"}
`

// wantDFArgs は発行するコマンド列。external-interfaces.md が定める形から
// ずれると docker の版によって出力形式が変わるため、完全一致で固定する。
var wantDFArgs = []string{"system", "df", "--format", "{{json .}}"}

func TestDockerUsage(t *testing.T) {
	f := exec.NewFake()
	f.Push(exec.Result{Stdout: []byte(dfOutput), Stderr: nil, ExitCode: 0}, nil)

	items, err := DockerUsage(context.Background(), f)
	if err != nil {
		t.Fatalf("DockerUsage がエラーを返した: %v", err)
	}

	want := []DockerItem{
		{Type: "Images", Label: "docker / イメージ", Size: 5_200_000_000, Reclaimable: 3_100_000_000},
		{Type: "Containers", Label: "docker / コンテナ", Size: 0, Reclaimable: 0},
		{Type: "Local Volumes", Label: "docker / ボリューム", Size: 1_572_864, Reclaimable: 1_572_864},
		{Type: "Build Cache", Label: "docker / ビルドキャッシュ", Size: 12_400_000_000, Reclaimable: 12_400_000_000},
	}
	if !slices.Equal(items, want) {
		t.Errorf("DockerItem\n got: %+v\nwant: %+v", items, want)
	}

	calls := f.Calls()
	if len(calls) != 1 {
		t.Fatalf("発行コマンド数 = %d, want 1（%v）", len(calls), calls)
	}
	c := calls[0]
	if c.Name != "docker" || !slices.Equal(c.Args, wantDFArgs) {
		t.Errorf("発行コマンド = %q, want docker %v", c.String(), wantDFArgs)
	}
	// 読み取り専用でも監査ログには残す（SkipAudit は使わない）。
	if c.Options.Action != "disk.df" || c.Options.SkipAudit {
		t.Errorf("Options = %+v, want Action=disk.df SkipAudit=false", c.Options)
	}
}

// TestDockerUsageSkipsUnparsableLines は解析できない行を飛ばし、未知の種別でも
// 行を落とさないことを確かめる。docker の版で行が増えても内訳を出せるようにするため。
func TestDockerUsageSkipsUnparsableLines(t *testing.T) {
	f := exec.NewFake()
	f.Push(exec.Result{
		Stdout: []byte("これは JSON ではない\n" +
			`{"Type":"Unknown Kind","Size":"1kB","Reclaimable":"-"}` + "\n\n"),
		Stderr: nil, ExitCode: 0,
	}, nil)

	items, err := DockerUsage(context.Background(), f)
	if err != nil {
		t.Fatalf("DockerUsage がエラーを返した: %v", err)
	}
	want := []DockerItem{{Type: "Unknown Kind", Label: "docker / Unknown Kind", Size: 1000, Reclaimable: 0}}
	if !slices.Equal(items, want) {
		t.Errorf("DockerItem\n got: %+v\nwant: %+v", items, want)
	}
}

func TestDockerUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		res  exec.Result
		err  error
	}{
		{name: "実行エラー", res: exec.Result{Stdout: nil, Stderr: nil, ExitCode: -1}, err: errors.New("docker がありません")},
		{name: "終了コードが 0 でない", res: exec.Result{Stdout: nil, Stderr: []byte("daemon 不応答"), ExitCode: 1}, err: nil},
		{name: "出力が空", res: exec.Result{Stdout: []byte("  \n"), Stderr: nil, ExitCode: 0}, err: nil},
		{name: "解析できる行が無い", res: exec.Result{Stdout: []byte("not json\n"), Stderr: nil, ExitCode: 0}, err: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := exec.NewFake()
			f.Push(tt.res, tt.err)

			if _, err := DockerUsage(context.Background(), f); err == nil {
				t.Fatal("DockerUsage がエラーを返さなかった")
			}
		})
	}
}

func TestParseDockerSize(t *testing.T) {
	tests := map[string]int64{
		"0B":          0,
		"-":           0,
		"":            0,
		"5.2GB":       5_200_000_000,
		"1.5MB":       1_500_000,
		"3.1GB (59%)": 3_100_000_000,
		"1.5MiB":      1_572_864,
		"2KiB":        2048,
		"1kB":         1000,
		"512B":        512,
		"1.2TB":       1_200_000_000_000,
		"1GiB":        1 << 30,
		"未知の表記":       0,
		"-1GB":        0,
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			if got := parseDockerSize(in); got != want {
				t.Errorf("parseDockerSize(%q) = %d, want %d", in, got, want)
			}
		})
	}
}

// TestPruneReclaimable は解放見込みに prune -f が実際に回収する種別だけを数えることを
// 確かめる。イメージとボリュームを足すと、確認ダイアログが実現しない量を約束する
// （--volumes 無しでボリュームは消えず、-a 無しで dangling 以外のイメージも残る）。
func TestPruneReclaimable(t *testing.T) {
	items := []DockerItem{
		{Type: "Images", Label: "docker / イメージ", Size: 5000, Reclaimable: 4000},
		{Type: "Containers", Label: "docker / コンテナ", Size: 300, Reclaimable: 200},
		{Type: "Local Volumes", Label: "docker / ボリューム", Size: 900, Reclaimable: 900},
		{Type: "Build Cache", Label: "docker / ビルドキャッシュ", Size: 700, Reclaimable: 700},
		{Type: "Unknown", Label: "docker / Unknown", Size: 10, Reclaimable: 10},
	}
	if got, want := PruneReclaimable(items), int64(200+700); got != want {
		t.Errorf("PruneReclaimable = %d, want %d", got, want)
	}
	if got := PruneReclaimable(nil); got != 0 {
		t.Errorf("内訳が無いときの PruneReclaimable = %d, want 0", got)
	}
}
