package gh

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
)

// labelsResponse はラベル系 4 エンドポイントに共通のレスポンス。
// 4 つとも「操作後のラベル全量」を同じ形で返すため型は 1 つで足りる。
type labelsResponse struct {
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

// labelsRequest は置換・追加のリクエスト本文。
type labelsRequest struct {
	Labels []string `json:"labels"`
}

// RunnerLabels は runner に付いているラベルの一覧を取得する。
//
// GET {scope}/actions/runners/{runner_id}/labels（FR-35）。予約ラベルを含む全量。
func (c *Client) RunnerLabels(ctx context.Context, sc scope.Scope, runnerID int64) ([]string, error) {
	return c.labelCall(ctx, sc, "runner_labels", http.MethodGet, runnerID, "", nil)
}

// ReplaceRunnerLabels は runner のカスタムラベルを labels で置き換える。
//
// PUT {scope}/actions/runners/{runner_id}/labels（FR-35）。置換なので残したい
// ラベルも含めた全量を渡す。予約ラベルを弾く検証は internal/setup/valid が持つ
// （本パッケージは通信だけを持つ）。戻り値は置換後のラベル全量。
func (c *Client) ReplaceRunnerLabels(
	ctx context.Context, sc scope.Scope, runnerID int64, labels []string,
) ([]string, error) {
	return c.labelCall(ctx, sc, "replace_runner_labels", http.MethodPut, runnerID, "", labelsBody(labels))
}

// AddRunnerLabels は runner に labels を追加する。既存のラベルは残る。
//
// POST {scope}/actions/runners/{runner_id}/labels（FR-35）。戻り値は追加後の全量。
func (c *Client) AddRunnerLabels(
	ctx context.Context, sc scope.Scope, runnerID int64, labels []string,
) ([]string, error) {
	return c.labelCall(ctx, sc, "add_runner_labels", http.MethodPost, runnerID, "", labelsBody(labels))
}

// RemoveRunnerLabel は runner からラベルを 1 つ外す。
//
// DELETE {scope}/actions/runners/{runner_id}/labels/{name}（FR-35）。name は
// パス要素になるためエスケープする。戻り値は削除後のラベル全量。
func (c *Client) RemoveRunnerLabel(
	ctx context.Context, sc scope.Scope, runnerID int64, name string,
) ([]string, error) {
	suffix := "/" + url.PathEscape(name)
	return c.labelCall(ctx, sc, "remove_runner_label", http.MethodDelete, runnerID, suffix, nil)
}

// labelCall はラベル系 4 エンドポイントの共通処理。
//
// 4 つはメソッド・パス末尾・本文の有無しか違わない。パスの組み立てと wrap を
// 1 箇所に閉じ、スコープごとの分岐やエラーの整形が散らばるのを防ぐ。
func (c *Client) labelCall(
	ctx context.Context, sc scope.Scope, op, method string, runnerID int64, suffix string, body any,
) ([]string, error) {
	base, err := scopePath(sc)
	if err != nil {
		return nil, wrap(op, sc, nil, err)
	}

	u := base + "/actions/runners/" + strconv.FormatInt(runnerID, 10) + "/labels" + suffix

	var got labelsResponse
	resp, err := c.doJSON(ctx, method, u, body, &got)
	if err != nil {
		return nil, wrap(op, sc, resp, err)
	}

	out := make([]string, 0, len(got.Labels))
	for _, l := range got.Labels {
		out = append(out, l.Name)
	}
	return out, nil
}

// labelsBody は置換・追加の本文を組み立てる。
//
// nil をそのまま JSON にすると "labels":null になり GitHub が 422 を返すため
// 必ず配列にする。空配列（全部外す）は正当な指定なので通す。
func labelsBody(labels []string) labelsRequest {
	return labelsRequest{Labels: append(make([]string, 0, len(labels)), labels...)}
}
