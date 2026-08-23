package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/doctor"
	"github.com/ousiassllc/gsr-helper/internal/ui/hostreq"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/tabset"
)

// startHostReq は起動時の前提チェック（FR-44）を発行する Cmd を返す。1 度だけ
// 走らせる仕組みは hostreq.StartOnce が持つ（doc 参照）。
func (a *App) startHostReq() tea.Cmd {
	return hostreq.StartOnce(&a.hostReqDone, doctor.Input{
		Runners: a.result.Runners,
		Caps:    a.caps,
		Exec:    a.ex,
	}, a.hostChecks)
}

// doctorKey は Doctor タブの番号キーを返す。無効なら空文字。
//
// 状態行の誘導（`⚠ ホスト前提 2 件（5 で詳細）`）に添える番号である。引くのは
// tabset の仕事で、親はタブの番号を書き写さない（tabset.KeyOf）。
func (a App) doctorKey() string { return tabset.KeyOf(a.tabs, page.TabDoctor) }
