package jobs

import (
	"path/filepath"
	"strconv"

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
		if m.asked == nil {
			m.asked = make(map[string]struct{})
		}
		m.asked[key] = struct{}{}
		cmds = append(cmds, m.parseWorker(key, r))
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
}

// parseWorker は runner の直近の Worker ログを解析する Cmd を返す。
//
// 直近のログを使うのは、走っているジョブが 1 本だけであることを呼び出し側が
// 確かめているためである（resolveInfo）。ログが 1 件も無い（まだ書かれていない）
// 場合は空の結果を返し、行は `-` に縮退する。
func (m Model) parseWorker(key string, r runner.Runner) tea.Cmd {
	dir := filepath.Join(r.Dir, logs.DiagDir)
	f, ok := logs.LatestWorker(r)
	return page.Do(m.tab, func() tea.Msg {
		if !ok {
			return jobInfoMsg{key: key, info: logs.JobInfo{}}
		}
		info, _ := logs.ParseWorker(dir, f.Name)
		return jobInfoMsg{key: key, info: info}
	})
}

// onJobInfo は解析結果を覚え、一覧に反映する。
func (m *Model) onJobInfo(msg jobInfoMsg) {
	if m.info == nil {
		m.info = make(map[string]logs.JobInfo)
	}
	m.info[msg.key] = msg.info
	delete(m.asked, msg.key)
	m.tbl.SetItems(sectionJobs, jobRows(m.st.Result.Runners, m.info))
}
