package dialog

import (
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/stopwatch"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

const (
	// drainTitle は見出しの前半。後ろに対象 runner 名を添える。
	drainTitle = "ドレイン停止中"
	// drainWaiting は待機していることを示す行。
	drainWaiting = "実行中のジョブの完了を待っています…"
	// drainElapsed は経過時間のラベル。
	drainElapsed = "経過 "
	// drainJobLabel は待機中のジョブ 1 件に付けるラベル。
	drainJobLabel = "対象ジョブ: "
	// labelGap は見出しと添え物の間隔（screens.md のモックに合わせる）。
	labelGap = "   "
	// spinnerWidth はスピナ 1 コマと後ろの空白が使う幅。行の幅から差し引く。
	spinnerWidth = 2
	// drainTick は経過時間を刻む間隔。
	//
	// **明示するのは bubbles/stopwatch の既定が 0 だからである**（doc は「1 秒」と
	// 書いているが New は Interval を設定しない）。0 のままだと加算が 0 で経過が
	// 増えず、しかも tea.Tick が待たずに発火して Tick が際限なく流れ続ける。
	drainTick = time.Second
)

// drainNote は待機の制約の注記を返す。**常時表示する。**
//
// GitHub には runner の受付を止める API がなく、ドレイン停止は「今のジョブが
// 終わるのを待つ」だけである（FR-07）。待っている間に次のジョブが割り当てられ得る
// ことを画面に出さないと、利用者は「待てば必ず空く」と誤解して待ち続ける。
// 高さが足りない場合も、この 2 行は最後まで残す（fitHeight の tail に渡す）。
//
// パッケージ変数にしないのは、書き換えられる共有状態を作らないためである
// （keymap の定義と同じ扱い）。文言を差し替えられると制約の明示が黙って消える。
func drainNote() []string {
	return []string{
		token.IconWarn + " 待機中も新しいジョブを受け付ける可能性があります",
		"  （GitHub に受付停止の API がないため）",
	}
}

// DrainInput はドレイン待機の表示内容。
type DrainInput struct {
	Runner string     // 対象 runner 名（見出しに使う）
	Jobs   []DrainJob // 完了を待っているジョブ
}

// DrainJob は待機中の Worker 1 つ。
//
// Runner.Workers は複数あり得るため、待機画面も複数件を並べられる形で受け取る。
type DrainJob struct {
	Repository string        // 分かっていれば。空なら "-"
	PID        int           // Worker のプロセス ID
	Elapsed    time.Duration // ジョブ開始からの経過
}

// DrainCanceledMsg は待機のキャンセルを page へ通知する。
//
// ConfirmedMsg と同じく、待機画面自身は閉じない。閉じるかどうかと、既に発行済みの
// ドレイン停止をどう扱うか（待つのをやめるだけで停止要求は残る）は page が決める。
type DrainCanceledMsg struct{}

// DrainWaiter はドレイン停止の待機画面。
//
// **進捗バーは出さない。** 待ち時間は無制限（FR-07）で完了時期を約束できないため、
// 分母のある表示にすると「あと少しで終わる」という誤った見通しを与える。動いている
// ことは bubbles/spinner、経過時間は bubbles/stopwatch で示す
// （atomic-design.md の「bubbles/progress を使う範囲」）。
type DrainWaiter struct {
	in      DrainInput
	sw      stopwatch.Model
	sp      spinner.Model
	cancel  key.Binding
	styles  token.Styles
	running bool
	width   int
	height  int
}

// NewDrainWaiter は待機画面を組み立てる。
//
// スピナは MiniDot（1 コマ 1 セル）を使う。絵文字のコマは端末によって 1 セルにも
// 2 セルにもなり、後続の文字が 1 セルずれる。
func NewDrainWaiter(keys keymap.Global, s token.Styles) DrainWaiter {
	return DrainWaiter{
		in:      DrainInput{Runner: "", Jobs: nil},
		sw:      stopwatch.New(stopwatch.WithInterval(drainTick)),
		sp:      spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(s.Accent)),
		cancel:  keys.Back,
		styles:  s,
		running: false,
		width:   0,
		height:  0,
	}
}

// SetInput は待機の対象を差し替える。3 秒ごとの再検出でジョブが減ったときに呼ぶ。
//
// 経過時間は差し替えない。計っているのは「待ち始めてからの時間」であり、
// 対象の増減で 0 に戻すと、どれだけ待ったのかが分からなくなる。
func (d *DrainWaiter) SetInput(in DrainInput) {
	d.in = in
}

// Restyle は配色とキー定義を差し替える。計時とスピナの状態は保つ。
//
// 作り直さずに差し替えるのは、共有状態が 3 秒ごとに配られるためである
// （organism.ChoiceList.Restyle と同じ理由）。作り直すと経過時間が 3 秒ごとに
// 0 へ戻り、待機時間を読めなくなる。
func (d *DrainWaiter) Restyle(keys keymap.Global, s token.Styles) {
	d.cancel, d.styles = keys.Back, s
	d.sp.Style = s.Accent
}

// SetSize は待機画面に配られた領域を設定する。
func (d *DrainWaiter) SetSize(w, h int) {
	d.width, d.height = w, h
}

