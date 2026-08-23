package apply_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/config/apply"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/svc"
)

// target は systemd 管理下の runner を 1 台組む。
func target() runner.Runner {
	return runner.Runner{
		Dir: "/opt/runners/build01-1", Config: runner.Config{AgentName: "build01-1"},
		UnitName: "actions.runner.foo.build01-1.service",
	}
}

// commands は Fake が受けた呼び出しを "name arg arg" の並びで返す。
func commands(f *exec.Fake) []string {
	out := make([]string, 0, len(f.Calls()))
	for _, c := range f.Calls() {
		out = append(out, c.Name+" "+strings.Join(c.Args, " "))
	}
	return out
}

// 反映方法ごとに実行されるコマンドが変わること。ここが取り違うと、
// 「反映しない」を選んだのにジョブが中断されるという最悪の誤りになる。
func TestRunPerMethod(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		method    apply.Method
		reload    bool
		wantAny   []string
		wantNone  []string
		wantDrain bool
	}{
		"反映しない": {
			method: apply.None, reload: false,
			wantAny: nil, wantNone: []string{"restart", "start"}, wantDrain: false,
		},
		"強制再起動": {
			method: apply.Force, reload: false,
			wantAny: []string{"restart"}, wantNone: []string{"daemon-reload"}, wantDrain: false,
		},
		"ドレイン再起動": {
			method: apply.Drain, reload: false,
			wantAny: []string{"start"}, wantNone: []string{"restart"}, wantDrain: true,
		},
		"drop-in は daemon-reload を先に行う": {
			method: apply.Force, reload: true,
			wantAny: []string{"daemon-reload", "restart"}, wantNone: nil, wantDrain: false,
		},
		"反映しない場合も daemon-reload は行う": {
			method: apply.None, reload: true,
			wantAny: []string{"daemon-reload"}, wantNone: []string{"restart", "start"}, wantDrain: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fake := exec.NewFake()
			drained := false

			err := apply.Run(context.Background(), apply.Input{
				Exec: fake, Runner: target(), Method: tt.method, Reload: tt.reload,
				Progress: nil,
				Drain: func(context.Context, exec.Executor, runner.Runner, func(svc.Progress)) error {
					drained = true
					return nil
				},
			})
			if err != nil {
				t.Fatalf("Run() でエラー: %v", err)
			}

			got := strings.Join(commands(fake), " | ")
			for _, want := range tt.wantAny {
				if !strings.Contains(got, want) {
					t.Errorf("実行されたコマンドに %q が無い: %s", want, got)
				}
			}
			for _, ng := range tt.wantNone {
				if strings.Contains(got, ng) {
					t.Errorf("実行されたコマンドに %q があってはいけない: %s", ng, got)
				}
			}
			if drained != tt.wantDrain {
				t.Errorf("ドレインの実行 = %v, want %v", drained, tt.wantDrain)
			}
		})
	}
}

// ドレインが中断されたら起動しないこと。svc.Drain は中断時に停止処理を行わないため、
// runner は動き続けている。そこで start を撃つと二重起動を試みることになる。
func TestRunDoesNotStartWhenDrainFails(t *testing.T) {
	t.Parallel()

	fake := exec.NewFake()
	sentinel := errors.New("中断された")

	err := apply.Run(context.Background(), apply.Input{
		Exec: fake, Runner: target(), Method: apply.Drain, Reload: false, Progress: nil,
		Drain: func(context.Context, exec.Executor, runner.Runner, func(svc.Progress)) error {
			return sentinel
		},
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Run() のエラー = %v, want %v を包んだもの", err, sentinel)
	}
	if got := commands(fake); len(got) != 0 {
		t.Errorf("中断後にコマンドが実行された: %v", got)
	}
}

// 既定（ゼロ値）がドレイン再起動であること。既定を取り違えると、
// 選び直さなかった利用者のジョブが中断される。
func TestZeroValueIsDrain(t *testing.T) {
	t.Parallel()

	var m apply.Method
	if m != apply.Drain {
		t.Errorf("ゼロ値 = %v, want Drain", m)
	}
	if got := apply.Methods(); got[0] != apply.Drain {
		t.Errorf("選択肢の先頭 = %v, want Drain", got[0])
	}
	if len(apply.Methods()) != 3 {
		t.Errorf("選択肢の数 = %d, want 3", len(apply.Methods()))
	}
}

// 選択肢の文言が画面仕様のモックと揃っていること。
func TestLabels(t *testing.T) {
	t.Parallel()

	tests := map[apply.Method]string{
		apply.Drain: "ドレイン再起動",
		apply.Force: "強制再起動",
		apply.None:  "反映しない",
	}

	for m, want := range tests {
		if got := m.Label(); !strings.HasPrefix(got, want) {
			t.Errorf("%v.Label() = %q, want %q で始まる文言", m, got, want)
		}
	}
	// 強制再起動だけが警告記号を持つ（モックの書き分け）。
	if !strings.Contains(apply.Force.Label(), "⚠") {
		t.Error("強制再起動の文言に警告記号が無い")
	}
	if strings.Contains(apply.Drain.Label(), "⚠") {
		t.Error("ドレイン再起動の文言に警告記号があってはいけない")
	}
}
