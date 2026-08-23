package disk

import (
	"context"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/disk"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// 使用量の集計（FR-27 / FR-28 / FR-29）を開始・停止・受信する。
//
// **UI をブロックしない。** 走査は runner 1 台で数分かかりうるため、集計はすべて
// tea.Cmd の外側（goroutine）で回し、判明した対象から 1 件ずつ Msg で受け取って
// 表に足す。全件そろってから描くと、大きな runner が 1 台あるだけで画面が数分固まる。

const (
	// dockerSkipLabel は docker が使えない環境で出す SKIP 行の表示名。
	dockerSkipLabel = "docker"
	// dockerPruneLabel は実際に選べる docker の行の表示名。内訳とは別に 1 行置くのは、
	// 発行するのが docker system prune -f 1 本で種別を選り分けられないためである
	// （internal/disk.DockerLabel と同じ文字列。一覧の行名と進捗の突き合わせを揃える）。
	dockerPruneLabel = disk.DockerLabel
	// 以下 3 つは選択できない理由。**いずれも 22 セル以内に収める。** 理由は一覧の
	// 最終列（PATH）に載り、organism/table が最終列を 1 セル狭めるため使えるのは
	// token.DiskColumns の 25 セルではなく 24 セルである。長いと末尾が中略されて
	// 理由が読めない（disk.busyReason の doc と同じ制約）。
	//
	// docker の行を消さずに理由付きで残すのは、内訳が「0 バイト」なのか「そもそも
	// 見ていない」のかを読み分けられるようにするためである。受け入れ条件の「docker が
	// 無い環境でも起動でき、docker 関連の集計・クリーンアップが SKIP として縮退する」
	// はこの 1 行で満たす。
	dockerSkipReason      = "docker が無く集計不可" // docker が使えない環境
	dockerFailReason      = "docker の集計に失敗"  // 集計そのものに失敗した
	dockerBreakdownReason = "内訳は選択できません"     // 内訳は選り分けて消せない
	// rootPath は runner が 1 台も無いときにファイルシステムの残量を見る場所。
	rootPath = "/"
)

// scanState は実行中の集計。nil なら集計していない。
//
// **判明した行（Model.seen）は持たない。** 表の中身と選択は集計より寿命が長く、
// 裏へ回って集計を畳んでも捨ててはいけない（page.DeactivateMsg の doc）。ここに
// 置くと stopScan で行ごと消え、次の共有状態（3 秒ごと）で表が空になる。
type scanState struct {
	// cancel は走査の打ち切り。呼ぶと disk.Scan の goroutine が畳まれ、集約側が
	// channel を閉じるので、待っている Cmd も必ず返る。
	cancel context.CancelFunc
	// ch は判明した対象が流れてくる channel。集約する goroutine だけが閉じる。
	ch <-chan disk.Usage
}

// usageMsg は集計 1 件の到着。ok が偽なら channel が閉じた（集計完了）ことを表す。
//
// 世代を載せるのは、古い集計の結果を捨てるためである。r による再集計とタブの
// 出入りで集計は何度も張り直され、前の集計の goroutine は打ち切りを受け取るまで
// 数百ミリ秒ぶん生き残る。世代を見ないと、その残りが新しい表に混ざる。
type usageMsg struct {
	gen   int
	usage disk.Usage
	ok    bool
}

// fsStatsMsg はファイルシステムの容量と inode の取得結果（FR-29）。
type fsStatsMsg struct {
	gen   int
	stats disk.Stats
	err   error
}

// startScan は集計を開始し、結果を待つ Cmd を返す。
//
// **goroutine を Update から起こす。** 一見すると副作用を page に持ち込んでいるが、
// ここで起こしているのは購読の開始であって処理の実行ではない。走査の結果は必ず
// page.Do を通した Msg として戻り、状態を書き換えるのは Update だけなので、副作用の
// 発生点は page に閉じている。tea.Cmd で 1 本ずつ回さないのは、Cmd が「1 回実行して
// 1 つの Msg を返す」形しか取れず、判明順に何件も返す集計を表せないためである。
//
// 集約の goroutine を挟むのは、runner ごと・docker の集計を 1 本の channel にまとめる
// ためである。disk.Scan は out を閉じない契約で（複数 runner を 1 本に集約する前提）、
// 閉じる責務は集約する側にある。
func (m *Model) startScan() tea.Cmd {
	m.stopScan()
	m.gen++
	gen := m.gen

	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan disk.Usage)
	runners := m.st.Result.Runners
	ex := m.st.Exec
	useDocker := m.st.Caps.Docker

	go func() {
		var wg sync.WaitGroup
		for _, r := range runners {
			wg.Add(1)
			go func() {
				defer wg.Done()
				disk.Scan(ctx, r, ch)
			}()
		}
		if useDocker {
			wg.Add(1)
			go func() {
				defer wg.Done()
				scanDocker(ctx, ex, ch)
			}()
		}
		wg.Wait()
		close(ch)
	}()

	m.scan = &scanState{cancel: cancel, ch: ch}
	m.seen = nil
	if !useDocker {
		m.seen = append(m.seen, dockerUsage(dockerSkipLabel, -1, false, dockerSkipReason, nil))
	}
	m.tbl.SetItems(sectionTargets, usageRows(m.seen))
	return tea.Batch(m.waitUsage(gen, ch), m.fsStats(gen))
}

