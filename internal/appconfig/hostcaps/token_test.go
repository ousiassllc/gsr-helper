package hostcaps

import (
	"context"
	"slices"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/appconfig/confpath"
	"github.com/ousiassllc/gsr-helper/internal/exec"
)

func TestDetectGitHubToken(t *testing.T) {
	for _, tt := range []struct {
		name      string
		avail     []string
		env       map[string]string
		euid      int
		fail      []string // この名前のコマンドは失敗させる
		want      bool
		wantCalls []string
	}{
		{
			name:  "GH_TOKEN があればコマンドを出さない",
			avail: []string{"gh"},
			env:   map[string]string{envGHToken: "ghp_dummy"},
			want:  true,
		},
		{
			name:      "sudo 経路（SUDO_USER + root）",
			avail:     []string{"gh"},
			env:       map[string]string{confpath.EnvSudoUser: "ousiass"},
			euid:      0,
			want:      true,
			wantCalls: []string{"sudo -u ousiass gh auth token"},
		},
		{
			name:      "非 root なら直接 gh のみ",
			avail:     []string{"gh"},
			env:       map[string]string{confpath.EnvSudoUser: "ousiass"},
			euid:      1000,
			want:      true,
			wantCalls: []string{"gh auth token"},
		},
		{
			name:      "sudo 経路が失敗したら直接 gh にフォールバック",
			avail:     []string{"gh"},
			env:       map[string]string{confpath.EnvSudoUser: "ousiass"},
			euid:      0,
			fail:      []string{"sudo"},
			want:      true,
			wantCalls: []string{"sudo -u ousiass gh auth token", "gh auth token"},
		},
		{
			name:      "両経路が失敗",
			avail:     []string{"gh"},
			env:       map[string]string{confpath.EnvSudoUser: "ousiass"},
			euid:      0,
			fail:      []string{"sudo", "gh"},
			want:      false,
			wantCalls: []string{"sudo -u ousiass gh auth token", "gh auth token"},
		},
		{
			name:  "SUDO_USER の文字種が不正なら sudo 経路を飛ばす",
			avail: []string{"gh"},
			env:   map[string]string{confpath.EnvSudoUser: "-x"},
			euid:  0,
			want:  true, wantCalls: []string{"gh auth token"},
		},
		{name: "gh が無ければコマンドを出さない", avail: nil, euid: 0, want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := exec.NewFake()
			f.SetFunc(func(name string, _ []string) (exec.Result, error) {
				if slices.Contains(tt.fail, name) {
					return exec.Result{ExitCode: 1}, nil
				}
				return okResult()
			})

			p := testProbes(tt.avail, tt.env, tt.euid)
			got := detect(context.Background(), f, p, testTimeout)
			if got.GitHubToken != tt.want {
				t.Errorf("GitHubToken = %v, want %v", got.GitHubToken, tt.want)
			}
			lines := cmdlines(f)
			if tt.wantCalls == nil && len(lines) != 0 {
				t.Errorf("コマンドを発行している: %v", lines)
			}
			if tt.wantCalls != nil && !slices.Equal(lines, tt.wantCalls) {
				t.Errorf("発行コマンド = %v, want %v", lines, tt.wantCalls)
			}
		})
	}
}
