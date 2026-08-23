package doctor

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/doctor"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// runTimeout は 1 回の診断にかける上限。
//
// 1 コマンドあたりの待ちは check.ProbeTimeout（3 秒）が押さえるが、runner ごとに
// 何本もコマンドを出す項目があるため、項目数ぶん積み上がりうる。全体の上限を切って
// おかないと、応答の遅いホストで画面が「実行中…」のまま戻らない。
const runTimeout = 60 * time.Second

// doneMsg は診断の完了。page.Do に包まれてこのタブへ戻る。
type doneMsg struct {
	// id は個別に再実行した項目の識別子。全項目の実行なら空。
	id string
	// results は結果。id が空なら全件、そうでなければその項目のぶんだけ。
	results []doctor.CheckResult
	// at は完了時刻。見出しの「最終実行」に出す。
	at time.Time
}

// startAll は全項目の診断を始める（FR-32 / FR-34 の全体再実行）。
func (m Model) startAll() tea.Cmd {
	return m.start("", doctor.Default())
}

// startOne は 1 項目だけを再実行する（FR-34 の個別再実行）。
//
// 対処したあと 1 項目だけ確かめたいときに、ネットワーク到達性まで含む全体を
// 走らせずに済ませるための経路である。
func (m Model) startOne(id string) tea.Cmd {
	checks := doctor.ByID(doctor.Default(), id)
	if len(checks) == 0 {
		return nil
	}
	return m.start(id, checks)
}

// start は checks を実行する Cmd を返す。
//
// **page.Do で包む。** 診断は数秒かかるので、待っている間に利用者がタブを
// 切り替えうる。包まないと結果が別のタブへ渡って静かに失われる（page.Do の doc）。
func (m Model) start(id string, checks []doctor.Check) tea.Cmd {
	in := m.checkInput()
	return page.Do(m.tab, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
		defer cancel()
		return doneMsg{id: id, results: doctor.Run(ctx, in, checks), at: time.Now()}
	})
}

// checkInput は共有状態から診断の入力を組み立てる。
//
// 差し替え口（Now / Dial / Getenv / LookPath / FSRoot / NewClient）は本番では
// すべてゼロ値のままにする。実環境を見る既定へ落ちる（check.Input の doc）。
//
// **runner 一覧は親が検出したものを使う。** doctor は自分で検出しない
// （atomic-design.md の page の責務）。
func (m Model) checkInput() doctor.Input {
	return doctor.Input{
		Runners: m.st.Result.Runners,
		Caps:    m.st.Caps,
		Exec:    m.st.Exec,
	}
}

// applyDone は診断の結果を取り込む。
func (m *Model) applyDone(msg doneMsg) {
	if msg.id == "" {
		m.results = msg.results
	} else {
		m.results = doctor.Replace(m.results, msg.results, msg.id)
	}
	m.summary = doctor.Count(m.results)
	m.lastRun = msg.at
	m.running = false
	m.tbl.SetItems(sectionResults, resultRows(m.results))
}
