package gh

import (
	"context"
	"net/http"
	"strconv"

	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
)

// labelsResponse はラベル系エンドポイントに共通のレスポンス。
// いずれも「操作後のラベル全量」を同じ形で返すため型は 1 つで足りる。
//
// type を読むのは、GitHub が付ける読み取り専用のラベルを名前ではなく種別で
// 見分けるためである（下記 readOnlyLabel）。
type labelsResponse struct {
	Labels []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"labels"`
}

// readOnlyLabel は GitHub が自動で付けるラベルの種別。
//
// **名前で判定しない。** 自動で付くのは self-hosted と OS 名だけでなく
// アーキテクチャ名も含み、その値はホストによって X64 だったり ARM64 だったり
// する（本ツールは arm64 も対象にしている）。名前の一覧を持つと、一覧に無い
// アーキテクチャのホストで読み取り専用のラベルが「カスタムラベル」として
// フォームに出てしまい、置換 API へ送って弾かれる。
const readOnlyLabel = "read-only"

// labelsRequest は置換のリクエスト本文。
type labelsRequest struct {
	Labels []string `json:"labels"`
}

// RunnerLabels は runner に付いているラベルの一覧を取得する。
//
// GET {scope}/actions/runners/{runner_id}/labels（FR-35）。GitHub が自動で付ける
// 読み取り専用のラベルは除いたカスタムラベルだけを返す（readOnlyLabel）。
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

// labelCall はラベル系エンドポイントの共通処理。
//
// メソッド・パス末尾・本文の有無しか違わない。パスの組み立てと wrap を 1 箇所に
// 閉じ、スコープごとの分岐やエラーの整形が散らばるのを防ぐ。
//
// **追加（POST）と個別削除（DELETE）は実装していない。** 設定編集は現在値を
// 取って全量を置き換える形（GET → PUT）で足りており、呼び出し元の無い公開 API は
// 置かないためである（コンポーネント設計）。必要になった Issue が suffix と
// メソッドを変えて足せる形にしてある。
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
		if l.Type == readOnlyLabel {
			continue
		}
		out = append(out, l.Name)
	}
	return out, nil
}

// labelsBody は置換の本文を組み立てる。
//
// nil をそのまま JSON にすると "labels":null になり GitHub が 422 を返すため
// 必ず配列にする。空配列（全部外す）は正当な指定なので通す。
func labelsBody(labels []string) labelsRequest {
	return labelsRequest{Labels: append(make([]string, 0, len(labels)), labels...)}
}
