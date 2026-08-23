package logs

import (
	"context"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	dlogs "github.com/ousiassllc/gsr-helper/internal/logs"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// 購読（ログの追従）の開始・停止と、届いた行の取り込みを集める。

const (
	// maxLines は本文が保持する行数の上限。超えた分は古い側から捨てる。
	//
	// 追従は止めなければ際限なく行が届く。画面に出せるのは高々数十行で、
	// 遡れる量として数千行あれば足りる。
	maxLines = 5000
	// lineBuffer は購読のチャネルの容量であり、1 回の取り込みで受け取る行数の上限でもある。
	//
	// 追記が一気に来たときに送出側（ドメイン層）を待たせないための余裕であると同時に、
	// **1 回の取り込みでこのチャネルを空にできる**という意味を持たせてある。取り込みは
	// 描画の周期に律速され、1 周につき本文の差し替え（O(maxLines)）が 1 回走るので、
	// 1 周で 1 行しか進めないと追記の多いログでチャネルが埋まる（content.go の doc）。
	// 容量と上限を別の値にしても得られるものが無いため、同じ定数で表している。
	lineBuffer = 256
)

// target は今追っている対象。
type target struct {
	runner runner.Runner
	file   dlogs.File
	// journal が真なら systemd ユニットのログを追う（FR-26）。
	journal bool
}

// empty は追う対象が決まっていないかを返す。
func (t target) empty() bool { return t.runner.Dir == "" }

// name は見出しに出す対象の名前を返す。
func (t target) name() string {
	if t.journal {
		return t.runner.UnitName
	}
	return t.file.Name
}

// stream は購読 1 本ぶんの状態。
//
// **世代（gen）を持つのが要点である。** 対象を切り替えても、前の購読が既に発行した
// 行の Cmd はランタイムの中に残っており、切り替えた直後に届く。世代を突き合わせて
// 古い購読の行を捨てないと、別のログの行が新しい本文に混ざる。
type stream struct {
	gen    int
	cancel context.CancelFunc
	lines  <-chan dlogs.Line
	err    <-chan error
}

// lineMsg は購読から届いた行の束。ok が偽なら購読の終わり。
//
// **1 行ではなく束で運ぶ。** 1 行につき Msg を 1 つ回すと、bubbletea の 1 周につき 1 行しか
// 取り込めず、その 1 周ごとに本文の差し替えが走る（wait と content.go の doc）。
// ok が偽でも lines は空とは限らない。閉じる直前に届いていた行はここに載る。
type lineMsg struct {
	gen   int
	lines []dlogs.Line
	ok    bool
}

// endMsg は購読が終わった理由。err が nil なら正常な終了（畳んだ・ファイルの終わり）。
type endMsg struct {
	gen int
	err error
}

// filesMsg は `_diag` の列挙結果。
type filesMsg struct{ rows []row }

// listFiles は全 runner の `_diag` を列挙する Cmd を返す。
//
// ディスクを読むのでドメイン層の呼び出しとして page.Do を通す（結果を発行元のタブへ
// 戻す）。Update の中で読まないのは、UI をブロックしないためである。
func (m Model) listFiles() tea.Cmd {
	runners := slices.Clone(m.st.Result.Runners)
	return page.Do(m.tab, func() tea.Msg { return filesMsg{rows: logRows(runners)} })
}

// setFiles は列挙結果を一覧へ反映する。
//
// 対象がまだ決まっていなければ先頭（最も新しいログ）を開く。前面に出た直後に
// 空の本文だけが出ると、何をすれば読めるのかが画面から分からない。
func (m Model) setFiles(msg filesMsg) (tea.Model, tea.Cmd) {
	m.tbl.SetItems(sectionLogs, msg.rows)
	if !m.target.empty() || len(msg.rows) == 0 {
		return m, m.chrome()
	}
	cmd := m.open(target{runner: msg.rows[0].runner, file: msg.rows[0].file, journal: false})
	return m, tea.Batch(m.chrome(), cmd)
}

// open は対象を切り替えて購読を張り直す。
func (m *Model) open(t target) tea.Cmd {
	m.stop()
	m.target = t
	m.lines = nil
	m.err = nil
	m.body.SetFollow(true)
	m.applyLines()
	return m.subscribe()
}

// subscribe は今の対象の購読を開始し、最初の 1 行を待つ Cmd を返す。
//
// ctx は親から受け取らず自分で作る。購読の寿命はタブの表裏とアプリの終了で決まり
// （page.DeactivateMsg / page.ShutdownMsg）、それを知っているのはこの page だけである。
func (m *Model) subscribe() tea.Cmd {
	if m.target.empty() {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	lines := make(chan dlogs.Line, lineBuffer)
	errc := make(chan error, 1)

	m.stream.gen++
	m.stream.cancel = cancel
	m.stream.lines = lines
	m.stream.err = errc

	t := m.target
	ex := m.st.Exec
	go func() {
		if t.journal {
			errc <- dlogs.Journal(ctx, ex, t.runner.UnitName, lines)
			return
		}
		errc <- dlogs.Tail(ctx, t.file.Path, lines)
	}()
	return m.wait()
}

// wait は購読に届いている行をまとめて受け取る Cmd を返す。
//
// 受信を Cmd の中で行うのは bubbletea の作法である。goroutine から直に Msg を送る
// 経路（tea.Program.Send）は page が Program を持たない設計と噛み合わない。
//
// **最初の 1 行は待ち、そのあとは既に届いている分だけを取る。** 取り込みの費用は行数では
// なく Msg の数で決まる（1 つにつき本文の差し替えが 1 回。content.go の doc）ので、束に
// すれば追記が集中したときほど 1 行あたりが安くなる。2 行目以降を待たないのは、待つと
// 追記が疎なログで表示が束の分だけ遅れるためである。行が 1 本も無ければ従来どおり待つ
// だけであり、空振りの Msg で Update を回すことはない。
//
// **途中でチャネルが閉じたら、そこまでの行を載せたうえで ok を偽にする。** 束ごと捨てると
// 購読の最後の数行が画面に出ない。
func (m Model) wait() tea.Cmd {
	gen, ch := m.stream.gen, m.stream.lines
	if ch == nil {
		return nil
	}
	return page.Do(m.tab, func() tea.Msg {
		first, ok := <-ch
		if !ok {
			return lineMsg{gen: gen, lines: nil, ok: false}
		}
		batch := make([]dlogs.Line, 1, lineBuffer)
		batch[0] = first
		for len(batch) < lineBuffer {
			select {
			case l, more := <-ch:
				if !more {
					return lineMsg{gen: gen, lines: batch, ok: false}
				}
				batch = append(batch, l)
			default:
				return lineMsg{gen: gen, lines: batch, ok: true}
			}
		}
		return lineMsg{gen: gen, lines: batch, ok: true}
	})
}

// waitEnd は購読が返した理由を待つ Cmd を返す。
//
// 行のチャネルが閉じた**あと**に読む。送出側は行のチャネルを閉じてから理由を返すため、
// 閉じた時点ではまだ理由が届いていない。
func (m Model) waitEnd() tea.Cmd {
	gen, ch := m.stream.gen, m.stream.err
	if ch == nil {
		return nil
	}
	return page.Do(m.tab, func() tea.Msg { return endMsg{gen: gen, err: <-ch} })
}

// stop は購読を畳む。
//
// **キャンセルはここで直ちに行い、Cmd に包まない。** context のキャンセルは戻り値も
// 待ち時間も無い呼び出しであり、Update の中で行っても UI は止まらない。Cmd に包むと
// 実行がランタイムの都合まで遅れ、その間に届いた行が新しい対象の本文へ混ざる
// （世代の突き合わせで捨てられるが、畳んだつもりの購読が生きている時間を作らない）。
func (m *Model) stop() {
	if m.stream.cancel != nil {
		m.stream.cancel()
	}
	m.stream = stream{gen: m.stream.gen, cancel: nil, lines: nil, err: nil}
}

// addLine は届いた行の束を取り込む。
//
// 終わりの合図（ok が偽）でも先に取り込むのは、閉じる直前に届いていた行が束に載って
// いるためである（lineMsg の doc）。
func (m Model) addLine(msg lineMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.stream.gen {
		// 畳んだ購読の行。取り込むと別のログの行が混ざる。
		return m, nil
	}

	// 増分の入口を通す（保持中の行へ足すのも pushLines の中で行う）。届いた行のために
	// 全行を組み直さないためである（content.go の doc）。
	m.pushLines(msg.lines)
	if !msg.ok {
		return m, tea.Batch(m.chrome(), m.waitEnd())
	}
	return m, tea.Batch(m.chrome(), m.wait())
}

// endStream は購読の終わりを取り込む。
func (m Model) endStream(msg endMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.stream.gen || msg.err == nil {
		return m, nil
	}
	m.err = msg.err
	return m, m.chrome()
}

// appendLine は行を足し、上限を超えた分を古い側から捨てる。
//
// **切り落としたうえで詰め直す（slices.Clone する）のが要点である。** 再スライスだけでは背後の
// 配列を切れず、捨てたはずの先頭の行が append に配列を取り直させるまで参照され続ける。行の
// 文字列を最大でもう maxLines 行ぶん抱えたままになり、上限を設けた意味が薄れる。Model は値で
// 複製されて回る（Update が値レシーバ）ので、配列を共有したまま先頭をずらすと複製元と書き込み
// 位置が重なりうる。Clone はその共有も断つ。1 行あたり maxLines 要素の複製が要るが、写るのは
// スライスの中身（文字列のヘッダ）だけで行の文字列そのものは写らない。ring buffer にすれば
// 省けるものの、添字の回り込みを持ち込むほどの差ではないと判断してこの形にしている。
func appendLine(lines []dlogs.Line, l dlogs.Line) []dlogs.Line {
	lines = append(lines, l)
	if len(lines) > maxLines {
		lines = slices.Clone(lines[len(lines)-maxLines:])
	}
	return lines
}

// sortByNewest は行を更新時刻の降順（同時刻はファイル名の降順）に並べる。
func sortByNewest(rows []row) {
	slices.SortStableFunc(rows, func(a, b row) int {
		if !a.file.ModTime.Equal(b.file.ModTime) {
			if a.file.ModTime.After(b.file.ModTime) {
				return -1
			}
			return 1
		}
		return strings.Compare(b.file.Name, a.file.Name)
	})
}
