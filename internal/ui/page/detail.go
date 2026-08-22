package page

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/pane"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

const (
	// detailLabelWidth は情報部のラベル列の幅。最も長いラベル（ディレクトリ）が
	// 収まる幅にして、値の開始桁を揃える。
	detailLabelWidth = 14
	// detailHeadingRows は操作リストの上に置く行数（空行 1 + 見出し 1）。
	detailHeadingRows = 2
	// detailHeading は操作リストの見出し（screens.md の詳細画面）。
	detailHeading = "操作"
	// detailManagedUnknown は起動方式が判定できていない（未稼働の）ときの表記。
	detailManagedUnknown = "未稼働（サービス登録なし・プロセスなし）"
)

// RunnerDetail は runner の詳細画面。情報部（pane.Detail）と操作リスト
// （organism.ChoiceList）の組み合わせで構成し、詳細画面専用の organism を作らない
// （atomic-design.md の「詳細画面の操作リストは ChoiceList を使う」）。
//
// Runners タブと Jobs タブが共用する（FR-46 / FR-47）。Jobs タブの操作対象も
// runner なので、同じ部品をそのまま開ける。
type RunnerDetail struct {
	keys   keymap.Set
	styles token.Styles

	target runner.Runner
	caps   appconfig.Caps

	info    pane.Detail
	list    organism.ChoiceList
	opRows  int // 操作リストが使う行数（項目 + 区切り線）
	infoLen int // 情報部の行数
	width   int
	height  int
}

// NewRunnerDetail は詳細画面を組み立てる。
func NewRunnerDetail(keys keymap.Set, s token.Styles) RunnerDetail {
	return RunnerDetail{
		keys:    keys,
		styles:  s,
		target:  runner.Runner{},
		caps:    appconfig.Caps{},
		info:    pane.NewDetail(),
		list:    organism.NewChoiceList(keys.List, s),
		opRows:  0,
		infoLen: 0,
		width:   0,
		height:  0,
	}
}

// Open は対象を差し替えて詳細を開く。
//
// 操作リストのカーソルは organism.ChoiceList.SetItems が常に先頭（安全側）へ戻す。
// 一覧の enter → 詳細の enter で破壊的操作に到達しないための規則（FR-46）である。
func (d *RunnerDetail) Open(r runner.Runner, caps appconfig.Caps) {
	d.target, d.caps = r, caps
	items := Choices(r, caps, d.keys.Runner)
	d.list.SetItems(items)
	d.opRows = len(items) + 1 // 破壊的操作の前に置く区切り線 1 本
	d.refresh()
}

// SetSize は詳細に割り当てられた領域を設定する。
func (d *RunnerDetail) SetSize(w, h int) {
	d.width, d.height = w, h
	d.refresh()
}

// Title は詳細画面の見出しを返す。
func (d RunnerDetail) Title() string {
	return d.target.Name() + "  詳細"
}

// Cursor は操作リストのカーソル位置を返す。
func (d RunnerDetail) Cursor() int { return d.list.Cursor() }

// Hints は詳細画面のフッタに出すキーヒントを返す（screens.md の詳細画面）。
//
// 操作キーの可否と理由は操作リストの各行に出るため、フッタには載せない。
// キー文字列は keymap から取り、説明だけを画面に合わせる。
func (d RunnerDetail) Hints() []atom.Hint {
	l := d.keys.List
	return []atom.Hint{
		{Key: BindingKey(l.Accept), Desc: "実行", Enabled: true, Reason: ""},
		{Key: BindingKey(l.Down) + "/" + BindingKey(l.Up), Desc: "選択", Enabled: true, Reason: ""},
		{Key: BindingKey(d.keys.Global.Back), Desc: "戻る", Enabled: true, Reason: ""},
	}
}

// Update は詳細画面のキーを処理する。
//
// ページ送り以外のキーは操作リストへ渡す。情報部（viewport）と操作リストは
// どちらも j / k を使うため、両方へ流すとカーソル移動とスクロールが同時に起きる。
// 詳細画面の j / k は操作の選択（screens.md の詳細画面）なので、スクロールは
// ページ送りのキーだけに割り当てる。
func (d RunnerDetail) Update(msg tea.Msg) (RunnerDetail, tea.Cmd) {
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		d.info, cmd = d.info.Update(msg)
		return d, cmd
	}
	if key.Matches(press, d.keys.List.PageDown, d.keys.List.PageUp) {
		var cmd tea.Cmd
		d.info, cmd = d.info.Update(press)
		return d, cmd
	}

	var cmd tea.Cmd
	d.list, cmd = d.list.Update(press)
	return d, cmd
}

// View は情報部と操作リストを縦に並べて返す。
func (d RunnerDetail) View() string {
	return strings.Join([]string{
		d.info.View(),
		"",
		d.styles.Header.Render(detailHeading),
		d.list.View(),
	}, "\n")
}

