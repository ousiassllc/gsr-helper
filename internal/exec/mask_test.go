package exec

import (
	"slices"
	"testing"
)

func TestMaskArgsByKey(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "--token の次の要素をマスクする",
			args: []string{"--url", "https://github.com/orgs/foo", "--token", "ABCDEFGH"},
			want: []string{"--url", "https://github.com/orgs/foo", "--token", "***"},
		},
		{
			name: "--pat もマスクする",
			args: []string{"--pat", "ghp_xxx"},
			want: []string{"--pat", "***"},
		},
		{
			name: "--jitconfig もマスクする",
			args: []string{"--jitconfig", "eyJ..."},
			want: []string{"--jitconfig", "***"},
		},
		{
			name: "大文字のキーもマスクする",
			args: []string{"--Token", "ABCDEFGH"},
			want: []string{"--Token", "***"},
		},
		{
			name: "単一ハイフンのキーもマスクする",
			args: []string{"-token", "ABCDEFGH"},
			want: []string{"-token", "***"},
		},
		{
			name: "= 区切りはキー名を残して値だけ置換する",
			args: []string{"--token=ABCDEFGH"},
			want: []string{"--token=***"},
		},
		{
			name: "= の後ろが空でも置換する",
			args: []string{"--token="},
			want: []string{"--token=***"},
		},
		{
			name: "最終要素がキーでも範囲外参照しない",
			args: []string{"--name", "build01", "--token"},
			want: []string{"--name", "build01", "--token"},
		},
		{
			name: "次がオプションに見えてもマスクする",
			args: []string{"--token", "--name", "build01"},
			want: []string{"--token", "***", "build01"},
		},
		{
			name: "同じキーが複数回あってもすべてマスクする",
			args: []string{"--token", "AAAAAAAA", "--token", "BBBBBBBB"},
			want: []string{"--token", "***", "--token", "***"},
		},
		{
			// gh auth token --hostname github.com の hostname を潰さない。
			name: "ハイフンなしの位置引数は次の要素をマスクしない",
			args: []string{"auth", "token", "--hostname", "github.com"},
			want: []string{"auth", "token", "--hostname", "github.com"},
		},
		{
			// gh secret set NAME のサブコマンドが読めなくならないようにする。
			name: "ハイフンなしのサブコマンド名は次の要素をマスクしない",
			args: []string{"secret", "set", "NAME"},
			want: []string{"secret", "set", "NAME"},
		},
		{
			// --name の値がたまたま token でも、その次の引数は無関係なので残す。
			name: "他のオプションの値がキー名と一致しても次の要素をマスクしない",
			args: []string{"--name", "token", "--labels", "self-hosted"},
			want: []string{"--name", "token", "--labels", "self-hosted"},
		},
		{
			// = 区切りはハイフンなしも拾う。右側が必ず値なので誤爆しない。
			name: "ハイフンなしの env 形式でも値だけ置換する",
			args: []string{"TOKEN=ghs_xxxxxxxx"},
			want: []string{"TOKEN=***"},
		},
		{
			name: "対象外のキーは変更しない",
			args: []string{"--url", "https://github.com/orgs/foo", "--name", "build01", "--labels", "self-hosted,linux"},
			want: []string{"--url", "https://github.com/orgs/foo", "--name", "build01", "--labels", "self-hosted,linux"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaskArgs(tt.args)
			if !slices.Equal(got, tt.want) {
				t.Errorf("MaskArgs() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMaskArgsByValue(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		secrets []string
		want    []string
	}{
		{
			name:    "完全一致を置換する",
			args:    []string{"--url", "https://github.com/orgs/foo", "-x", "supersecrettoken"},
			secrets: []string{"supersecrettoken"},
			want:    []string{"--url", "https://github.com/orgs/foo", "-x", "***"},
		},
		{
			name:    "URL に埋め込まれた値も置換する",
			args:    []string{"https://x-access-token:supersecrettoken@github.com/foo/bar.git"},
			secrets: []string{"supersecrettoken"},
			want:    []string{"https://x-access-token:***@github.com/foo/bar.git"},
		},
		{
			name:    "複数の secret を置換する",
			args:    []string{"aaaaaaaaaa", "bbbbbbbbbb", "cccccccccc"},
			secrets: []string{"aaaaaaaaaa", "cccccccccc"},
			want:    []string{"***", "bbbbbbbbbb", "***"},
		},
		{
			name:    "8 文字未満の secret は無視する",
			args:    []string{"--name", "build01", "abc"},
			secrets: []string{"abc", "build01"},
			want:    []string{"--name", "build01", "abc"},
		},
		{
			name:    "空文字の secret は無視する",
			args:    []string{"--name", "build01"},
			secrets: []string{""},
			want:    []string{"--name", "build01"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaskArgs(tt.args, tt.secrets...)
			if !slices.Equal(got, tt.want) {
				t.Errorf("MaskArgs() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMaskArgsIsIdempotent(t *testing.T) {
	args := []string{"--token", "supersecrettoken", "--pat=supersecrettoken", "--url", "https://u:supersecrettoken@github.com"}
	secrets := []string{"supersecrettoken"}

	once := MaskArgs(args, secrets...)
	twice := MaskArgs(once, secrets...)
	if !slices.Equal(once, twice) {
		t.Errorf("二重適用で結果が変わった\n once: %q\ntwice: %q", once, twice)
	}
	for _, arg := range once {
		if arg == "supersecrettoken" {
			t.Errorf("マスク漏れがある: %q", once)
		}
	}
}

func TestMaskArgsDoesNotModifyInput(t *testing.T) {
	args := []string{"--token", "supersecrettoken"}
	want := slices.Clone(args)

	MaskArgs(args, "supersecrettoken")
	if !slices.Equal(args, want) {
		t.Errorf("入力スライスが変更された: %q, want %q", args, want)
	}
}

func TestCommandMaskArgsUsesSecretsProvider(t *testing.T) {
	c := New(func() []string { return []string{"supersecrettoken"} })

	got := c.MaskArgs([]string{"--url", "https://u:supersecrettoken@github.com", "--token", "AAAAAAAA"})
	want := []string{"--url", "https://u:***@github.com", "--token", "***"}
	if !slices.Equal(got, want) {
		t.Errorf("MaskArgs() = %q, want %q", got, want)
	}

	// provider 未設定でもキー名ベースのマスクは働く。
	if got := New(NoSecrets).MaskArgs([]string{"--token", "AAAAAAAA"}); !slices.Equal(got, []string{"--token", "***"}) {
		t.Errorf("MaskArgs() = %q", got)
	}
}

func TestIsSecretKey(t *testing.T) {
	for _, name := range []string{"--token", "-token", "--Token", "--TOKEN", "pat", "--jitconfig", "--password", "--secret"} {
		if !isSecretKey(name) {
			t.Errorf("isSecretKey(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"--url", "--name", "--labels", "", "--", "--tokens", "--runner-token"} {
		if isSecretKey(name) {
			t.Errorf("isSecretKey(%q) = true, want false", name)
		}
	}
}
