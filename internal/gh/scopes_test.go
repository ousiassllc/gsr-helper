package gh_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
)

// 保有スコープは X-OAuth-Scopes から読み取る。
func TestTokenScopesReadsHeader(t *testing.T) {
	t.Parallel()

	var gotPath string
	c, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("X-OAuth-Scopes", "repo, admin:org , workflow")
		w.WriteHeader(http.StatusOK)
	}))

	got, err := c.TokenScopes(context.Background())
	if err != nil {
		t.Fatalf("TokenScopes: %v", err)
	}
	if !got.Classic {
		t.Error("Classic = false, want true（X-OAuth-Scopes が返っている）")
	}
	want := []string{"repo", "admin:org", "workflow"}
	if !slices.Equal(got.Held, want) {
		t.Errorf("Held = %q, want %q", got.Held, want)
	}
	// レート制限を消費しない endpoint を使う。
	if gotPath != "/rate_limit" {
		t.Errorf("パス = %q, want %q", gotPath, "/rate_limit")
	}
}

// ヘッダが無いトークン（fine-grained PAT / GitHub App）は
// 「スコープを持たない」ではなく「スコープという概念が無い」として返す。
func TestTokenScopesWithoutHeaderIsNotClassic(t *testing.T) {
	t.Parallel()

	c, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	got, err := c.TokenScopes(context.Background())
	if err != nil {
		t.Fatalf("TokenScopes: %v", err)
	}
	if got.Classic {
		t.Error("Classic = true, want false（X-OAuth-Scopes が無い）")
	}
	if len(got.Held) != 0 {
		t.Errorf("Held = %q, want 空", got.Held)
	}
}

// スコープを 1 つも持たない classic トークンは Classic が真で Held が空になる。
func TestTokenScopesEmptyHeaderIsClassic(t *testing.T) {
	t.Parallel()

	c, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-OAuth-Scopes", "")
		w.WriteHeader(http.StatusOK)
	}))

	got, err := c.TokenScopes(context.Background())
	if err != nil {
		t.Fatalf("TokenScopes: %v", err)
	}
	if !got.Classic {
		t.Error("Classic = false, want true（空でもヘッダは返っている）")
	}
	if len(got.Held) != 0 {
		t.Errorf("Held = %q, want 空", got.Held)
	}
}

// 失敗は APIError として返る。
func TestTokenScopesWrapsError(t *testing.T) {
	t.Parallel()

	c, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))

	if _, err := c.TokenScopes(context.Background()); err == nil {
		t.Fatal("エラーを返さなかった")
	}
}

// admin:x は write:x / read:x を含む。文字列一致だけで判定しない。
func TestScopesHasFollowsHierarchy(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		held []string
		need string
		want bool
	}{
		"完全一致":                   {held: []string{"repo"}, need: "repo", want: true},
		"admin は write を含む":      {held: []string{"admin:org"}, need: "write:org", want: true},
		"admin は read を含む":       {held: []string{"admin:org"}, need: "read:org", want: true},
		"read は admin を含まない":     {held: []string{"read:org"}, need: "admin:org", want: false},
		"別の対象は含まない":              {held: []string{"admin:org"}, need: "admin:enterprise", want: false},
		"repo は public_repo を含む": {held: []string{"repo"}, need: "public_repo", want: true},
		"不足":                     {held: []string{"repo", "workflow"}, need: "admin:org", want: false},
		"必要なしなら常に真":              {held: nil, need: "", want: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := gh.Scopes{Held: tt.held, Classic: true}
			if got := s.Has(tt.need); got != tt.want {
				t.Errorf("Has(%q) = %v, want %v（保有 %q）", tt.need, got, tt.want, tt.held)
			}
		})
	}
}

// 必要なスコープと登録先の言い換えは API のエラー・診断・表示層で同じ表を使う。
// 1 つのテストに並べているのは**片方だけが埋まった状態を落とす**ためである
// （Issue #98。欠けると `レベルの操作には ... が必要です` と先頭を欠いた文言になる）。
func TestRequiredScopeAndLevelNamePairUp(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		kind      scope.Kind
		want      string
		wantLevel string
	}{
		"repo":       {kind: scope.Repo, want: "repo", wantLevel: "repo"},
		"org":        {kind: scope.Org, want: "admin:org", wantLevel: "org"},
		"enterprise": {kind: scope.Enterprise, want: "admin:enterprise", wantLevel: "enterprise"},
		"判定不能":       {kind: scope.Unknown, want: "", wantLevel: ""},
		// 表に無い Kind（新設したが表へ足していない場合）。両方とも空になり、
		// 塞ぐ側（RequiredScope が空なら塞がない）にも倒れない。
		"表に無い": {kind: scope.Enterprise + 1, want: "", wantLevel: ""},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sc := scope.Scope{Kind: tt.kind, Owner: "o", Repo: "r"}
			got, level := gh.RequiredScope(sc), gh.ScopeLevelName(sc)
			if got != tt.want {
				t.Errorf("RequiredScope = %q, want %q", got, tt.want)
			}
			if level != tt.wantLevel {
				t.Errorf("ScopeLevelName = %q, want %q", level, tt.wantLevel)
			}
			if (got == "") != (level == "") {
				t.Errorf("必要スコープ %q と言い換え %q の片方だけが埋まっている", got, level)
			}
		})
	}
}
