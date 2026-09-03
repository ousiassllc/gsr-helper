package setup_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 追加は「メニュー → フォーム → 実行前プレビュー → 承認」と進み、承認の前に
// 作成するディレクトリとコマンド全文を出す（AC-4 / FR-16）。一括と 1 台ずつで
// 命名の規則が違う（連番か入力そのままか）ため、どちらの入口も辿る。
//
// 台数を打ち替える組を置くのは、AddSpec.Count が画面の入力から来ていることを
// 端から端まで固定するためである。フォームの既定は 1 台なので、既定のまま送る
// 組だけでは「台数の欄を読まずに 1 を渡す」実装でも通ってしまう（AC-2）。
//
// ジョブ実行中の runner を置く組は AC-6 の警告を見張る。本文と実行中の runner 名の
// **両方**を見るのは、計画へ渡す Busy を空にしても本文は出続けるためである。
func TestAddFormShowsDirsAndCommandsBeforeApproval(t *testing.T) {
	t.Parallel()

	const url = "https://github.com/orgs/foo"
	tests := map[string]struct {
		open []tea.Msg // メニューで追加の項目を選ぶまで
		fill []tea.Msg // フォームへの入力
		busy bool      // 共有状態にジョブ実行中の runner を置くか
		dirs []string
		warn []string // 実行前プレビューに求める警告
		cmd  string
	}{
		"一括": {
			open: []tea.Msg{pagetest.Press("enter")},
			fill: []tea.Msg{pagetest.Paste(url)},
			dirs: []string{"/opt/runners/build01-2"},
			cmd:  "--token *** --name build01-2 --work _work --unattended",
		},
		"一括で 2 台": {
			open: []tea.Msg{pagetest.Press("enter")},
			fill: []tea.Msg{
				pagetest.Paste(url), huh.NextField(), pagetest.Backspace(), pagetest.Paste("2"),
			},
			dirs: []string{"/opt/runners/build01-2", "/opt/runners/build01-3"},
			cmd:  "--token *** --name build01-3 --work _work --unattended",
		},
		"ジョブ実行中の runner がある": {
			open: []tea.Msg{pagetest.Press("enter")},
			fill: []tea.Msg{pagetest.Paste(url)},
			busy: true,
			dirs: []string{"/opt/runners/build01-2"},
			warn: []string{"トークンはプロセス引数として渡ります",
				"現在ジョブ実行中の runner: " + pagetest.BusyRunner().Name()},
			cmd: "--token *** --name build01-2 --work _work --unattended",
		},
		"1 台ずつ": {
			open: []tea.Msg{pagetest.Press("down"), pagetest.Press("enter")},
			fill: []tea.Msg{pagetest.Paste(url), huh.NextField(), pagetest.Paste("gpu-box")},
			dirs: []string{"/opt/runners/gpu-box"},
			cmd:  "--token *** --name gpu-box --work _work --unattended",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			r := pagetest.SampleRunner()
			if tt.busy {
				r = pagetest.BusyRunner()
			}
			st, api := pagetest.SetupState(t.Cleanup, r)
			st.BodyW = pagetest.WideBody
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
			want := append([]string{"./config.sh --url " + url + " " + tt.cmd,
				"./svc.sh install", "./svc.sh start"}, tt.dirs...)
			want = append(want, tt.warn...)
			for _, w := range want {
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

			// 承認して初めて tarball の取得へ進む。**取得の差し替え（SetupDeps.Fetch）が
			// 実際に呼ばれることを確かめる唯一の経路である。** ここを通らないと、Fetch を
			// 渡し忘れた実装が本物のダウンロードを始めても気付けない（ErrNoFetchInTests）。
			pagetest.Quick(m, pagetest.Press("y"))
			if api.WaitFetches(3*time.Second) == 0 {
				t.Error("差し替えた Fetch が使われていない（本物の取得経路へ落ちた疑い）")
			}
		})
	}
}

// 進捗の見出しに件数を二重に出さない。pane.ProgressList が Done/Total を自分で
// 添える（progresslist.go の header）ため、ページが件数つきの見出しを渡すと
// `削除中… 0/1 0/1` になる。
func TestProgressHeadingShowsCountOnce(t *testing.T) {
	t.Parallel()

	st, _ := pagetest.SetupState(t.Cleanup, pagetest.SampleRunner())
	f := fakeOf(t, st)

	// 実行を止めておく。見出しは実行中の描画でしか確かめられない。
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	f.SetFunc(func(string, []string) (exec.Result, error) {
		<-block
		return exec.Result{Stdout: nil, Stderr: nil, ExitCode: 0}, nil
	})

	m := pagetest.Quick(newModel(t, st), pagetest.RemoveRequest(pagetest.SampleRunner()), pagetest.Press("y"))
	got := view(m)
	if n := strings.Count(got, "0/1"); n != 1 {
		t.Errorf("進捗の見出しの `0/1` の数 = %d, want 1:\n%s", n, got)
	}
	// 状態行には分母を添える相手がいないので、こちらは件数まで含める。
	if status := chromeOf(t, m).Status; status != "削除中… 0/1" {
		t.Errorf("状態行 = %q, want %q", status, "削除中… 0/1")
	}
}
