package pane

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// spinnerWidth はスピナ 1 コマと後ろの空白が使う幅。見出しの幅から差し引く。
const spinnerWidth = 2

// ProgressInput は進捗表示の中身。
//
// 行の状態（完了・実行中・待機・失敗）と結果報告の文面を決めるのは page である。
// ここは受け取った値を並べるだけで、ドメイン層の結果を解釈しない。
type ProgressInput struct {
	Title  string                  // 見出し（「追加中…」）
	Rows   []molecule.ProgressView // 対象ごとの進み具合
	Done   int                     // 完了した件数
	Total  int                     // 全体の件数。0 なら分母不明でバーを描かない
	Report []string                // 完了後の結果報告（成功・未実行・失敗理由）。素の文字列で渡す
}

// ProgressList は一括処理の逐次表示と結果報告の領域（FR-15）。
//
// **バーを描くのは全体件数が事前に確定している場合だけである**（atomic-design.md の
// 「`bubbles/progress` を使う範囲」）。分母が分からない処理でバーを出すと「あと少しで
// 終わる」という約束にならない見通しを与えるため、その場合はスピナだけを出す。
//
// 見出しとバーは viewport の外に置く。スピナは 1 コマごとに描き替わるので、行と
// 一緒に viewport の中身へ焼き込むと、送り込み直すまで前のコマが残る。
type ProgressList struct {
	in      ProgressInput
	vp      viewport.Model
	bar     progress.Model
	sp      spinner.Model
	styles  token.Styles
	running bool
	width   int
	height  int
}

// NewProgressList は進捗表示を組み立てる。
//
// スピナは MiniDot（1 コマ 1 セル）を使う（dialog.NewDrainWaiter と同じ理由。絵文字の
// コマは端末によって 1 セルにも 2 セルにもなり、後続の文字が 1 セルずれる）。
//
// バーは半ブロックではなく全ブロックで塗る。半ブロックは前景と背景に別の色を置いて
// 塗り分ける解像度を稼ぐためのものであり、単色で塗るここでは色が 2 つ必要になるだけ
// である。百分率は添えない。件数（`2/3`）を見出しに出すので、同じ情報が 1 行に 2 度出る。
func NewProgressList(s token.Styles) ProgressList {
	vp := viewport.New()
	vp.KeyMap = viewportKeyMap(keymap.NewList())

	p := ProgressList{
		in:      ProgressInput{Title: "", Rows: nil, Done: 0, Total: 0, Report: nil},
		vp:      vp,
		bar:     newBar(),
		sp:      spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(s.Accent)),
		styles:  s,
		running: false,
		width:   0,
		height:  0,
	}
	p.paintBar()
	return p
}

// newBar は進捗バーを組み立てる。色は paintBar が Styles から与える。
func newBar() progress.Model {
	return progress.New(
		progress.WithoutPercentage(),
		progress.WithFillCharacters(progress.DefaultFullCharFullBlock, progress.DefaultEmptyCharBlock),
	)
}

// SetInput は表示する中身を差し替える。逐次の完了通知が届くたびに page が呼ぶ。
func (p *ProgressList) SetInput(in ProgressInput) {
	p.in = in
	p.refresh()
}

// Restyle は配色を差し替える。進捗と受信状態は保つ。
//
// 作り直さずに差し替えるのは、背景の明暗が起動後に届き（tea.BackgroundColorMsg）、
// 共有状態が 3 秒ごとに配られるためである（pane.Log.Restyle と同じ理由）。作り直すと
// 実行中の進捗が消える。
func (p *ProgressList) Restyle(s token.Styles) {
	p.styles = s
	p.sp.Style = s.Accent
	p.paintBar()
	p.refresh()
}

// SetSize は進捗表示に配られた領域を設定する。
func (p *ProgressList) SetSize(w, h int) {
	p.width, p.height = w, h
	p.refresh()
}

// Start はスピナを動かす Cmd を返す。page が処理を始めるときに流す。
func (p *ProgressList) Start() tea.Cmd {
	p.running = true
	return p.sp.Tick
}

// Stop はスピナを止める。
//
// スピナは自分の Tick を Update で繋いで回り続けるため、止めるには配るのをやめる
// しかない（running）。止め忘れると、処理を終えた後も 12 分の 1 秒ごとに Msg が
// 流れ続け、他の画面の再描画を無駄に起こす（dialog.DrainWaiter.Stop と同じ）。
//
// 戻り値の Cmd は常に nil だが、DrainWaiter.Stop と形を揃えて残す。呼び出し側が
// 「止める Cmd を流す」書き方を 2 通り覚えずに済む。
func (p *ProgressList) Stop() tea.Cmd {
	p.running = false
	return nil
}

