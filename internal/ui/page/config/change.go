package config

import (
	"errors"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/config"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// change は承認を得て書き込む 1 件の変更。
//
// **差分に出す内容と実際に書き込む内容を同じ値から作る。** 別々に組むと、
// 承認した内容と書かれる内容が食い違う余地ができる（page/setup が計画を
// そのまま表示して実行に渡すのと同じ理由）。
type change struct {
	kind kind
	// path は書き込む先。GitHub 側の値（ラベル / runner group）では空。
	path string
	// body は path へ書き込む内容。
	body string
	// before / after は差分の材料。
	before string
	after  string
	// labels は置き換えるラベル（kindLabels）。
	labels []string
	// group は設定する runner group の名前（kindGroup）。
	group string
	// groupID は設定する runner group の ID（kindGroup）。API はこちらを取る。
	groupID int64
	// copies は複製先の .env の書き込み先（kindCopy）。
	copies []copyTarget
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

// errNoUnit は systemd ユニットが無い runner で drop-in を編集しようとした場合の
// エラー。drop-in はユニットに対する上書きなので、置く先が決まらない。
var errNoUnit = errors.New("systemd ユニットが無いため drop-in を編集できません")

// copyTarget は複製先 1 台分。
type copyTarget struct {
	name string
	path string
}

// fileBacked はファイルへの書き込みを伴うか。伴わない変更（ラベル / runner
// group）は GitHub API で即時に反映され、再起動も要らない。
func (c change) fileBacked() bool { return c.path != "" || len(c.copies) > 0 }

// backupPath は差分に出す退避先を返す。ファイルを書かない変更では空。
func (c change) backupPath() string {
	switch {
	case len(c.copies) > 0:
		return "複製先ごとの .env" + config.BackupSuffix
	case c.path != "":
		return c.path + config.BackupSuffix
	default:
		return ""
	}
}

// diffLines は承認画面に出す差分の行を返す。
func (c change) diffLines() []string {
	if len(c.lines) > 0 {
		return c.lines
	}
	return splitDiff(config.Diff(c.before, c.after))
}

// changed は差分があるかを返す。無変更なら書き込みも承認も要らない。
func (c change) changed() bool { return len(c.lines) > 0 || c.before != c.after }

// buildEnv は .env の変更を組み立てる。
//
// 空欄にした項目は行ごと取り除く。開いた時点で空だった項目は触らない
// （もともと無いキーに空行を足さない）。
func buildEnv(ld loader, r runner.Runner, v *values) (change, error) {
	path := ld.envPath(r)

	f, err := config.LoadEnv(path)
	if err != nil {
		return change{}, err
	}
	before := f.String()

	for i, k := range envKeys {
		switch {
		case v.env[i] != "":
			f.Set(k.key, v.env[i])
		case v.envBefore[i] != "":
			f.Unset(k.key)
		}
	}

	return newFileChange(kindEnv, path, before, f.String(), false,
		func() error { return config.SaveEnv(f, path) }), nil
}

// buildPath は .path の変更を組み立てる。
func buildPath(ld loader, r runner.Runner, v *values) (change, error) {
	path := ld.pathPath(r)

	p, err := config.LoadPathFile(path)
	if err != nil {
		return change{}, err
	}

	next := config.PathFile{Value: v.path}

	return newFileChange(kindPath, path, p.Value+"\n", v.path+"\n", false,
		func() error { return config.SavePathFile(next, path) }), nil
}

// buildDropIn は systemd drop-in の変更を組み立てる。
func buildDropIn(ld loader, r runner.Runner, v *values) (change, error) {
	path, ok := ld.dropInPath(r)
	if !ok {
		return change{}, errNoUnit
	}

	d, err := config.LoadDropIn(path)
	if err != nil {
		return change{}, err
	}
	before := d.Render()

	setOrUnset(&d, "Restart", v.restart)
	setOrUnset(&d, "MemoryMax", v.memoryMax)

	return newFileChange(kindDropIn, path, before, d.Render(), true,
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
func newFileChange(k kind, path, before, after string, reload bool, write func() error) change {
	return change{
		kind: k, path: path, body: after, before: before, after: after,
		labels: nil, group: "", groupID: 0, copies: nil, reload: reload, lines: nil, write: write,
	}
}

// buildLabels はラベルの変更を組み立てる（GitHub API で即時反映）。
func buildLabels(before, after []string) change {
	return change{
		kind: kindLabels, path: "", body: "",
		before: strings.Join(before, "\n"), after: strings.Join(after, "\n"),
		labels: after, group: "", groupID: 0, copies: nil, reload: false, lines: nil, write: nil,
	}
}

// writeCopies は複製先それぞれへ .env を書き込む（FR-40）。
//
// 1 台ごとに退避してから書く。途中で失敗した場合、それまでに書いた台は
// 新しい内容で、残りは元のままになる。どこまで進んだかは呼び出し側が
// 結果として報告する。
func writeCopies(body config.EnvFile, copies []copyTarget) error {
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

// buildGroup は runner group の変更を組み立てる（GitHub API で即時反映）。
func buildGroup(before, after string, id int64) change {
	return change{
		kind: kindGroup, path: "", body: "", before: before, after: after,
		labels: nil, group: after, groupID: id, copies: nil, reload: false,
		lines: nil, write: nil,
	}
}

// buildCopy は .env の複製を組み立てる（FR-40）。
//
// 差分は複製先ごとに見出しを付けて並べる。見出しを変更なしの印で始めるのは、
// 差分の行として色が付かないようにするためである（描画側は行頭の印だけを見る）。
func buildCopy(ld loader, src runner.Runner, targets []runner.Runner, v *values) (change, error) {
	body, err := config.LoadEnv(ld.envPath(src))
	if err != nil {
		return change{}, err
	}
	after := body.String()

	chosen := make(map[string]bool, len(v.copyTo))
	for _, n := range v.copyTo {
		chosen[n] = true
	}

	c := change{
		kind: kindCopy, path: "", body: after, before: "", after: "",
		labels: nil, group: "", groupID: 0, copies: nil, reload: false,
		lines: nil, write: nil,
	}

	var lines []string
	for _, t := range targets {
		if !chosen[t.Name()] {
			continue
		}

		p := ld.envPath(t)
		cur, lerr := config.LoadEnv(p)
		if lerr != nil {
			return change{}, lerr
		}

		c.copies = append(c.copies, copyTarget{name: t.Name(), path: p})
		lines = append(lines, config.MarkContext+"=== "+t.Name()+" ===")
		lines = append(lines, splitDiff(config.Diff(cur.String(), after))...)
	}

	if len(c.copies) == 0 {
		return change{}, ErrEmptyCopyTarget
	}

	// 複製は差分を組み立て済みなので、before/after ではなくこの行を出す。
	c.lines = lines
	copies := c.copies
	c.write = func() error { return writeCopies(body, copies) }

	return c, nil
}

// splitDiff は Diff の結果を行へ分ける。
func splitDiff(d string) []string {
	if d == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(d, "\n"), "\n")
}