// stopScan は実行中の集計を畳む。集計していなければ何もしない。
//
// **表の中身と選択は捨てない。** 裏へ回ったことが利用者に見えてしまうためである
// （page.DeactivateMsg の doc）。畳むのは外へ伸びている処理（走査の goroutine と
// docker の呼び出し）だけである。
//
// ここで世代を進める必要は無い。畳んでいる間は m.scan が nil なので届いた結果は
// すべて捨てられ、張り直すときは startScan が必ず世代を進めるためである。世代の
// 突き合わせが効くのは「新しい集計が走っている最中に古い集計の残りが届く」場合だけ
// である（startScan が前の集計を畳んでから張り直す経路）。
func (m *Model) stopScan() {
	if m.scan == nil {
		return
	}
	m.scan.cancel()
	m.scan = nil
}

// waitUsage は集計 1 件の到着を待つ Cmd を返す。
//
// 1 件受け取るたびに次の 1 件を待つ Cmd を返し直すことで、判明順の反映（FR-28）を
// tea.Cmd の「1 回で 1 つの Msg」という形に収める。
func (m Model) waitUsage(gen int, ch <-chan disk.Usage) tea.Cmd {
	return page.Do(m.tab, func() tea.Msg {
		u, ok := <-ch
		return usageMsg{gen: gen, usage: u, ok: ok}
	})
}

// onUsage は集計 1 件を表に反映し、次の 1 件を待つ Cmd を返す。
//
// 古い世代の Msg は黙って捨てる。捨て損ねると、再集計の途中に前回の結果が混ざって
// 同じ対象が 2 行並ぶ（識別子は同じなので選択も取り合う）。
func (m *Model) onUsage(msg usageMsg) tea.Cmd {
	if m.scan == nil || msg.gen != m.gen {
		return nil
	}
	if !msg.ok {
		// channel が閉じた＝全対象を送り終えた。打ち切りの関数はここで解放する。
		m.scan.cancel()
		m.scan = nil
		return nil
	}

	m.seen = append(m.seen, msg.usage)
	m.tbl.SetItems(sectionTargets, usageRows(m.seen))
	return m.waitUsage(msg.gen, m.scan.ch)
}