// refresh は情報部の行を組み直し、領域を情報部と操作リストへ配る。
//
// 幅が変わると値の中略位置が変わるため、行はサイズが決まってから組み立てる。
func (d *RunnerDetail) refresh() {
	lines := d.infoLines()
	d.infoLen = len(lines)
	d.info.SetContent(lines)
	d.info.SetSize(d.width, d.infoHeight())
	d.list.SetWidth(d.width)
}

// infoHeight は情報部に配る行数を返す。操作リストを先に確保する。
//
// 操作リストを優先するのは、情報部はスクロールできるが操作リストは全項目が
// 見えないと「押せない操作とその理由」を読めないためである。
func (d *RunnerDetail) infoHeight() int {
	avail := d.height - d.opRows - detailHeadingRows
	return max(min(avail, d.infoLen), 1)
}

// infoLines は情報部の行を返す。項目は screens.md の詳細画面のモックに従う。
func (d RunnerDetail) infoLines() []string {
	r := d.target
	svc, svcRole := serviceText(r)
	job, jobRole := jobText(r)
	version, versionRole := versionText(r)
	items := []struct {
		label string
		value string
		role  token.RoleToken
	}{
		{"スコープ", r.Scope.String(), token.RolePlain},
		{"起動方式", managedText(r), token.RolePlain},
		{"サービス", svc, svcRole},
		{"ジョブ", job, jobRole},
		{"バージョン", version, versionRole},
		{"ディレクトリ", r.Dir, token.RolePlain},
		{"work", r.WorkDir, token.RolePlain},
	}

	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, d.infoLine(it.label, it.value, it.role))
	}
	return out
}

// infoLine はラベルと値を 1 行に組む。
//
// 値は装飾する前に幅へ収める。装飾してから切り詰めると ANSI 列が壊れる
// （molecule.styledCell と同じ順序に揃えてある）。
func (d RunnerDetail) infoLine(label, value string, role token.RoleToken) string {
	rest := max(d.width-detailLabelWidth, 1)
	return d.styles.Muted.Render(atom.Cell(label, detailLabelWidth, atom.Left)) +
		d.styles.Style(role).Render(atom.Truncate(value, rest))
}

// managedText は起動方式の行の値を返す。systemd 管理ならユニット名も添える。
//
// 未稼働（ManagedUnknown）は一覧と同じ "-" にせず文で出す。列幅の制約がある一覧では
// 「値なし」と同じ記号になるが、余裕のある詳細画面では「サービス登録もプロセスも無い」
// ことを読み取れるようにする（ManagedBy.String はテーブル表示用の短い表記であり、
// 詳細用の表記は表示側の関心事なのでここに置く）。
func managedText(r runner.Runner) string {
	if r.Managed == runner.ManagedUnknown {
		return detailManagedUnknown
	}
	if r.UnitName == "" {
		return r.Managed.String()
	}
	return r.Managed.String() + "（" + r.UnitName + "）"
}

// serviceText はサービスの行の値と表示上の役割を返す。
//
// enabled / disabled（UnitFileState）を併記するのは、停止中の runner が次の起動で
// 上がるかどうかがここでしか分からないためである。
func serviceText(r runner.Runner) (string, token.RoleToken) {
	if r.Svc == nil {
		return atom.StatusText("", "")
	}
	text, role := atom.StatusText(r.Svc.Active, r.Svc.Sub)
	if r.Svc.FileState != "" {
		text += " / " + r.Svc.FileState
	}
	return text, role
}

// jobText はジョブの行の値と表示上の役割を返す。
//
// リポジトリ名は出さない。Runner.Worker（/proc 由来）にはジョブのリポジトリ情報が
// 無く、ログから取るのはログのパッケージの担当である。
func jobText(r runner.Runner) (string, token.RoleToken) {
	if !r.Busy() {
		// 表記を一覧（JOB 列）と揃えるため atom に任せる。
		return atom.JobText(false, 0)
	}
	pids := make([]string, 0, len(r.Workers))
	for _, w := range r.Workers {
		pids = append(pids, strconv.Itoa(w.PID))
	}
	return token.IconJob + " 実行中 " + atom.Duration(r.JobElapsed()) +
		"（Worker PID " + strings.Join(pids, ", ") + "）", token.RoleAccent
}

// versionText はバージョンの行の値と表示上の役割を返す。
//
// 最新版は渡さない。最新版の取得は GitHub API を使う機能の担当であり、未取得の
// 状態を「古い」と示さないためである（atom.VersionText の契約）。disableUpdate は
// 自動更新が止まっていることを示すため、設定されていれば添える。
func versionText(r runner.Runner) (string, token.RoleToken) {
	text, role := atom.VersionText(r.Version, "")
	if r.Config.DisableUpdate {
		text += "（disableUpdate=true）"
	}
	return text, role
}
