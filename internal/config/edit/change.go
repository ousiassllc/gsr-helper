package edit

import (
	"errors"
	"strconv"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/config"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// Change は承認を得て書き込む 1 件の変更。
//
// **差分に出す内容と実際に書き込む内容を同じ値から作る。** 別々に組むと、
// 承認した内容と書かれる内容が食い違う余地ができる（page/setup が計画を
// そのまま表示して実行に渡すのと同じ理由）。
type Change struct {
	// Kind はどの設定項目の変更か。
	Kind Kind
	// path は書き込む先。GitHub 側の値（ラベル / runner group）では空。
	path string
	// body は path へ書き込む内容。
	body string
	// before / after は差分の材料。
	before string
	after  string
	// labels は置き換えるラベル（KindLabels）。
	labels []string
	// runner は変更の対象となる runner の名前。差分の見出しに出す。
	//
	// ラベルと runner group は書き込み先が Commit の時点で決まる（名前から ID を
	// 引く）。承認の時点でどの runner への変更かを見せないと、対象を取り違えた
	// まま GitHub 側の破壊的な変更を承認できてしまう。
	runner string
	// group は設定する runner group の名前（KindGroup）。
	group string
	// groupID は設定する runner group の ID（KindGroup）。API はこちらを取る。
	groupID int64
	// copies は複製先の .env の書き込み先（KindCopy）。
	copies []CopyTarget
	// reload は書き込み後に daemon-reload が要るか（drop-in）。
	reload bool
	// lines は組み立て済みの差分行。複製のように before/after の 2 つでは
	// 表せない変更で使う。空なら before/after から組み立てる。
	lines []string
	// write はファイルへの書き込み。GitHub 側の変更では nil。
	//
	// **差分を組んだ値そのものを閉じ込める。** 書き込みのたびに文字列を解析し
	// 直すと、承認した差分と実際に書かれる内容が別の経路で作られることになる。
	write func() error
}

// 変更を組み立てられない場合のエラー。
var (
	// ErrNoUnit は systemd ユニットが無い runner で drop-in を編集しようとした
	// 場合のエラー。drop-in はユニットに対する上書きなので、置く先が決まらない。
	ErrNoUnit = errors.New("systemd ユニットが無いため drop-in を編集できません")
	// ErrEmptyCopyTarget は複製先を 1 つも選ばずに確定した場合のエラー。
	ErrEmptyCopyTarget = errors.New("複製先の runner を 1 つ以上選んでください")
	// ErrCopyNoChange は選んだ複製先がすべて既に同じ内容だった場合のエラー。
	ErrCopyNoChange = errors.New("選んだ runner の .env は既に同じ内容です")
)

// CopyTarget は複製先 1 台分。
type CopyTarget struct {
	name string
	path string
}

// FileBacked はファイルへの書き込みを伴うか。伴わない変更（ラベル / runner
// group）は GitHub API で即時に反映され、再起動も要らない。
func (c Change) FileBacked() bool { return c.path != "" || len(c.copies) > 0 }

// BackupPath は差分に出す退避先を返す。ファイルを書かない変更では空。
func (c Change) BackupPath() string {
	switch {
	case len(c.copies) > 0:
		return "複製先ごとの .env" + config.BackupSuffix
	case c.path != "":
		return c.path + config.BackupSuffix
	default:
		return ""
	}
}

// DiffLines は承認画面に出す差分の行を返す。
func (c Change) DiffLines() []string {
	if len(c.lines) > 0 {
		return c.lines
	}
	return SplitDiff(config.Diff(c.before, c.after))
}

// Changed は差分があるかを返す。無変更なら書き込みも承認も要らない。
func (c Change) Changed() bool { return len(c.lines) > 0 || c.before != c.after }

// BuildEnv は .env の変更を組み立てる。
//
// 空欄にした項目は行ごと取り除く。開いた時点で空だった項目は触らない
// （もともと無いキーに空行を足さない）。
// next と before は EnvKeys と同じ添字で並ぶ入力値と、開いた時点の値である。
func BuildEnv(ld Loader, r runner.Runner, next, before []string) (Change, error) {
	path := ld.EnvPath(r)

	f, err := config.LoadEnv(path)
	if err != nil {
		return Change{}, err
	}
	was := f.String()

	for i, spec := range EnvKeys {
		switch {
		case i < len(next) && next[i] != "":
			f.Set(spec.Key, next[i])
		case i < len(before) && before[i] != "":
			f.Unset(spec.Key)
		}
	}

	return newFileChange(KindEnv, path, was, f.String(), false,
		func() error { return config.SaveEnv(f, path) }), nil
}

// BuildPath は .path の変更を組み立てる。
func BuildPath(ld Loader, r runner.Runner, value string) (Change, error) {
	path := ld.PathPath(r)

	p, err := config.LoadPathFile(path)
	if err != nil {
		return Change{}, err
	}

	next := config.PathFile{Value: value}

	return newFileChange(KindPath, path, p.Value+"\n", value+"\n", false,
		func() error { return config.SavePathFile(next, path) }), nil
}

// BuildDropIn は systemd drop-in の変更を組み立てる。
func BuildDropIn(ld Loader, r runner.Runner, restart, memoryMax string) (Change, error) {
	path, ok := ld.DropInPath(r)
	if !ok {
		return Change{}, ErrNoUnit
	}

	// before は解析結果の再描画ではなくファイルの中身そのものにする。
	// **Parse はコメント・[Unit]・[Install]・未知のセクションを捨て、Render は
	// [Service] だけを書き出す。** 再描画を before にすると、手書きの
	// override.conf を編集したときに消える行が差分に 1 行も出ず、失われることを
	// 知らないまま承認させてしまう（FR-37）。
	before, err := config.ReadDropInRaw(path)
	if err != nil {
		return Change{}, err
	}
	d := config.ParseDropIn(before)

	setOrUnset(&d, "Restart", restart)
	setOrUnset(&d, "MemoryMax", memoryMax)

	return newFileChange(KindDropIn, path, before, d.Render(), true,
		func() error { return config.SaveDropIn(d, path) }), nil
}

// setOrUnset は空文字なら行を取り除き、そうでなければ設定する。
//
// systemd では値を空にすること自体が「本体の設定を打ち消す」意味を持つが、
// フォームの空欄は「指定しない」の意味で使う。打ち消しを表現したい場合は
// 本体ユニットを読んで判断する必要があり、この版では扱わない。
func setOrUnset(d *config.DropIn, key, value string) {
	if value == "" {
		d.Unset(key)
		return
	}
	d.Set(key, value)
}

// newFileChange はファイルを書き換える変更を組み立てる。
func newFileChange(k Kind, path, before, after string, reload bool, write func() error) Change {
	return Change{
		Kind: k, path: path, body: after, before: before, after: after,
		runner: "", labels: nil, group: "", groupID: 0, copies: nil, reload: reload,
		lines: nil, write: write,
	}
}

// BuildLabels はラベルの変更を組み立てる（GitHub API で即時反映）。
//
// name は対象の runner 名で、差分の見出しに出す。before には取得した現在の
// カスタムラベルを渡す。渡さないと差分が全行 + になり、同じ内容で確定しても
// 置換 API を呼んでしまう。
func BuildLabels(name string, before, after []string) Change {
	return Change{
		Kind: KindLabels, path: "", body: "",
		before: strings.Join(before, "\n"), after: strings.Join(after, "\n"),
		runner: name, labels: after, group: "", groupID: 0, copies: nil, reload: false,
		lines: nil, write: nil,
	}
}

// BuildGroup は runner group の変更を組み立てる（GitHub API で即時反映）。
//
// name は対象の runner 名で、差分の見出しに出す。before にはフォームを開いた
// 時点で選ばれていた group を渡す。渡さないと同じ group を選び直しただけでも
// 付け替えの API を呼んでしまう。
func BuildGroup(name, before, after string, id int64) Change {
	return Change{
		Kind: KindGroup, path: "", body: "", before: before, after: after,
		runner: name, labels: nil, group: after, groupID: id, copies: nil, reload: false,
		lines: nil, write: nil,
	}
}

// SplitDiff は Diff の結果を行へ分ける。
func SplitDiff(d string) []string {
	if d == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(d, "\n"), "\n")
}

