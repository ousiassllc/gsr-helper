package valid_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/setup/valid"
)

func TestName(t *testing.T) {
	t.Parallel()

	existing := []string{"build01-1", "build01-2"}
	tests := map[string]struct {
		name string
		want error
	}{
		"通常":            {"build01-3", nil},
		"英数と - _ . を許す": {"a-b_c.1", nil},
		"空":             {"", valid.ErrEmptyName},
		"先頭が -":         {"-rf", valid.ErrLeadingDash},
		"空白を含む":         {"build 01", valid.ErrBadNameChar},
		"スラッシュを含む":      {"a/b", valid.ErrBadNameChar},
		"シェルのメタ文字を含む":   {"a;rm", valid.ErrBadNameChar},
		"長すぎる":          {string(make([]byte, 0, 65)) + str65(), valid.ErrNameTooLong},
		"既存と重複":         {"build01-1", valid.ErrDuplicateName},
	}

	for label, tt := range tests {
		t.Run(label, func(t *testing.T) {
			t.Parallel()

			err := valid.Name(tt.name, existing)
			if tt.want == nil {
				if err != nil {
					t.Errorf("err = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

// str65 は 65 文字の名前を返す。
func str65() string {
	b := make([]byte, 65)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}

func TestLabels(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in      []string
		want    []string
		wantErr error
	}{
		"trim して返す":         {[]string{" gpu ", "cuda"}, []string{"gpu", "cuda"}, nil},
		"空要素は落とす":           {[]string{"gpu", "  ", ""}, []string{"gpu"}, nil},
		"重複は先勝ちで除く":         {[]string{"gpu", "GPU", "gpu"}, []string{"gpu"}, nil},
		"順序を保つ":             {[]string{"b", "a"}, []string{"b", "a"}, nil},
		"先頭が -":             {[]string{"-rf"}, nil, valid.ErrLeadingDash},
		"予約ラベル self-hosted": {[]string{"self-hosted"}, nil, valid.ErrReservedLabel},
		"予約ラベルは大文字でも弾く":     {[]string{"Linux"}, nil, valid.ErrReservedLabel},
		"予約ラベル x64":         {[]string{"x64"}, nil, valid.ErrReservedLabel},
		"使えない文字":            {[]string{"a b"}, nil, valid.ErrBadLabelChar},
		"カンマは区切り文字なので弾く":    {[]string{"a,b"}, nil, valid.ErrBadLabelChar},
	}

	for label, tt := range tests {
		t.Run(label, func(t *testing.T) {
			t.Parallel()

			got, err := valid.Labels(tt.in)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Labels = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDir(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in      string
		want    string
		wantErr error
	}{
		"絶対パス":         {"/opt/runners", "/opt/runners", nil},
		"末尾のスラッシュは落とす": {"/opt/runners/", "/opt/runners", nil},
		"前後の空白を落とす":    {"  /opt/runners  ", "/opt/runners", nil},
		"相対パス":         {"runners", "", valid.ErrNotAbs},
		"空":            {"", "", valid.ErrNotAbs},
		".. を含む":       {"/opt/../etc", "", valid.ErrHasDotDot},
		"末尾が ..":       {"/opt/runners/..", "", valid.ErrHasDotDot},
	}

	for label, tt := range tests {
		t.Run(label, func(t *testing.T) {
			t.Parallel()

			got, err := valid.Dir("インストール先", tt.in)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if got != tt.want {
				t.Errorf("Dir = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDirErrorNamesTheField(t *testing.T) {
	t.Parallel()

	_, err := valid.Dir("work dir", "relative")
	if err == nil || !strings.Contains(err.Error(), "work dir") {
		t.Errorf("err = %v, 利用者から見た項目名を文言に含めること", err)
	}
}

func TestURL(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in      string
		want    string
		wantErr error
	}{
		"org":           {"https://github.com/orgs/foo", "https://github.com/orgs/foo", nil},
		"repo":          {"https://github.com/foo/bar", "https://github.com/foo/bar", nil},
		"末尾のスラッシュは落とす":  {"https://github.com/foo/bar/", "https://github.com/foo/bar", nil},
		"www も許す":       {"https://www.github.com/foo", "https://www.github.com/foo", nil},
		"http は拒否":      {"http://github.com/foo", "", valid.ErrNotGitHub},
		"GitHub 以外のホスト": {"https://example.test/foo", "", valid.ErrNotGitHub},
		"よく似たホスト":       {"https://github.com.evil.test/foo", "", valid.ErrNotGitHub},
		"スキーム無し":        {"github.com/foo/bar", "", valid.ErrNotGitHub},
		"空":             {"", "", valid.ErrNotGitHub},
		// 認証情報を埋め込んだ URL は config.sh --url と監査ログへ平文で流れる。
		"利用者名とパスワードの埋め込み": {
			"https://x:ghp_AAAABBBBCCCCDDDDEEEEFFFFGGGGHHHH@github.com/orgs/foo", "",
			valid.ErrURLUserInfo,
		},
		"利用者名だけの埋め込み": {"https://ghp_AAAA@github.com/orgs/foo", "", valid.ErrURLUserInfo},
	}

	for label, tt := range tests {
		t.Run(label, func(t *testing.T) {
			t.Parallel()

			got, err := valid.URL(tt.in)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if got != tt.want {
				t.Errorf("URL = %q, want %q", got, tt.want)
			}
		})
	}
}

// 埋め込まれた認証情報を守るための検証が、その認証情報を文言に載せてはならない。
func TestURLErrorDoesNotEchoCredentials(t *testing.T) {
	t.Parallel()

	const secret = "ghp_AAAABBBBCCCCDDDDEEEEFFFFGGGGHHHH"

	_, err := valid.URL("https://x:" + secret + "@github.com/orgs/foo")
	if err == nil {
		t.Fatal("err = nil, want ErrURLUserInfo")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("エラー文言に認証情報が載っている: %s", err)
	}
}

func TestCount(t *testing.T) {
	t.Parallel()

	for _, n := range []int{1, 25, 50} {
		if err := valid.Count(n); err != nil {
			t.Errorf("Count(%d) = %v, want nil", n, err)
		}
	}
	for _, n := range []int{0, -1, 51, 1000} {
		if err := valid.Count(n); !errors.Is(err, valid.ErrBadCount) {
			t.Errorf("Count(%d) = %v, want ErrBadCount", n, err)
		}
	}
}
