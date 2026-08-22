package systemd

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// テーブルを短く書くためのユニット名。runner パッケージのテストにある同名の
// 定数から、このパッケージで使う分だけを写した。
const (
	u1 = "actions.runner.myorg.host-1.service"
	u2 = "actions.runner.myorg.host-2.service"
)

// fixture は testdata 配下のフィクスチャを読む。
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("フィクスチャ %s の読み込みに失敗しました: %v", name, err)
	}
	return b
}

func TestParseListUnits(t *testing.T) {
	tests := []struct {
		name, file, out string
		want            []string
	}{
		{name: "実機出力", file: "list_units_normal.txt", want: []string{
			"actions.runner.myorg-myrepo.host-1.service", "actions.runner.myorg-myrepo.host-2.service",
		}},
		// 行頭 ● / 空行 / 無関係ユニット / 紛らわしい名前（接頭辞・接尾辞の不一致）。
		{name: "記号付き・無関係ユニット混在", file: "list_units_mixed.txt", want: []string{
			"actions.runner.myorg.failed-1.service", "actions.runner.myorg.host-9.service",
		}},
		{name: "not-found 行", file: "list_units_notfound.txt", want: []string{"actions.runner.myorg.gone.service"}},
		{name: "エスケープ入りのユニット名", file: "list_units_escaped.txt",
			want: []string{`actions.runner.myorg-my\x2drepo.host\x2d1.service`}},
		{name: "空文字"},
		// 1 行に複数一致しても先頭のフィールドだけを採る（description にユニット名が
		// 再掲される not-found 行で二重に数えないため）。
		{name: "1 行に複数一致", out: "actions.runner.a.service loaded active running actions.runner.a.service",
			want: []string{"actions.runner.a.service"}},
	}
	for _, tt := range tests {
		if got := parseListUnits(showOutput(t, tt.file, tt.out)); !slices.Equal(got, tt.want) {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

// showOutput は file が指定されていれば testdata から読み、無ければ out を返す。
func showOutput(t *testing.T, file, out string) string {
	t.Helper()
	if file == "" {
		return out
	}
	return string(fixture(t, file))
}

func TestParseShow(t *testing.T) {
	const arg = "actions.runner.arg.service" // 引数で渡すユニット名
	tests := []struct {
		name, file, out string
		want            State
	}{
		{name: "実機の出力順（MainPID が先頭）", file: "show_active.txt", want: State{Unit: u1,
			Load: "loaded", Active: "active", Sub: "running", FileState: "enabled",
			WorkingDir: "/opt/actions-runner", User: "runner", MainPID: 12345}},
		{name: "キー順シャッフル・空値・MainPID=0", file: "show_shuffled.txt", want: State{Unit: u2,
			Load: "loaded", Active: "inactive", Sub: "dead", FileState: "disabled",
			WorkingDir: "/opt/actions-runner"}},
		{name: "not-found", file: "show_notfound.txt", want: State{
			Unit: "actions.runner.myorg.gone.service", Load: "not-found", Active: "inactive", Sub: "dead"}},
		{name: "MainPID=(unknown) は 0。Id 行が無ければ引数の unit が残る", file: "show_unknown_pid.txt",
			want: State{Unit: arg, Load: "loaded", Active: "activating", Sub: "start"}},
		// WorkingDirectory=-/path の先頭 - を除去する（孤児判定の照合キーがずれる）。
		{name: "WorkingDirectory の先頭 - を除去", file: "show_dash_workdir.txt", want: State{Unit: u2,
			Load: "loaded", Active: "active", Sub: "running", FileState: "enabled",
			WorkingDir: "/opt/actions-runner-3", User: "runner", MainPID: 999}},
		{name: "Id 行が無い場合は引数の unit を使う", file: "show_no_id.txt", want: State{Unit: arg,
			Load: "loaded", Active: "active", Sub: "running", FileState: "enabled",
			WorkingDir: "/opt/actions-runner", User: "root", MainPID: 1}},
		// 改行が CRLF でも値に \r が残らないこと。testdata に置くと改行の自動変換で
		// 壊れうるためコード内の文字列で持つ。
		{name: "CRLF 行末", out: "Id=actions.runner.crlf.service\r\nLoadState=loaded\r\nUser=runner\r\n",
			want: State{Unit: "actions.runner.crlf.service", Load: "loaded", User: "runner"}},
		{name: "値に = を含む", out: "WorkingDirectory=/opt/a=b\n",
			want: State{Unit: arg, WorkingDir: "/opt/a=b"}},
		{name: "空文字", want: State{Unit: arg}},
	}
	for _, tt := range tests {
		if got := parseShow(arg, showOutput(t, tt.file, tt.out)); got != tt.want {
			t.Errorf("%s:\n got %+v\nwant %+v", tt.name, got, tt.want)
		}
	}
}

func TestStateLabel(t *testing.T) {
	tests := []struct {
		name string
		st   State
		want string
	}{
		{"Active が空", State{}, "-"},
		{"Active と Sub が異なる", State{Active: "active", Sub: "running"}, "active/running"},
		{"Active と Sub が同じ", State{Active: "failed", Sub: "failed"}, "failed"},
		{"Sub が空", State{Active: "inactive"}, "inactive"},
		{"停止中", State{Active: "inactive", Sub: "dead"}, "inactive/dead"},
	}
	for _, tt := range tests {
		if got := tt.st.Label(); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}
