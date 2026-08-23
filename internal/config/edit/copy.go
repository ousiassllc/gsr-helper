package edit

import (
	"fmt"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/config"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

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
		runner: src.Name(), labels: nil, group: "", groupID: 0, copies: nil, reload: false,
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

// writeCopies は複製先それぞれへ .env を書き込む（FR-40）。
//
// 1 台ごとに退避してから書く。途中で失敗した場合、それまでに書いた台は
// 新しい内容で、残りは元のままになる。どこまで進んだかは呼び出し側が
// 結果として報告する。
func writeCopies(body config.EnvFile, copies []CopyTarget) error {
	done := make([]string, 0, len(copies))
	for _, t := range copies {
		if err := backupIfExists(t.path); err != nil {
			return copyErr(done, t.name, err)
		}
		if err := config.SaveEnv(body, t.path); err != nil {
			return copyErr(done, t.name, err)
		}
		done = append(done, t.name)
	}
	return nil
}

// copyErr は複製がどこまで進んだかを添えたエラーを返す。
//
// 台数ぶんの書き込みが途中で止まると、書き終えた台は新しい内容・残りは元のまま
// という中途半端な状態になる。どの台が書き換わったかを文言に載せないと、
// 利用者は全台を開いて確かめ直すしかない。
func copyErr(done []string, failed string, err error) error {
	where := "書き換えた runner はありません"
	if len(done) > 0 {
		where = "書き換え済み: " + strings.Join(done, ", ")
	}
	return fmt.Errorf("%s への複製に失敗しました（%s）: %w", failed, where, err)
}
