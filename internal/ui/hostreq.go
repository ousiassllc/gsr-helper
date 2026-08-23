package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/doctor"
	"github.com/ousiassllc/gsr-helper/internal/ui/hostreq"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/tabset"
)

// startHostReq は起動時の前提チェック（FR-44）を発行する Cmd を返す。
//
// **runner を検出したあとに 1 度だけ走らせる。** パスワード不要 sudo と docker
// グループ所属は実行ユーザーごとに判定するので runner 一覧が要り、判定対象は
// ホストの構成なので秒単位では変わらない。3 秒ごとに走らせると、監査ログへ記録
// される `sudo -l -U` が他のレコードを押し流す。
func (a *App) startHostReq() tea.Cmd {
	if a.hostReqDone {
		return nil
	}
	cmd := hostreq.Start(doctor.Input{
		Runners: a.result.Runners,
		Caps:    a.caps,
		Exec:    a.ex,
	}, a.hostChecks)
	if cmd == nil {
		return nil
	}
	a.hostReqDone = true
	return cmd
}

// doctorKey は Doctor タブの番号キーを返す。無効なら空文字。
//
// 状態行の誘導（`⚠ ホスト前提 2 件（5 で詳細）`）に添える番号である。引くのは
// tabset の仕事で、親はタブの番号を書き写さない（tabset.KeyOf）。
func (a App) doctorKey() string { return tabset.KeyOf(a.tabs, page.TabDoctor) }
