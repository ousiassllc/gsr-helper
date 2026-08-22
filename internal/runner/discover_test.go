package runner

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/exec"
)

const (
	orphanA = "actions.runner.aa.gone.service" // 対応ディレクトリの無いユニット
	orphanZ = "actions.runner.zz.gone.service"
)

// TestDiscover は 3 経路の突き合わせ結果を通しで見る。個々の部品は単体テストで
// 網羅しているので、ここで確かめるのは Runner への配線と全体の決定性である。
func TestDiscover(t *testing.T) {
	base := normalizeDir(t.TempDir())
	// ディレクトリ名の昇順（= 構築順）とスコープ順で並びが変わるようにしてある。
	mkDir(t, filepath.Join(base, "a-svc"), map[string]string{
		".runner":  `{"agentName":"host-1","gitHubUrl":"https://github.com/myorg/myrepo"}`,
		".service": u1 + "\n",
	})
	wdDir := mkDir(t, filepath.Join(base, "b-wd"), map[string]string{
		".runner": `{"agentName":"host-2","gitHubUrl":"https://github.com/orgs/myorg"}`,
	})
	noScope := mkDir(t, filepath.Join(base, "c-noscope"), map[string]string{".runner": `{"agentName":"host-3"}`})
	broken := mkDir(t, filepath.Join(base, "d-broken"), map[string]string{".runner": "{"})

	show := map[string]string{
		u1:      "Id=" + u1 + "\nLoadState=loaded\nActiveState=active\nUser=runner\n",
		u2:      "Id=" + u2 + "\nLoadState=loaded\nActiveState=active\nWorkingDirectory=" + wdDir + "\n",
		orphanA: "Id=" + orphanA + "\nLoadState=loaded\nWorkingDirectory=/nonexistent\n",
		orphanZ: "Id=" + orphanZ + "\nLoadState=loaded\nWorkingDirectory=/nonexistent\n",
	}
	f := exec.NewFake()
	f.SetFunc(func(_ string, args []string) (exec.Result, error) {
		if args[0] == "list-units" {
			// ユニット名の昇順では返さない。Discover が並べ直さないと、
			// 孤児ユニットの順序が list-units の出力順に流れる。
			return exec.Result{Stdout: []byte(listOutput(orphanZ, u2, orphanA, u1))}, nil
		}
		return exec.Result{Stdout: []byte(show[args[1]])}, nil
	})

	res := Discover(context.Background(), Options{Roots: []string{base}, Exec: f})

	// 既定の走査ルート（実ホストの設置場所）も見るため base 配下だけに絞る。
	var got []string
	for _, r := range res.Runners {
		if strings.HasPrefix(r.Dir, base+string(filepath.Separator)) {
			got = append(got, r.Scope.String()+"/"+r.Name()+"/"+r.RunAsUser)
		}
	}
	// スコープ→名前の順。RunAsUser はユニットの User=、未指定なら root、
	// ユニットが無ければ空（Listener が居ないため）。
	want := []string{"-/host-3/", "myorg/myrepo/host-1/runner", "org:myorg/host-2/root"}
	if !slices.Equal(got, want) {
		t.Errorf("Runners\n got: %q\nwant: %q", got, want)
	}
	if names, wantNames := unitNames(res.OrphanUnits), []string{orphanA, orphanZ}; !slices.Equal(names, wantNames) {
		t.Errorf("OrphanUnits = %q, want %q", names, wantNames)
	}

	// gitHubUrl 欠落と .runner 破損の 2 件。どちらもどの runner の警告か分かる
	// ようディレクトリを含む（実ホスト由来の警告は base で絞って除く）。
	var warns []string
	for _, w := range res.Warnings {
		if strings.Contains(w.Error(), base) {
			warns = append(warns, w.Error())
		}
	}
	for _, dir := range []string{noScope, broken} {
		if !slices.ContainsFunc(warns, func(w string) bool { return strings.Contains(w, dir) }) {
			t.Errorf("Warnings = %q, want %s を含む警告", warns, dir)
		}
	}
	if len(warns) != 2 {
		t.Errorf("Warnings = %q, want 2 件", warns)
	}
}

// listOutput は systemctl list-units の出力を組み立てる。
// systemd パッケージのテストにある同名ヘルパの写し。Discover に渡す Fake の
// 出力を作るためだけに使うので、テスト用ヘルパを公開して共有はしない。
func listOutput(units ...string) string {
	var out string
	for _, u := range units {
		out += u + " loaded active running GitHub Actions Runner\n"
	}
	return out
}
