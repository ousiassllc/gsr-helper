package table_test

import (
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule/listrow"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/table"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/table/tabletest"
)

// molecule の *Row 関数は RowInput を開く 1 行の関数で RenderRow に渡せる
// （RenderRow の doc の担保。page/<tab> の render* も同じ形をしている）。
var (
	_ table.RenderRow[listrow.RunnerView] = func(in table.RowInput[listrow.RunnerView]) []string {
		return listrow.RunnerRow(in.Item, in.Cols, in.Styles)
	}
	_ table.RenderRow[listrow.JobView] = func(in table.RowInput[listrow.JobView]) []string {
		return listrow.JobRow(in.Item, in.Cols, in.Styles)
	}
	_ table.RenderRow[listrow.OrphanView] = func(in table.RowInput[listrow.OrphanView]) []string {
		return listrow.OrphanRow(in.Item, in.Cols, in.Styles)
	}
)

// 共通のフィクスチャは tabletest にある。**ここへ書き戻さないこと**——`table` 直下は
// 行数上限（1 ディレクトリ 2000 行）に張り付いており、フィクスチャを本体と同じ予算に
// 載せると次に検証を足す Issue が境界に当たる（atomic-design.md のディレクトリの行数）。
// 参照は tabletest → table の一方向で、`table` の非公開な状態は 1 つも export していない。
//
// ここでは短い名前へ束縛するだけにする。関数値なので呼び出し側の書き方は変わらない。
type row = tabletest.Row

var (
	testStyles     = tabletest.Styles
	runnerSection  = tabletest.RunnerSection
	orphanSection  = tabletest.OrphanSection
	renderRow      = tabletest.Render
	rows           = tabletest.Rows
	newTable       = tabletest.New
	newSectioned   = tabletest.NewSectioned
	newColumned    = tabletest.NewColumned
	send           = tabletest.Send
	names          = tabletest.Names
	columnIDs      = tabletest.ColumnIDs
	selectedName   = tabletest.SelectedName
	columnSets     = tabletest.ColumnSets
	longFilterKeys = tabletest.LongFilterKeys
	paletteSamples = tabletest.PaletteSamples
)
