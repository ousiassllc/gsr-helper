package gh

import (
	"context"
	"net/http"
	"strconv"

	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
)

// RunnerGroup は org / enterprise に定義された runner group。
//
// 用途は config.sh --runnergroup に渡す名前を一覧から選ばせることだけなので
// （FR-12、FR-35）、ID と Name しか持たない。visibility や default も API は
// 返すが、使う画面が無いうちは載せない。
type RunnerGroup struct {
	ID   int64  // GitHub 側の runner group ID
	Name string // config.sh --runnergroup にそのまま渡せる名前
}

// runnerGroupsPage は runner group 一覧 1 ページ分のレスポンス。
type runnerGroupsPage struct {
	RunnerGroups []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"runner_groups"`
}

// ListRunnerGroups は runner group の一覧を取得する。
//
// GET /orgs/{org}/actions/runner-groups（docs/api/external-interfaces.md）。
// 他のエンドポイントと違い {scope} ではなくパスが固定である点に注意する。
// ページングは ListRunners と同じく最後まで辿る。repo スコープの runner には
// runner group の概念が無く、ErrNoRunnerGroups を返す（送信もしない）。
func (c *Client) ListRunnerGroups(ctx context.Context, sc scope.Scope) ([]RunnerGroup, error) {
	base, err := runnerGroupsPath(sc)
	if err != nil {
		return nil, wrap("list_runner_groups", sc, nil, err)
	}

	out := make([]RunnerGroup, 0)
	for page := 1; page <= maxPages; page++ {
		u := base + "/actions/runner-groups?per_page=100&page=" + strconv.Itoa(page)

		var got runnerGroupsPage
		resp, rerr := c.do(ctx, http.MethodGet, u, &got)
		if rerr != nil {
			return nil, wrap("list_runner_groups", sc, resp, rerr)
		}
		out = append(out, convertRunnerGroups(got)...)

		if resp == nil || resp.NextPage == 0 {
			break
		}
	}
	return out, nil
}

// runnerGroupsPath は runner group 一覧のパス接頭辞を返す。
//
// repo スコープを要求前に弾くのは、repos/... へ GET すると 404 になり
// 「権限不足か指定誤り」という的外れなヒントが出るためである。enterprise は
// /orgs/ ではなく /enterprises/ を叩く。GitHub がそちらで返すためで、/orgs/
// へ投げると必ず 404 になる（外部インタフェース仕様の表を実態に合わせた）。
func runnerGroupsPath(sc scope.Scope) (string, error) {
	switch sc.Kind {
	case scope.Org:
		return "orgs/" + sc.Owner, nil
	case scope.Enterprise:
		return "enterprises/" + sc.Owner, nil
	case scope.Repo:
		return "", ErrNoRunnerGroups
	case scope.Unknown:
		return "", ErrUnknownScope
	default:
		return "", ErrUnknownScope
	}
}

// convertRunnerGroups は API のレスポンスを内部の型に写す。
func convertRunnerGroups(p runnerGroupsPage) []RunnerGroup {
	out := make([]RunnerGroup, 0, len(p.RunnerGroups))
	for _, g := range p.RunnerGroups {
		out = append(out, RunnerGroup{ID: g.ID, Name: g.Name})
	}
	return out
}

// AddRunnerToGroup は登録済みの runner を runner group へ移す（FR-35）。
//
// PUT {scope}/actions/runner-groups/{runner_group_id}/runners/{runner_id}。
// GitHub 側では「group に runner を足す」操作だが、runner が属せる group は
// 1 つなので、結果として付け替えになる。
//
// 一覧（ListRunnerGroups）と同じく org / enterprise 専用で、repo スコープでは
// 要求を送らずに ErrNoRunnerGroups を返す。
func (c *Client) AddRunnerToGroup(ctx context.Context, sc scope.Scope, groupID, runnerID int64) error {
	base, err := runnerGroupsPath(sc)
	if err != nil {
		return wrap("add_runner_to_group", sc, nil, err)
	}

	u := base + "/actions/runner-groups/" + strconv.FormatInt(groupID, 10) +
		"/runners/" + strconv.FormatInt(runnerID, 10)

	resp, err := c.do(ctx, http.MethodPut, u, nil)
	if err != nil {
		return wrap("add_runner_to_group", sc, resp, err)
	}
	return nil
}
