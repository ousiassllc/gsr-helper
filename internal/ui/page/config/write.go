package config

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/ousiassllc/gsr-helper/internal/config"
	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
)

// ErrNoRunnerID はラベル / runner group の変更に要る runner の ID が分からない
// 場合のエラー。ホスト内の検出だけでは GitHub 側の ID が決まらないため、
// 一覧の取得で名前から引き当てる。
var ErrNoRunnerID = errors.New("GitHub 側の runner が見つかりません")

// commit は承認された変更を書き込む（FR-37 の承認後）。
//
// **呼ぶのは承認を受けた 1 か所だけである。** page/setup と同じく、書き込みへ
// 至る経路を 1 本に絞ることで、確認を経ない破壊的経路を作らない。
func commit(ctx context.Context, in commitInput) error {
	c := in.change

	if c.write != nil {
		if c.path != "" {
			if err := backupIfExists(c.path); err != nil {
				return err
			}
		}
		return c.write()
	}

	return commitAPI(ctx, in)
}

// commitInput は書き込みに要るもの。
type commitInput struct {
	change change
	runner runner.Runner
	// client は GitHub API のクライアントを作る。ラベル / runner group で使う。
	client func(ctx context.Context) (*gh.Client, error)
}

// backupIfExists は書き込み前に退避する（FR-38）。
//
// まだファイルが無い場合は退避せずに進む。新規作成では戻す先が無く、
// 空の .bak を置いても利用者の役に立たない。
func backupIfExists(path string) error {
	err := config.Backup(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("バックアップの作成に失敗しました: %w", err)
	}
	return nil
}

// commitAPI は GitHub 側の値（ラベル / runner group）を変更する。
func commitAPI(ctx context.Context, in commitInput) error {
	cl, err := in.client(ctx)
	if err != nil {
		return fmt.Errorf("GitHub の認証に失敗しました: %w", err)
	}

	sc := in.runner.Scope
	id, err := runnerID(ctx, cl, sc, in.runner.Name())
	if err != nil {
		return err
	}

	if in.change.kind == kindLabels {
		if _, rerr := cl.ReplaceRunnerLabels(ctx, sc, id, in.change.labels); rerr != nil {
			return fmt.Errorf("ラベルの更新に失敗しました: %w", rerr)
		}
		return nil
	}

	if err := cl.AddRunnerToGroup(ctx, sc, in.change.groupID, id); err != nil {
		return fmt.Errorf("runner group の変更に失敗しました: %w", err)
	}
	return nil
}

// runnerID は名前から GitHub 側の runner の ID を引く。
//
// ホスト内の検出は GitHub 側の ID を持たない（.runner の agentId はあるが、
// これは API の runner ID とは別物である）。ラベルの API は ID を要求するため、
// 操作の起点で 1 度だけ一覧を引いて突き合わせる。
func runnerID(ctx context.Context, cl *gh.Client, sc scope.Scope, name string) (int64, error) {
	list, err := cl.ListRunners(ctx, sc)
	if err != nil {
		return 0, fmt.Errorf("runner の一覧の取得に失敗しました: %w", err)
	}

	for _, r := range list {
		if r.Name == name {
			return r.ID, nil
		}
	}
	return 0, fmt.Errorf("%s: %w", name, ErrNoRunnerID)
}

// fetchLabels は現在のラベルを取得する（フォームの初期値に使う）。
func fetchLabels(ctx context.Context, in commitInput) ([]string, error) {
	cl, err := in.client(ctx)
	if err != nil {
		return nil, fmt.Errorf("GitHub の認証に失敗しました: %w", err)
	}

	sc := in.runner.Scope
	id, err := runnerID(ctx, cl, sc, in.runner.Name())
	if err != nil {
		return nil, err
	}

	labels, err := cl.RunnerLabels(ctx, sc, id)
	if err != nil {
		return nil, fmt.Errorf("ラベルの取得に失敗しました: %w", err)
	}
	return labels, nil
}

// fetchGroups は選べる runner group の一覧を取得する。
//
// 名前だけでなく ID も返すのは、付け替えの API が ID を取るためである
// （名前から引き直すともう 1 度一覧を取ることになる）。
func fetchGroups(ctx context.Context, in commitInput) ([]gh.RunnerGroup, error) {
	cl, err := in.client(ctx)
	if err != nil {
		return nil, fmt.Errorf("GitHub の認証に失敗しました: %w", err)
	}

	groups, err := cl.ListRunnerGroups(ctx, in.runner.Scope)
	if err != nil {
		return nil, fmt.Errorf("runner group の一覧の取得に失敗しました: %w", err)
	}
	return groups, nil
}
