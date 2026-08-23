package gh

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/google/go-github/v83/github"

	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
)

// ShortToken は短命トークン（registration token / remove token）。
//
// 既定 1 時間で失効するが、有効な間は runner の登録・解除ができる強い権限を持つ
// （docs/architecture/security.md）。
//
// String() をマスク済みにしてあるのは、%v や %s で誤って出力しても平文にならない
// ようにするためである。値が要る箇所は Value を明示的に読む。
type ShortToken struct {
	Value     string
	ExpiresAt time.Time
}

// String はマスク済みの表現を返す。書式指定子経由の漏洩を防ぐ保険である。
func (t ShortToken) String() string { return "gh.ShortToken(***)" }

// Valid は now の時点でまだ使えるかを返す。
//
// 一括追加では 1 つのトークンを台数分の登録に使い回す（FR-14）。その判定に使う。
// 期限が不明（ゼロ値）の場合は使い回さない方に倒す。
func (t ShortToken) Valid(now time.Time) bool {
	if t.Value == "" || t.ExpiresAt.IsZero() {
		return false
	}
	return now.Before(t.ExpiresAt.Add(-tokenMargin))
}

// tokenMargin は期限切れ間際のトークンを使い回さないための余裕。
//
// 登録は 1 台あたり数十秒かかりうるため、残りがこれ未満なら取り直す。
const tokenMargin = 5 * time.Minute

// Runner は GitHub 側が把握している runner の登録情報。
type Runner struct {
	ID     int64
	Name   string
	OS     string
	Status string
	Busy   bool
	Labels []string
}

// tokenResponse は registration / remove token のレスポンス。
type tokenResponse struct {
	Token     string            `json:"token"`
	ExpiresAt *github.Timestamp `json:"expires_at"`
}

// RegistrationToken は runner を登録するための短命トークンを取得する。
//
// POST {scope}/actions/runners/registration-token（FR-14）
func (c *Client) RegistrationToken(ctx context.Context, sc scope.Scope) (ShortToken, error) {
	return c.shortLivedToken(ctx, sc, "registration-token", "registration_token")
}

// RemoveToken は runner の登録を解除するための短命トークンを取得する。
//
// POST {scope}/actions/runners/remove-token（FR-17）
func (c *Client) RemoveToken(ctx context.Context, sc scope.Scope) (ShortToken, error) {
	return c.shortLivedToken(ctx, sc, "remove-token", "remove_token")
}

// shortLivedToken は 2 つの短命トークン取得の共通処理。
//
// go-github の型付きメソッドを使わないのは、enterprise スコープの remove token に
// 対応するメソッドが無く、スコープごとに経路が割れてしまうためである。パスの
// 組み立てを scopePath 1 箇所に閉じ、3 スコープを同じ流れで扱う。
func (c *Client) shortLivedToken(ctx context.Context, sc scope.Scope, path, op string) (ShortToken, error) {
	base, err := scopePath(sc)
	if err != nil {
		return ShortToken{Value: "", ExpiresAt: time.Time{}}, wrap(op, sc, nil, err)
	}

	var out tokenResponse
	resp, err := c.do(ctx, http.MethodPost, base+"/actions/runners/"+path, &out)
	if err != nil {
		return ShortToken{Value: "", ExpiresAt: time.Time{}}, wrap(op, sc, resp, err)
	}

	tok := ShortToken{Value: out.Token, ExpiresAt: time.Time{}}
	if out.ExpiresAt != nil {
		tok.ExpiresAt = out.ExpiresAt.Time
	}
	return tok, nil
}

// runnersPage は runner 一覧 1 ページ分のレスポンス。
type runnersPage struct {
	TotalCount int `json:"total_count"`
	Runners    []struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		OS     string `json:"os"`
		Status string `json:"status"`
		Busy   bool   `json:"busy"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	} `json:"runners"`
}

// ListRunners は登録済み runner の一覧を取得する。
//
// GET {scope}/actions/runners。ページングは最後まで辿る。
func (c *Client) ListRunners(ctx context.Context, sc scope.Scope) ([]Runner, error) {
	base, err := scopePath(sc)
	if err != nil {
		return nil, wrap("list_runners", sc, nil, err)
	}

	out := make([]Runner, 0)
	for page := 1; page <= maxPages; page++ {
		u := base + "/actions/runners?per_page=100&page=" + strconv.Itoa(page)

		var got runnersPage
		resp, rerr := c.do(ctx, http.MethodGet, u, &got)
		if rerr != nil {
			return nil, wrap("list_runners", sc, resp, rerr)
		}
		out = append(out, convertRunners(got)...)

		if resp == nil || resp.NextPage == 0 {
			break
		}
	}
	return out, nil
}

// maxPages は一覧取得で辿るページ数の上限。
//
// 1 ページ 100 件なので 10000 台まで届く。想定は 1 ホスト 20 台程度（非機能要件）
// だが、org / enterprise スコープの一覧はホスト外の runner も含むため余裕を取る。
// 上限を設けるのは、NextPage の解釈を誤ったときに無限ループさせないためである。
const maxPages = 100

// convertRunners は API のレスポンスを内部の型に写す。
func convertRunners(p runnersPage) []Runner {
	out := make([]Runner, 0, len(p.Runners))
	for _, r := range p.Runners {
		labels := make([]string, 0, len(r.Labels))
		for _, l := range r.Labels {
			labels = append(labels, l.Name)
		}
		out = append(out, Runner{
			ID: r.ID, Name: r.Name, OS: r.OS, Status: r.Status, Busy: r.Busy, Labels: labels,
		})
	}
	return out
}

// DeleteRunner は GitHub 側の登録を削除する。
//
// DELETE {scope}/actions/runners/{runner_id}。config.sh remove が使えない場合の
// 代替であり、通常の削除は config.sh remove を使う（FR-17）。
func (c *Client) DeleteRunner(ctx context.Context, sc scope.Scope, id int64) error {
	base, err := scopePath(sc)
	if err != nil {
		return wrap("delete_runner", sc, nil, err)
	}

	u := base + "/actions/runners/" + strconv.FormatInt(id, 10)
	resp, err := c.do(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return wrap("delete_runner", sc, resp, err)
	}
	return nil
}

// do はリクエスト本文を伴わない API 呼び出しを行う。out が nil ならレスポンス本文を読み捨てる。
func (c *Client) do(ctx context.Context, method, path string, out any) (*github.Response, error) {
	return c.doJSON(ctx, method, path, nil, out)
}

// doJSON は 1 本の API 呼び出しを行う。body は JSON にして送る（nil なら
// 本文を付けない）。out が nil ならレスポンス本文を読み捨てる。組み立てを
// ここ 1 箇所に集め、認証ヘッダとベース URL の解決を go-github に一元化する。
func (c *Client) doJSON(
	ctx context.Context, method, path string, body, out any,
) (*github.Response, error) {
	req, err := c.api.NewRequest(method, path, body)
	if err != nil {
		return nil, err
	}

	resp, err := c.api.Do(ctxOrBackground(ctx), req, out)
	if err != nil {
		return resp, err
	}
	return resp, nil
}