// Update はスクロールのキーとスピナの Tick を内側へ配る。
func (p ProgressList) Update(msg tea.Msg) (ProgressList, tea.Cmd) {
	var vpCmd, spCmd tea.Cmd
	p.vp, vpCmd = p.vp.Update(msg)
	if p.running {
		p.sp, spCmd = p.sp.Update(msg)
	}
	return p, tea.Batch(vpCmd, spCmd)
}

// View は見出し・バー・行・結果報告を縦に並べて返す。
//
// 見出しとバーは常に残り、高さが足りない分は行と結果報告の側がスクロールに回る。
// 何件中何件が終わったのかは、行が 1 つも見えていなくても読めるようにする。
func (p ProgressList) View() string {
	head := p.header()
	if p.vp.Height() <= 0 {
		return strings.Join(head, "\n")
	}
	return strings.Join(append(head, p.vp.View()), "\n")
}

// Offset はスクロール位置（先頭から隠している行数）を返す。
func (p ProgressList) Offset() int { return p.vp.YOffset() }

// refresh は大きさと中身の変化を viewport とバーへ反映する。
//
// 見出しの行数は分母の有無で変わる（バーの 1 行）ため、中身を差し替えるたびに
// viewport の高さを計算し直す。固定にすると、バーが出た瞬間に領域が 1 行はみ出す。
func (p *ProgressList) refresh() {
	p.bar.SetWidth(max(p.width, 1))
	p.vp.SetWidth(max(p.width, 0))
	p.vp.SetHeight(max(p.height-len(p.header()), 0))
	p.vp.SetContentLines(p.body())
}

// paintBar はバーの色を Styles から与える。
//
// 色を使わない Styles では前景が未設定（nil）になり、バーも素の記号だけで描かれる。
// NO_COLOR の縮退を Styles と同じ経路で伝えるためであり、ここで既定の配色に頼ると
// バーだけが色付きで残る。
func (p *ProgressList) paintBar() {
	p.bar.FullColor = p.styles.Accent.GetForeground()
	p.bar.EmptyColor = p.styles.Muted.GetForeground()
}

// header は見出しとバーの行を返す。
//
// 分母が分かっているときは `追加中… 2/3` と件数を添えてバーを描き、分からないときは
// スピナを頭に置いてバーを省く（atomic-design.md の「`bubbles/progress` を使う範囲」）。
func (p ProgressList) header() []string {
	if p.in.Total > 0 {
		title := p.in.Title + " " + strconv.Itoa(p.in.Done) + "/" + strconv.Itoa(p.in.Total)
		return []string{p.fit(title, 0), p.bar.ViewAs(p.percent())}
	}
	return []string{p.sp.View() + " " + p.fit(p.in.Title, spinnerWidth)}
}

// percent は完了の割合を 0〜1 で返す。分母が 0 のときは 0 とする。
//
// 範囲外の値を丸めるのは、完了通知の取りこぼしや二重計上で Done が Total を超えても
// バーが崩れないようにするためである（atom.Ratio と同じ扱い）。
func (p ProgressList) percent() float64 {
	if p.in.Total <= 0 {
		return 0
	}
	return min(max(float64(p.in.Done)/float64(p.in.Total), 0), 1)
}

// body はスクロールする側の行（対象ごとの進捗と結果報告）を返す。
//
// 結果報告は区切り線の下に置く（screens.md の Setup タブのモック）。処理中の行と
// 終わった後の報告が地続きに並ぶと、どこまでが進捗でどこからが結果なのか読めない。
func (p ProgressList) body() []string {
	lines := make([]string, 0, len(p.in.Rows)+len(p.in.Report)+2)
	for _, r := range p.in.Rows {
		lines = append(lines, molecule.ProgressRow(r, p.width, p.styles))
	}
	if len(p.in.Report) == 0 {
		return lines
	}

	if len(lines) > 0 {
		lines = append(lines, "")
	}
	lines = append(lines, atom.Divider(p.width, "", p.styles))
	for _, r := range p.in.Report {
		// 報告は素の文字列で受け取る。幅に収めてから装飾しないと ANSI 列が壊れる
		// （atom.Truncate の契約）ため、装飾済みの行は受け取れない。
		lines = append(lines, p.fit(r, 0))
	}
	return lines
}

// fit は 1 行を幅に収める。reserve は行頭に別途置くもの（スピナ）が使う幅である。
//
// 折り返さず中略するのは、この領域が 1 対象 1 行で読む表示だからである。折り返すと
// 行数が対象の数と合わなくなり、どの行がどの対象なのか追えなくなる。幅が未設定
// （0 以下）なら何もしない（dialog.Confirm.fit と同じ）。
func (p ProgressList) fit(line string, reserve int) string {
	if p.width <= 0 {
		return line
	}
	return atom.Truncate(line, max(p.width-reserve, 0))
}
