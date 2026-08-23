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

	d, err := config.LoadDropIn(path)
	if err != nil {
		return Change{}, err
	}
	before := d.Render()

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
		labels: nil, group: "", groupID: 0, copies: nil, reload: reload, lines: nil, write: write,
	}
}

// BuildLabels はラベルの変更を組み立てる（GitHub API で即時反映）。
func BuildLabels(before, after []string) Change {
	return Change{
		Kind: KindLabels, path: "", body: "",
		before: strings.Join(before, "\n"), after: strings.Join(after, "\n"),
		labels: after, group: "", groupID: 0, copies: nil, reload: false, lines: nil, write: nil,
	}
}

// writeCopies は複製先それぞれへ .env を書き込む（FR-40）。
//
// 1 台ごとに退避してから書く。途中で失敗した場合、それまでに書いた台は
// 新しい内容で、残りは元のままになる。どこまで進んだかは呼び出し側が
// 結果として報告する。
func writeCopies(body config.EnvFile, copies []CopyTarget) error {
	for _, t := range copies {
		if err := backupIfExists(t.path); err != nil {
			return err
		}
		if err := config.SaveEnv(body, t.path); err != nil {
			return err
		}
	}
	return nil
}

// BuildGroup は runner group の変更を組み立てる（GitHub API で即時反映）。
func BuildGroup(before, after string, id int64) Change {
	return Change{
		Kind: KindGroup, path: "", body: "", before: before, after: after,
		labels: nil, group: after, groupID: id, copies: nil, reload: false,
		lines: nil, write: nil,
	}
}

// BuildCopy は .env の複製を組み立てる（FR-40）。
//
// 差分は複製先ごとに見出しを付けて並べる。見出しを変更なしの印で始めるのは、
// 差分の行として色が付かないようにするためである（描画側は行頭の印だけを見る）。
func BuildCopy(ld Loader, src runner.Runner, targets []runner.Runner, chosen []string) (Change, error) {
	body, err := config.LoadEnv(ld.EnvPath(src))
	if err != nil {
		return Change{}, err
	}
	after := body.String()

	picked := make(map[string]bool, len(chosen))
	for _, n := range chosen {
		picked[n] = true
	}

	c := Change{
		Kind: KindCopy, path: "", body: after, before: "", after: "",
		labels: nil, group: "", groupID: 0, copies: nil, reload: false,
		lines: nil, write: nil,
	}

	var lines []string
	for _, t := range targets {
		if !picked[t.Name()] {
			continue
		}

		p := ld.EnvPath(t)
		cur, lerr := config.LoadEnv(p)
		if lerr != nil {
			return Change{}, lerr
		}

		c.copies = append(c.copies, CopyTarget{name: t.Name(), path: p})
		lines = append(lines, config.MarkContext+"=== "+t.Name()+" ===")
		lines = append(lines, SplitDiff(config.Diff(cur.String(), after))...)
	}

	if len(c.copies) == 0 {
		return Change{}, ErrEmptyCopyTarget
	}

	// 複製は差分を組み立て済みなので、before/after ではなくこの行を出す。
	c.lines = lines
	copies := c.copies
	c.write = func() error { return writeCopies(body, copies) }

	return c, nil
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

// Title は差分の見出しに出す対象を返す。
func (c Change) Title() string {
	if c.path != "" {
		return c.path
	}

	switch c.Kind {
	case KindLabels:
		return "ラベル（GitHub）"
	case KindGroup:
		return "runner group（GitHub）"
	case KindCopy:
		return ".env の複製（" + strconv.Itoa(len(c.copies)) + " 台）"
	case KindEnv, KindPath, KindDropIn, KindReregister, KindSelf:
		return ""
	default:
		return ""
	}
}