// fsStats はファイルシステムの容量と inode を取る Cmd を返す（FR-29）。
//
// 見る場所を最初の runner の _work にするのは、runner の作業ディレクトリが別の
// ファイルシステムに載っていることがあるためである（/opt を別ボリュームにする構成）。
// ルートの残量を出すと、空けたい相手とは違うファイルシステムを見せることになる。
// runner が 1 台も無ければ見る相手が決まらないのでルートを見る。
func (m Model) fsStats(gen int) tea.Cmd {
	path := rootPath
	if rs := m.st.Result.Runners; len(rs) > 0 && rs[0].WorkDir != "" {
		path = rs[0].WorkDir
	}
	return page.Do(m.tab, func() tea.Msg {
		s, err := disk.FSStats(path)
		return fsStatsMsg{gen: gen, stats: s, err: err}
	})
}

// onFSStats は取得結果を保持する。失敗しても一覧は出す（要約行だけが縮退する）。
func (m *Model) onFSStats(msg fsStatsMsg) {
	if msg.gen != m.gen {
		return
	}
	m.stats, m.statsErr = msg.stats, msg.err
}

// scanDocker は docker の使用量を集計して out へ送る。
//
// disk.DockerUsage が返すのは内訳（disk.DockerItem）であり、行にするための
// disk.Usage への変換は page が行う。**KindDocker を作れるのは page だけである**
// （internal/disk の中では誰も生成しない）。docker の対象が「選べるか」を決めるのは
// 能力判定（Caps.Docker）を持つ page であり、ドメイン側は判断材料を持たない。
//
// 失敗しても行を落とさず、選択できない 1 行として出す。docker があるのに内訳を
// 取れない状態は利用者が知るべき異常であり、黙って消すと「docker の分が計上されて
// いない一覧」を正しい一覧として見せることになる。選べる 1 行（dockerPruneLabel）を
// 内訳より先に送るのは、それが唯一の操作対象だからである。
func scanDocker(ctx context.Context, ex exec.Executor, out chan<- disk.Usage) {
	items, err := disk.DockerUsage(ctx, ex)
	if err != nil {
		sendUsage(ctx, out, dockerUsage(dockerSkipLabel, -1, false, dockerFailReason, err))
		return
	}
	// 選べる 1 行に載せるのは prune -f が回収する種別だけの合計である。どの種別が
	// 回収されるかを知っているのは、発行するコマンドを持つ internal/disk である。
	sendUsage(ctx, out, dockerUsage(dockerPruneLabel, disk.PruneReclaimable(items), true, "", nil))
	for _, it := range items {
		// 内訳に出すのは総容量ではなく解放できる量である（prune で消えるのは未使用分
		// だけで、総容量を出すと内訳の合計が実際より大きくなる）。**内訳は選べない。**
		// -f だけの prune はボリュームを 1 バイトも消さず dangling 以外のイメージも残す
		// ため、内訳の Reclaimable を選択合計に載せると確認ダイアログが実現しない量を
		// 約束する。行を残すのは FR-27。
		sendUsage(ctx, out, dockerUsage(it.Label, it.Reclaimable, false, dockerBreakdownReason, nil))
	}
}

// sendUsage は打ち切りを尊重しつつ 1 件送る。
//
// 素の送信にしないのは、受け手（waitUsage）が世代違いを捨てて次を待たなくなった
// 後に送ると、goroutine が永久に残るためである。
func sendUsage(ctx context.Context, out chan<- disk.Usage, u disk.Usage) {
	select {
	case out <- u:
	case <-ctx.Done():
	}
}

// dockerUsage は docker の集計結果 1 行を組み立てる。docker の対象は runner にもパスにも
// 紐付かず（Runner / Base / Path は常に空）、ファイル数も数えられない（disk.Usage.Files
// の doc）。同じゼロ値を 4 か所へ書き写すと、フィールドが増えたときに一部だけ古く残る。
func dockerUsage(label string, bytes int64, removable bool, reason string, err error) disk.Usage {
	return disk.Usage{
		Kind: disk.KindDocker, Runner: "", Base: "", Path: "",
		Label: label, Bytes: bytes, Files: -1,
		Removable: removable, Reason: reason, Err: err,
	}
}
