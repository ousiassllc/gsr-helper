package doctor

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/doctor"
	"github.com/ousiassllc/gsr-helper/internal/ui/hostreq"
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
// （atomic-design.md の page の責務）。ディスク使用率の閾値も同じ理由で共有状態
// から取る（doctor は設定ファイルを読み直さない）。
func (m Model) checkInput() doctor.Input {
	return doctor.Input{
		Runners:        m.st.Result.Runners,
		Caps:           m.st.Caps,
		Exec:           m.st.Deps.Exec,
		DiskThresholds: m.st.Disk.Thresholds,
	}
}

// applyDone は診断の結果を取り込み、詳細画面と親へ知らせる Cmd を返す。
func (m *Model) applyDone(msg doneMsg) tea.Cmd {
	if msg.id == "" {
		m.results = msg.results
	} else {
		m.results = doctor.Replace(m.results, msg.results, msg.id)
	}
	m.summary = doctor.Count(m.results)
	m.lastRun = msg.at
	m.running = false
	m.tbl.SetItems(sectionResults, resultRows(m.results))
	return tea.Batch(m.refreshDetail(), m.reportHostReq())
}

// refreshDetail は開いたままの詳細画面を新しい結果で描き直す Cmd を返す。
//
// 個別再実行（FR-34）の唯一の入口はこの画面である。描き直さないと、対処を終えて
// r を押した運用者が古い 検出内容 / 影響 / 推奨する対処 を見続けることになり、
// 「対処後に再実行できる」という受け入れ条件が実効を失う。
//
// **開き直さず宛先を明示して差し替える**（page.ModalMsg）。開き直すとモーダルの
// 重なりの最上位がこの画面へ動くため、詳細の上でヘルプを開いていた場合に、
// 読んでいたヘルプが黙って裏へ回る。
func (m *Model) refreshDetail() tea.Cmd {
	if !m.overlay.Active() || m.detail.id == "" {
		return nil
	}
	for _, r := range m.results {
		if keyOf(r) != m.detail {
			continue
		}
		var cmd tea.Cmd
		m.overlay, cmd = m.overlay.Update(page.ModalMsg{Kind: Kind, Msg: OpenMsg{Result: r}})
		return cmd
	}
	return nil
}

// reportHostReq は起動時の前提チェック（FR-44）の件数を親へ届ける Cmd を返す。
//
// **page.Do では包まない。** 包みは page が発行した Cmd の結果を発行元のタブへ
// 戻すためのものであり、この件数の宛先は親 Model である（親の Update は
// hostreq.Msg を受ける分岐を持つ）。page.ChromeMsg にも載せられない。ヘッダと
// 状態行の「ホスト前提 N 件」は起動時に見た項目の話であって、タブが報告する
// 状態行の右側とは別の値だからである。
//
// **数え直すのは個別再実行のときも同じである。** 取り込み済みの結果（doctor.Replace
// でマージ後の m.results）全体から数えるので、1 項目だけを直したときも件数が動く。
func (m Model) reportHostReq() tea.Cmd {
	msg := hostreq.CountStartup(m.results)
	return func() tea.Msg { return msg }
}