// Reload は書き込み後に systemctl daemon-reload が要るかを返す。
func (c Change) Reload() bool { return c.reload }

// Labels は置き換えるラベルを返す（KindLabels のとき）。
func (c Change) Labels() []string { return c.labels }

// Copies は複製先の台数を返す（KindCopy のとき）。
func (c Change) Copies() int { return len(c.copies) }

// CopyNames は複製先の runner 名を返す（KindCopy のとき）。
//
// **反映（再起動）の対象は複製元ではなくこちらである。** .env が書き換わったのは
// 複製先であり、複製元の設定は 1 バイトも変わっていない。取り違えると、変更が
// 効いていない複製先を放置したまま、無関係な複製元のジョブを止めることになる。
func (c Change) CopyNames() []string {
	out := make([]string, 0, len(c.copies))
	for _, t := range c.copies {
		out = append(out, t.name)
	}
	return out
}

// Title は差分の見出しに出す対象を返す。
func (c Change) Title() string {
	if c.path != "" {
		return c.path
	}

	switch c.Kind {
	case KindLabels:
		return c.withRunner("ラベル（GitHub）")
	case KindGroup:
		return c.withRunner("runner group（GitHub）")
	case KindCopy:
		return ".env の複製（" + strconv.Itoa(len(c.copies)) + " 台）"
	case KindEnv, KindPath, KindDropIn, KindReregister, KindSelf:
		return ""
	default:
		return ""
	}
}

// withRunner は見出しへ対象の runner 名を添える。
func (c Change) withRunner(s string) string {
	if c.runner == "" {
		return s
	}
	return c.runner + " の" + s
}
