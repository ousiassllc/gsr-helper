package gh_test

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
)

// statusHandler は指定のステータスと本文を返すハンドラを作る。
func statusHandler(code int, body string, headers map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}
}

func TestAPIErrorCarriesActionableHint(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		code    int
		body    string
		headers map[string]string
		sc      scope.Scope
		want    []string
	}{
		"401 は gh auth status を促す": {
			code: http.StatusUnauthorized,
			body: `{"message":"Bad credentials"}`,
			sc:   repoScope(),
			want: []string{"gh auth status", "HTTP 401"},
		},
		"403 の権限不足は org なら admin:org を示す": {
			code:    http.StatusForbidden,
			body:    `{"message":"Resource not accessible"}`,
			headers: map[string]string{"X-RateLimit-Remaining": "42"},
			sc:      orgScope(),
			want:    []string{"admin:org", "gh auth refresh"},
		},
		"403 の権限不足は repo なら repo を示す": {
			code:    http.StatusForbidden,
			body:    `{"message":"Resource not accessible"}`,
			headers: map[string]string{"X-RateLimit-Remaining": "42"},
			sc:      repoScope(),
			want:    []string{"必要なスコープは repo です"},
		},
		"403 の権限不足は enterprise なら admin:enterprise を示す": {
			code:    http.StatusForbidden,
			body:    `{"message":"Resource not accessible"}`,
			headers: map[string]string{"X-RateLimit-Remaining": "42"},
			sc:      entScope(),
			want:    []string{"admin:enterprise"},
		},
		"404 はスコープ誤りと権限不足の両方を併記する": {
			code: http.StatusNotFound,
			body: `{"message":"Not Found"}`,
			sc:   repoScope(),
			want: []string{"スコープの指定が誤っている", "隠されています"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, _ := newClient(t, statusHandler(tt.code, tt.body, tt.headers))

			_, err := c.RegistrationToken(context.Background(), tt.sc)
			if err == nil {
				t.Fatal("err = nil, want エラー")
			}

			var apiErr *gh.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("err の型 = %T, want *gh.APIError", err)
			}
			if apiErr.Status != tt.code {
				t.Errorf("Status = %d, want %d", apiErr.Status, tt.code)
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("エラー文言に %q を含むこと:\n%s", w, err.Error())
				}
			}
		})
	}
}

func TestRateLimitHintShowsWaitTime(t *testing.T) {
	t.Parallel()

	reset := time.Now().Add(17 * time.Minute)
	c, _ := newClient(t, statusHandler(
		http.StatusForbidden,
		`{"message":"API rate limit exceeded"}`,
		map[string]string{
			"X-RateLimit-Remaining": "0",
			"X-RateLimit-Limit":     "60",
			"X-RateLimit-Reset":     strconv.FormatInt(reset.Unix(), 10),
		},
	))

	_, err := c.RegistrationToken(context.Background(), repoScope())
	if err == nil {
		t.Fatal("err = nil, want エラー")
	}
	msg := err.Error()
	if !strings.Contains(msg, "レート制限") {
		t.Errorf("レート制限であることを示すこと:\n%s", msg)
	}
	if !strings.Contains(msg, "まで待って再試行") {
		t.Errorf("待機時間を示すこと:\n%s", msg)
	}
	if strings.Contains(msg, "gh auth refresh") {
		t.Errorf("レート制限を権限不足と取り違えている:\n%s", msg)
	}
}

func TestAPIErrorIncludesScopeLabel(t *testing.T) {
	t.Parallel()

	c, _ := newClient(t, statusHandler(http.StatusNotFound, `{"message":"Not Found"}`, nil))

	_, err := c.ListRunners(context.Background(), orgScope())
	if err == nil {
		t.Fatal("err = nil, want エラー")
	}
	if !strings.Contains(err.Error(), "org:foo") {
		t.Errorf("対象スコープを文言に含めること:\n%s", err.Error())
	}
	if !strings.Contains(err.Error(), "list_runners") {
		t.Errorf("失敗した操作を文言に含めること:\n%s", err.Error())
	}
}

func TestSecretsHoldsValuesForMasking(t *testing.T) {
	t.Parallel()

	s := gh.NewSecrets()
	if got := s.Values(); len(got) != 0 {
		t.Errorf("初期値 = %v, want 空", got)
	}

	s.Add("registration-token-value")
	s.Add("")
	if got := s.Values(); len(got) != 1 || got[0] != "registration-token-value" {
		t.Errorf("Values = %v, want 1 件（空文字は無視する）", got)
	}

	s.Forget("registration-token-value")
	if got := s.Values(); len(got) != 0 {
		t.Errorf("Forget 後 = %v, want 空", got)
	}
}

func TestSecretsNilReceiverIsSafe(t *testing.T) {
	t.Parallel()

	var s *gh.Secrets
	s.Add("x")
	s.Forget("x")
	if got := s.Values(); got != nil {
		t.Errorf("Values = %v, want nil", got)
	}
}

func TestSecretsIsConcurrencySafe(t *testing.T) {
	t.Parallel()

	s := gh.NewSecrets()
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(2)
		go func() { defer wg.Done(); s.Add("token-" + strconv.Itoa(i)) }()
		go func() { defer wg.Done(); _ = s.Values() }()
	}
	wg.Wait()

	if got := len(s.Values()); got != 32 {
		t.Errorf("件数 = %d, want 32", got)
	}
}