// Start は計時を 0 に戻したうえで計時とスピナを動かす Cmd を返す。page が流す。
//
// **Reset を添えるのは、この画面が使い回されるためである。** 待機画面は Overlay へ
// 1 度だけ登録され（runnerop.New）、2 件目以降の対象でも 2 回目のドレインでも同じ
// 実体が開き直す。戻さないと前回の経過が引き継がれ、押した直後に「経過 5m」と出る。
func (d *DrainWaiter) Start() tea.Cmd {
	d.running = true
	return tea.Batch(d.sw.Reset(), d.sw.Start(), d.sp.Tick)
}

// Stop は計時とスピナを止める Cmd を返す。
//
// スピナは自分の Tick を Update で繋いで回り続けるため、止めるには配るのをやめる
// しかない（running）。止め忘れると、待機を終えた後も 12 分の 1 秒ごとに Msg が
// 流れ続け、他の画面の再描画を無駄に起こす。
func (d *DrainWaiter) Stop() tea.Cmd {
	d.running = false
	return d.sw.Stop()
}

// Elapsed は待ち始めてからの経過時間を返す。
func (d DrainWaiter) Elapsed() time.Duration { return d.sw.Elapsed() }

// Title は見出しを返す。枠（template.Modal）に渡すために公開する。
func (d DrainWaiter) Title() string {
	if d.in.Runner == "" {
		return drainTitle
	}
	return drainTitle + labelGap + d.in.Runner
}

// Update は計時とスピナの Tick を内側へ配り、esc をキャンセルとして解釈する。
//
// esc をここで解釈するため、page はこの画面の Modal.HandlesBack に真を返させること
// （Confirm.Update と同じ。返さないと Overlay が esc を閉じる操作として消費する）。
func (d DrainWaiter) Update(msg tea.Msg) (DrainWaiter, tea.Cmd) {
	if press, ok := msg.(tea.KeyPressMsg); ok {
		if key.Matches(press, d.cancel) {
			return d, func() tea.Msg { return DrainCanceledMsg{} }
		}
		return d, nil
	}

	var swCmd, spCmd tea.Cmd
	d.sw, swCmd = d.sw.Update(msg)
	if d.running {
		d.sp, spCmd = d.sp.Update(msg)
	}
	return d, tea.Batch(swCmd, spCmd)
}

// View は待機の中身を返す。制約の注記は必ず含まれる。
//
// 「esc: 待機をキャンセル」の行はここに置かない。キーヒントはフッタ
// （molecule.KeyBar）が一手に描く決まりであり、画面ごとに本文へも書くと
// 有効なキーの表示が 2 箇所に分かれる（Hints が返す）。
func (d DrainWaiter) View() string {
	body := make([]string, 0, len(d.in.Jobs)+2)
	body = append(body, d.waitingLine())
	for _, job := range d.in.Jobs {
		body = append(body, d.fit(drainJobLabel+jobText(job), 0))
	}
	// 空行は注記と対にせず本文側の末尾に置く。高さが足りないときに真っ先に
	// 落ちる行がこの空行になり、注記の 2 行は最後まで丸ごと残る。
	body = append(body, "")

	return strings.Join(fitHeight(body, d.note(), d.height), "\n")
}

// Hints はフッタに出すキーヒントを返す。
func (d DrainWaiter) Hints() []atom.Hint {
	return []atom.Hint{hint(d.cancel, "待機をキャンセル")}
}

// waitingLine は待機中であることと経過時間の行を返す。
func (d DrainWaiter) waitingLine() string {
	text := drainWaiting + labelGap + drainElapsed + atom.Duration(d.sw.Elapsed())
	return d.sp.View() + " " + d.fit(text, spinnerWidth)
}

// note は制約の注記を返す。
func (d DrainWaiter) note() []string {
	note := drainNote()

	lines := make([]string, 0, len(note))
	for _, line := range note {
		lines = append(lines, d.styles.Warn.Render(d.fit(line, 0)))
	}
	return lines
}

// fit は 1 行を幅に収める。reserve は行頭に別途置くもの（スピナ）が使う幅である。
//
// 装飾する前に呼ぶ理由と、幅が未設定のときに切り詰めない理由は Confirm.fit と同じ。
func (d DrainWaiter) fit(line string, reserve int) string {
	if d.width <= 0 {
		return line
	}
	return atom.Truncate(line, max(d.width-reserve, 0))
}

// jobText は待機中のジョブ 1 件を 1 行の文字列にする。
//
// リポジトリ名が空のときに "-" を出すのは、Runner.Worker からリポジトリ名を
// 取れない場合があるためである（screens.md の Jobs タブと同じ扱い）。空欄のままだと
// 「リポジトリ名が無いジョブ」と「取得できなかったジョブ」を読み分けられない。
func jobText(j DrainJob) string {
	repo := j.Repository
	if repo == "" {
		repo = token.IconNoUnit
	}
	return repo + "（Worker PID " + pidText(j.PID) + "、開始から " + atom.Duration(j.Elapsed) + "）"
}

// pidText はプロセス ID を返す。取得できていない場合は「値なし」の記号にする。
func pidText(pid int) string {
	if pid <= 0 {
		return token.IconNoUnit
	}
	return strconv.Itoa(pid)
}
