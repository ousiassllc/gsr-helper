// Package gh は GitHub REST API へのアクセスと gh CLI からのトークン借用を担う。
//
// 本パッケージだけが GitHub と通信する。ドメイン層（internal/setup など）は
// Client のメソッド越しにしか API を触らず、go-github の型は外へ出さない。
// エンドポイントの一覧は docs/api/external-interfaces.md に定める。
//
// トークンはメモリ上でのみ扱う。ファイルにも画面にも監査ログにも出さない
// （docs/architecture/security.md）。値を持つ型（Token）は String() を
// マスク済みにしてあり、書式指定子で誤って出力しても平文にならない。
package gh

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/go-github/v83/github"

	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
)

// Client は GitHub REST API のクライアント。go-github を包み、スコープ
// （repo / org / enterprise）ごとのパスの違いを内側に閉じ込める。
//
// 呼び出し側は scope.Scope を渡すだけでよく、エンドポイントの選び分けを
// 各所に散らさない。
type Client struct {
	api *github.Client
}

// Option は Client の設定。
type Option func(*settings)

// settings は New が組み立てる設定。
type settings struct {
	httpClient *http.Client
	baseURL    string
}

// WithHTTPClient は API 呼び出しに使う http.Client を差し替える。
//
// 主にテスト（httptest）とプロキシ環境の指定に使う。nil は無視する。
func WithHTTPClient(c *http.Client) Option {
	return func(s *settings) {
		if c != nil {
			s.httpClient = c
		}
	}
}

// WithBaseURL は API のベース URL を差し替える。
//
// 末尾のスラッシュは go-github の要求に合わせて補う。空文字は無視する。
func WithBaseURL(raw string) Option {
	return func(s *settings) {
		if raw != "" {
			s.baseURL = raw
		}
	}
}

// New はトークンを使う Client を作る。
//
// token が空でも Client は作れる（未認証で叩けるエンドポイントがあるため）。
// 権限不足は呼び出し時に APIError として返る。
func New(token string, opts ...Option) (*Client, error) {
	s := settings{httpClient: nil, baseURL: ""}
	for _, o := range opts {
		o(&s)
	}

	api := github.NewClient(s.httpClient)
	if token != "" {
		api = api.WithAuthToken(token)
	}

	if s.baseURL != "" {
		u, err := url.Parse(ensureSlash(s.baseURL))
		if err != nil {
			return nil, &APIError{Op: "base_url", Scope: "", Status: 0, Hint: "", Err: err}
		}
		api.BaseURL = u
	}

	return &Client{api: api}, nil
}

// ensureSlash は go-github の BaseURL 要件（末尾スラッシュ必須）を満たす。
func ensureSlash(raw string) string {
	if strings.HasSuffix(raw, "/") {
		return raw
	}
	return raw + "/"
}

// scopePath は scope に対応する API のパス接頭辞を返す。
//
// docs/api/external-interfaces.md の「使用するエンドポイント」の表に対応する。
// 判定不能なスコープはエラーにする（誤ったパスへ POST しないため）。
func scopePath(sc scope.Scope) (string, error) {
	switch sc.Kind {
	case scope.Repo:
		return "repos/" + sc.Owner + "/" + sc.Repo, nil
	case scope.Org:
		return "orgs/" + sc.Owner, nil
	case scope.Enterprise:
		return "enterprises/" + sc.Owner, nil
	case scope.Unknown:
		return "", ErrUnknownScope
	default:
		return "", ErrUnknownScope
	}
}

// ctxOrBackground は nil の ctx を弾く小さな保険。
func ctxOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
