package setup_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// wideBody はコマンド全文が 1 行に収まる幅。既定の 100 桁では切り詰められる。
const wideBody = 160

// 追加は「メニュー → フォーム → 実行前プレビュー」と進み、承認の前に作成する
// ディレクトリとコマンド全文を出す（AC-4 / FR-16）。一括と 1 台ずつで命名の規則が
// 違う（連番か入力そのままか）ため、どちらの入口も辿る。
func TestAddFormShowsDirsAndCommandsBeforeApproval(t *testing.T) {
	t.Parallel()

	const url = "https://github.com/orgs/foo"
	tests := map[string]struct {
		open []tea.Msg // メニューで追加の項目を選ぶまで
		fill []tea.Msg // フォームへの入力
		dir  string
		cmd  string
	}{
		"一括": {
			open: []tea.Msg{pagetest.Press("enter")},
			fill: []tea.Msg{pagetest.Paste(url)},
			dir:  "/opt/runners/build01-2",
			cmd:  "--token *** --name build01-2 --work _work --unattended",
		},
		"1 台ずつ": {
			open: []tea.Msg{pagetest.Press("down"), pagetest.Press("enter")},
			fill: []tea.Msg{pagetest.Paste(url), huh.NextField(), pagetest.Paste("gpu-box")},
			dir:  "/opt/runners/gpu-box",
			cmd:  "--token *** --name gpu-box --work _work --unattended",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			st, api := stateAPI(t, pagetest.SampleRunner())
			st.BodyW = wideBody
			f := fakeOf(t, st)

			m := pagetest.Quick(newModel(t, st), tt.open...)
			if !strings.Contains(view(m), "登録先の URL") {
				t.Fatalf("追加フォームが開いていない:\n%s", view(m))
			}

			m, ok := pagetest.SubmitHuh(pagetest.Quick(m, tt.fill...), 12,
				func(m tea.Model) bool { return strings.Contains(view(m), "追加の確認") })
			if !ok {
				t.Fatalf("実行前プレビューへ進めなかった:\n%s", view(m))
			}

			got := view(m)
			for _, w := range []string{tt.dir, "./config.sh --url " + url + " " + tt.cmd,
				"./svc.sh install", "./svc.sh start"} {
				if !strings.Contains(got, w) {
					t.Errorf("実行前プレビューに %q が無い:\n%s", w, got)
				}
			}
			if len(f.Calls()) != 0 {
				t.Errorf("承認の前にコマンドを発行している: %v", f.Calls())
			}
			// 差し替えが効いていなければ本物の api.github.com を引いている。
			if api.Clients() == 0 {
				t.Error("差し替えた API クライアントが使われていない")
			}
		})
	}
}

// 進捗の見出しに件数を二重に出さない。pane.ProgressList が Done/Total を自分で
// 添える（progresslist.go の header）ため、ページが件数つきの見出しを渡すと
// `削除中… 0/1 0/1` になる。
func TestProgressHeadingShowsCountOnce(t *testing.T) {
	t.Parallel()

	st, _ := stateAPI(t, pagetest.SampleRunner())
	f := fakeOf(t, st)

	// 実行を止めておく。見出しは実行中の描画でしか確かめられない。
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	f.SetFunc(func(string, []string) (exec.Result, error) {
		<-block
		return exec.Result{Stdout: nil, Stderr: nil, ExitCode: 0}, nil
	})

	m := pagetest.Quick(newModel(t, st), removeSample(), pagetest.Press("y"))
	got := view(m)
	if n := strings.Count(got, "0/1"); n != 1 {
		t.Errorf("進捗の見出しの `0/1` の数 = %d, want 1:\n%s", n, got)
	}
	// 状態行には分母を添える相手がいないので、こちらは件数まで含める。
	if status := chromeOf(t, m).Status; status != "削除中… 0/1" {
		t.Errorf("状態行 = %q, want %q", status, "削除中… 0/1")
	}
}

// 実行まで進めても外へは出ない。**このパッケージのどのテストも本物の GitHub を
// 叩かないことの見張りである。** 差し替えが外れると job は gh.Token へ落ち、
// 周囲の GH_TOKEN で remove-token を実際に発行する（page.SetupDeps の doc）。
func TestRunUsesTheInjectedClientOnly(t *testing.T) {
	t.Parallel()

	st, api := stateAPI(t, pagetest.SampleRunner())
	pagetest.Quick(newModel(t, st), removeSample(), pagetest.Press("y"))

	if api.Clients() == 0 {
		t.Fatal("差し替えた API クライアントが使われていない")
	}
	if !strings.Contains(strings.Join(api.Paths(), " "), "remove-token") {
		t.Errorf("短命トークンの発行が模したサーバへ来ていない: %v", api.Paths())
	}
}

// removeSample は標本の runner 1 台の削除を依頼する Msg を返す。
func removeSample() page.SetupRequestMsg {
	return page.SetupRequestMsg{
		Op: page.SetupRemove, Runners: []runner.Runner{pagetest.SampleRunner()},
	}
}
