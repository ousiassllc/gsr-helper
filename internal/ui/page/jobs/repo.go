package jobs

import (
	"path/filepath"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/logs"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// REPOSITORY 列と `_work` 列を Worker ログの解析で埋める（Issue #68）。
//
// 解析はファイルの読み取りなので Update の中では行わず、page.Do を通した Cmd で
// 行って結果を Msg で受け取る（atomic-design.md の非同期処理モデル）。
//
// **再検出（3 秒ごと）のたびに読み直さない。** 一度引いた結果は覚えておき、
// まだ引いていないジョブについてだけ Cmd を発行する。ジョブが変われば鍵も変わる
// ので、同じ runner で次のジョブが始まれば自動的に引き直される。

// maxParseTries は 1 つのジョブについて Worker ログを引き直す上限。
//
// 再検出は 3 秒ごとなので、およそ 30 秒ぶん待ってから諦める計算になる。ログが
// 書かれ始めるまでの猶予としては十分で、取り出し口を持たないジョブ（checkout を
// しないワークフロー）でも 3 秒ごとの読み取りがジョブの間ずっと続くことはない。
const maxParseTries = 10

// jobKey はジョブ 1 件を識別する鍵。runner ディレクトリと Worker の PID の組。
//
// PID を含めるのは、**同じ runner で次のジョブが始まったら引き直す**ためである。
// runner だけを鍵にすると、最初のジョブのリポジトリ名が以後ずっと残る。
func jobKey(dir string, pid int) string {
	return dir + "\x00" + strconv.Itoa(pid)
}

// jobInfoMsg は Worker ログ 1 件の解析結果。
type jobInfoMsg struct {
	key  string
	info logs.JobInfo
	// retry が真なら、まだこのジョブ自身のログが現れていない。覚えずに次の周期で
	// 引き直す（onJobInfo）。
	retry bool
}

// resolveInfo はまだ引いていないジョブについて解析の Cmd を発行し、覚えている結果を
// 現在のジョブのぶんだけに刈り込む。
//
// **1 台で 2 本以上のジョブが走っている runner は引かない。** Worker ログの名前には
// タイムスタンプしか無く PID が無いため、どのログがどのジョブのものかを決められない。
// 取り違えた表示は、無い表示より悪い。その runner の行は `-` に縮退する。
func (m *Model) resolveInfo(runners []runner.Runner) tea.Cmd {
	live := make(map[string]struct{})
	var cmds []tea.Cmd

	for _, r := range runners {
		if len(r.Workers) != 1 {
			continue
		}
		key := jobKey(r.Dir, r.Workers[0].PID)
		live[key] = struct{}{}
		if _, done := m.info[key]; done {
			continue
		}
		if _, pending := m.asked[key]; pending {
			continue
		}
		// **引き直しには上限を置く。** 取り出し口を持たないジョブ（checkout をしない
		// ワークフロー）では永久に空のままなので、際限なく引くと 3 秒ごとにログを
		// 読み続けることになる。上限に達したら引かず、`-` のままにする。
		if m.tries[key] >= maxParseTries {
			continue
		}
		if m.tries == nil {
			m.tries = make(map[string]int)
		}
		m.tries[key]++
		if m.asked == nil {
			m.asked = make(map[string]struct{})
		}
		m.asked[key] = struct{}{}
		cmds = append(cmds, m.parseWorker(key, r, r.Workers[0].Started))
	}

	m.prune(live)
	return tea.Batch(cmds...)
}

// prune は終わったジョブの結果を捨てる。
//
// 捨てないと、長く開いたままのセッションで解析結果が際限なく溜まる。
func (m *Model) prune(live map[string]struct{}) {
	for key := range m.info {
		if _, ok := live[key]; !ok {
			delete(m.info, key)
		}
	}
	for key := range m.asked {
		if _, ok := live[key]; !ok {
			delete(m.asked, key)
		}
	}
	for key := range m.tries {
		if _, ok := live[key]; !ok {
			delete(m.tries, key)
		}
	}
}

// parseWorker は runner の直近の Worker ログを解析する Cmd を返す。
//
// 直近のログを使うのは、走っているジョブが 1 本だけであることを呼び出し側が
// 確かめているためである（resolveInfo）。
//
// **ログの更新時刻がジョブの開始より古ければ、それは前のジョブのログである。**
// ジョブが切り替わった直後は新しい Worker ログがまだ無く、そのまま読むと前のジョブの
// リポジトリ名を今のジョブのものとして出す。取り違えた表示は、無い表示より悪い
// （同時実行中に埋めないのと同じ理由）。この場合は覚えずに次の周期へ回す。
//
// **一覧の取得も Cmd の中で行う。** logs.LatestWorker はディレクトリの走査と
// エントリごとの Stat を伴うファイル I/O であり、Update の中で走らせると再検出の
// たびに UI が止まる（このファイル冒頭の約束）。
func (m Model) parseWorker(key string, r runner.Runner, started time.Time) tea.Cmd {
	dir := filepath.Join(r.Dir, logs.DiagDir)
	return page.Do(m.tab, func() tea.Msg {
		f, ok := logs.LatestWorker(r)
		if !ok || f.ModTime.Before(started) {
			// このジョブのログがまだ現れていない。次の周期で引き直す。
			return jobInfoMsg{key: key, retry: true}
		}
		info, _ := logs.ParseWorker(dir, f.Name)
		// **ログはあるが取り出し口がまだ書かれていない**ことがある（Worker が
		// ファイルを作った直後）。ここで覚えると、そのジョブは終了まで `-` のまま
		// 固定される。リポジトリ名が取れるまでは覚えずに引き直す（上限は
		// maxParseTries）。
		return jobInfoMsg{key: key, info: info, retry: info.Repository == ""}
	})
}

// onJobInfo は解析結果を覚え、一覧に反映する。
//
// retry のものは覚えない。覚えると、ログが書かれる前に 1 度引いただけのジョブが
// 以後ずっと `-` のまま再試行されなくなる。発行済みの印（asked）だけを外して
// 次の周期に委ねる。
func (m *Model) onJobInfo(msg jobInfoMsg) {
	delete(m.asked, msg.key)
	if msg.retry {
		return
	}
	if m.info == nil {
		m.info = make(map[string]logs.JobInfo)
	}
	m.info[msg.key] = msg.info
	m.tbl.SetItems(sectionJobs, jobRows(m.st.Result.Runners, m.info))
}
