package page

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
)

// ConfigSavedMsg は自身の設定を書き込めたことを親へ知らせる Msg（Issue #128）。
//
// 親 Model の設定は起動時に 1 度決まるだけなので、Config タブがファイルへ書いても
// 共有状態には古い値が載り続け、ディスク閾値の判定は再起動まで変わらなかった。
// 書けた値を親へ返し、親が以後の StateMsg に載せ直すことで解く。
//
// **page.Do で包まない**（包むと発行元のタブへ戻る。TabMsg の doc）。**書き込みに
// 成功したときだけ発行する**（失敗した値を親が取り込むと画面の判定が食い違う）。
type ConfigSavedMsg struct {
	// Conf は書き込めた設定そのもの。親はこれを現在値として採用する。
	Conf appconfig.Config
}

// ConfigSaved は ConfigSavedMsg を発行する Cmd を返す。
func ConfigSaved(cfg appconfig.Config) tea.Cmd {
	return func() tea.Msg { return ConfigSavedMsg{Conf: cfg} }
}
