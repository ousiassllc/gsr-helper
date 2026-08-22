package runner

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
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

	// 既定の走査ルートと実 /proc を外し、この一時ディレクトリだけを入力にする。
	// 実ホストに runner が居ても結果が変わらないようにするためである。
	stubProcs(t, nil)
	res := Discover(context.Background(), Options{
		Roots: []string{base}, SkipDefaultRoots: true, Exec: f,
	})

	got := make([]string, 0, len(res.Runners))
	for _, r := range res.Runners {
		got = append(got, r.Scope.String()+"/"+r.Name()+"/"+r.RunAsUser)
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

	// gitHubUrl 欠落と .runner 破損の 2 件だけ。どちらもどの runner の警告か
	// 分かるようディレクトリを含む。件数・内容とも完全一致で固定する。
	wantWarns := []string{
		noScope + ": gitHubUrl が空です",
		filepath.Join(broken, ".runner") + ": .runner の JSON 解析に失敗しました: unexpected end of JSON input",
	}
	if !slices.Equal(warnStrings(res.Warnings), wantWarns) {
		t.Errorf("Warnings\n got: %q\nwant: %q", warnStrings(res.Warnings), wantWarns)
	}
}

// stubProcs は /proc の走査結果を差し替える。実ホストで runner が動いていても
// Discover の結果が変わらないようにするために使う。
func stubProcs(t *testing.T, running []Process) {
	t.Helper()
	orig := scanProcs
	scanProcs = func() ([]Process, error) { return running, nil }
	t.Cleanup(func() { scanProcs = orig })
}

// warnStrings は警告を文字列にして返す。
func warnStrings(warns []error) []string {
	out := make([]string, 0, len(warns))
	for _, w := range warns {
		out = append(out, w.Error())
	}
	return out
}

// systemd のユニット一覧が取れたかどうかが起動方式の判定まで伝わること。
// 取れていないのに「ユニットが無い」と読み替えると、systemd 管理の runner が
// run.sh 直起動や未稼働として表示される（FR-03 の「ユニットが無く」を確認できて
// いない）。稼働プロセスは実 /proc 由来で作れないため、ここでは一覧の可否が
// managedBy まで届くことを見る。
func TestDiscoverManagedWhenUnitsNotListed(t *testing.T) {
	base := normalizeDir(t.TempDir())
	dir := mkRunner(t, filepath.Join(base, "r1"))
	stubProcs(t, nil)

	// list-units 自体が失敗する Executor。ユニットの有無が分からない。
	listFails := exec.NewFake()
	listFails.SetFunc(func(_ string, _ []string) (exec.Result, error) {
		return exec.Result{ExitCode: 1}, errors.New("systemctl が見つかりません")
	})
	// 一覧は取れて 0 件。ユニットが無いと確認できている。
	noUnits := exec.NewFake()
	noUnits.SetFunc(func(_ string, _ []string) (exec.Result, error) {
		return exec.Result{}, nil
	})
	// 一覧は取れるが 1 ユニットの show が失敗する。一覧自体は取れている。
	showFails := exec.NewFake()
	showFails.SetFunc(func(_ string, args []string) (exec.Result, error) {
		if args[0] == "list-units" {
			return exec.Result{Stdout: []byte(listOutput(orphanA))}, nil
		}
		return exec.Result{ExitCode: 1}, errors.New("ユニットが見つかりません")
	})

	tests := []struct {
		name string
		ex   exec.Executor
		want ManagedBy
	}{
		{"Executor が無い（systemctl が無い環境の縮退）", nil, ManagedUnavailable},
		{"list-units が失敗", listFails, ManagedUnavailable},
		{"list-units が成功しユニット 0 件", noUnits, ManagedUnknown},
		// show の失敗は 1 ユニットの状態が不明なだけ。従来どおりの判定を保つ。
		{"list-units は成功し show が失敗", showFails, ManagedUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := Discover(context.Background(), Options{
				Roots: []string{base}, SkipDefaultRoots: true, Exec: tt.ex,
			})

			got := make([]ManagedBy, 0, len(res.Runners))
			for _, r := range res.Runners {
				got = append(got, r.Managed)
			}
			if len(got) != 1 || got[0] != tt.want {
				t.Errorf("%s の Managed = %v, want [%v]", dir, got, tt.want)
			}
			// show 失敗のユニットは孤児にしない（従来どおり）。
			if len(res.OrphanUnits) != 0 {
				t.Errorf("孤児 = %q, want 0 件", unitNames(res.OrphanUnits))
			}
		})
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

// 稼働したまま runner ディレクトリを削除すると、.runner が読めないので一覧には
// 出せない。黙って消えないよう警告 1 件で報告することを固定する。
func TestMissingProcDirWarnings(t *testing.T) {
	base := normalizeDir(t.TempDir())
	alive := mkRunner(t, filepath.Join(base, "alive"))
	gone := filepath.Join(base, "gone")

	got := missingProcDirWarnings([]Process{
		{PID: 1, Kind: ProcListener, Dir: alive},
		{PID: 2, Kind: ProcListener, Dir: gone},
		{PID: 3, Kind: ProcWorker, Dir: gone}, // 同じディレクトリは 1 件にまとめる
		{PID: 4, Kind: ProcListener, Dir: ""}, // 消えた根拠にならないので報告しない
	})

	want := []string{gone + ": runner ディレクトリが見つかりません。Runner.Listener" +
		"（PID 2）が稼働したままディレクトリが削除された可能性があります"}
	if len(got) != len(want) {
		t.Fatalf("警告 %d 件 (%v), want %d 件", len(got), got, len(want))
	}
	for i := range want {
		if got[i].Error() != want[i] {
			t.Errorf("got %q, want %q", got[i].Error(), want[i])
		}
	}
}
