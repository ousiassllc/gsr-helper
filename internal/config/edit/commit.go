package edit

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

// ErrNotAPIChange は GitHub API で反映できない種類の変更を API 経路へ渡した
// 場合のエラー。ファイルを書く変更は Change.write を持つのでここへは来ない。
var ErrNotAPIChange = errors.New("GitHub API で反映できる変更ではありません")

// Commit は承認された変更を書き込む（FR-37 の承認後）。
//
// **呼ぶのは承認を受けた 1 か所だけである。** page/setup と同じく、書き込みへ
// 至る経路を 1 本に絞ることで、確認を経ない破壊的経路を作らない。
func Commit(ctx context.Context, in CommitInput) error {
	if in.Change.write != nil {
		return in.Change.Write()
	}

	return commitAPI(ctx, in)
}

// CommitInput は書き込みに要るもの。
type CommitInput struct {
	// Change は書き込む変更。
	Change Change
	// Runner は対象。
	Runner runner.Runner
	// Client は GitHub API のクライアントを作る。ラベル / runner group で使う。
	// テストでは httptest のサーバへ向けたクライアントを返す。
	Client func(ctx context.Context) (*gh.Client, error)
}

// Write は退避してからファイルへ書き込む（FR-38）。
//
// GitHub 側の変更（ラベル / runner group）では何もしない。呼び出し側は Commit を
// 通すこと。これを公開しているのは、書き込みだけを端末なしで検証できるように
// するためである。
func (c Change) Write() error {
	if c.write == nil {
		return nil
	}
	if c.path != "" {
		if err := backupIfExists(c.path); err != nil {
			return err
		}
	}
	return c.write()
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
//
// **扱える種類を先に絞る。** 「ラベルでなければ runner group」とだけ書くと、
// 想定外の Kind（ファイルを書くはずの変更や Change のゼロ値）が groupID 0 のまま
// 付け替えの API へ落ちる。書き込み経路の既定を破壊的な側に倒さない。判定を通信の
// 前に置くのは、そもそも GitHub を叩かないためである。
func commitAPI(ctx context.Context, in CommitInput) error {
	if in.Change.Kind != KindLabels && in.Change.Kind != KindGroup {
		return fmt.Errorf("kind=%d: %w", int(in.Change.Kind), ErrNotAPIChange)
	}

	cl, err := in.Client(ctx)
	if err != nil {
		return fmt.Errorf("GitHub の認証に失敗しました: %w", err)
	}

	sc := in.Runner.Scope
	id, err := runnerID(ctx, cl, sc, in.Runner.Name())
	if err != nil {
		return err
	}

	if in.Change.Kind == KindLabels {
		if _, rerr := cl.ReplaceRunnerLabels(ctx, sc, id, in.Change.labels); rerr != nil {
			return fmt.Errorf("ラベルの更新に失敗しました: %w", rerr)
		}
		return nil
	}

	if err := cl.AddRunnerToGroup(ctx, sc, in.Change.groupID, id); err != nil {
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

// FetchLabels は現在のラベルを取得する（フォームの初期値に使う）。
func FetchLabels(ctx context.Context, in CommitInput) ([]string, error) {
	cl, err := in.Client(ctx)
	if err != nil {
		return nil, fmt.Errorf("GitHub の認証に失敗しました: %w", err)
	}

	sc := in.Runner.Scope
	id, err := runnerID(ctx, cl, sc, in.Runner.Name())
	if err != nil {
		return nil, err
	}

	labels, err := cl.RunnerLabels(ctx, sc, id)
	if err != nil {
		return nil, fmt.Errorf("ラベルの取得に失敗しました: %w", err)
	}
	return labels, nil
}

// FetchGroups は選べる runner group の一覧を取得する。
//
// 名前だけでなく ID も返すのは、付け替えの API が ID を取るためである
// （名前から引き直すともう 1 度一覧を取ることになる）。
func FetchGroups(ctx context.Context, in CommitInput) ([]gh.RunnerGroup, error) {
	cl, err := in.Client(ctx)
	if err != nil {
		return nil, fmt.Errorf("GitHub の認証に失敗しました: %w", err)
	}

	groups, err := cl.ListRunnerGroups(ctx, in.Runner.Scope)
	if err != nil {
		return nil, fmt.Errorf("runner group の一覧の取得に失敗しました: %w", err)
	}
	return groups, nil
}
