package logs

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/exec"
)

// systemd ユニットのログの追従（FR-26）を置く。

const (
	// JournalLines は 1 回の取得で遡る行数（`journalctl -n` の引数）。
	//
	// 画面に出せるのは高々数十行なので、初回に見える範囲としてはこれで足りる。
	// 増やすほど取得のたびに読み捨てる行が増える。
	JournalLines = 200
	// JournalInterval は取得の間隔。
	//
	// 一覧の自動更新（3 秒）より短くしているのは、こちらが「追従」だからである。
	// 1 秒より短くすると、1 回の取得が終わる前に次を発行しうる。
	JournalInterval = 2 * time.Second
	// journalTimeout は 1 回の取得に課す上限。
	//
	// Executor の既定（30 秒）より短くするのは、追従が JournalInterval ごとの
	// 繰り返しだからである。1 回が 30 秒待たされると、その間ずっと画面が
	// 止まったまま利用者には理由が分からない。
	journalTimeout = 10 * time.Second
	// journalAction は監査ログの action（記録する場合の名前）。
	journalAction = "logs.journal"
)

// ErrNoUnit は systemd ユニットを持たない runner に対して追従を求められたことを表す。
var ErrNoUnit = errors.New("systemd ユニットがありません")

// Journal は unit のログを追従し、行を out へ送る（FR-26）。
//
// **`journalctl -f` は使わない。** Executor は 1 回の実行の出力をまとめて返す契約で
// あり（internal/exec の Result）、`-f` を渡すとタイムアウトまで 1 行も届かないまま
// プロセスだけが残る。外部プロセスの実行経路を Executor 1 本に保つ（監査記録と
// タイムアウトの適用漏れを構造的に防ぐ）ほうが、追従のためだけに別経路を開けるより
// 安全なので、`journalctl -u <unit> -n <N> --no-pager` を JournalInterval ごとに
// 発行し、前回の出力との重なりを除いた差分だけを送る形にしている。
//
// Tail と同じく、戻るときに out を閉じる。
//
// 取得は監査ログに記録しない（exec.Options.SkipAudit）。変更を伴わない読み取りで
// あり、追従している間ずっと繰り返し発行されて他のレコードを押し流すためである
// （docs/architecture/security.md の「記録対象外とする読み取りコマンド」）。
func Journal(ctx context.Context, ex exec.Executor, unit string, out chan<- Line) error {
	defer close(out)

	if ex == nil {
		return errors.New("外部コマンドを実行できません")
	}
	if unit == "" {
		return ErrNoUnit
	}

	var prev []string
	for {
		lines, err := readJournal(ctx, ex, unit)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		for _, s := range lines[overlap(prev, lines):] {
			select {
			case out <- NewLine(s):
			case <-ctx.Done():
				return nil
			}
		}
		prev = lines

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(JournalInterval):
		}
	}
}

// readJournal は unit の直近 JournalLines 行を取得する。
func readJournal(ctx context.Context, ex exec.Executor, unit string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, journalTimeout)
	defer cancel()
	ctx = exec.WithOptions(ctx, exec.Options{
		Action:    journalAction,
		Runner:    "",
		Dir:       "",
		Env:       nil,
		SkipAudit: true,
	})
	res, err := ex.Run(ctx, "journalctl", "-u", unit, "-n", strconv.Itoa(JournalLines), "--no-pager")
	if err != nil {
		return nil, fmt.Errorf("%s のログを取得できません: %w", unit, err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("%s のログを取得できません: journalctl が終了コード %d を返しました", unit, res.ExitCode)
	}
	return splitLines(string(res.Stdout)), nil
}

// splitLines は出力を行へ分ける。末尾の改行が生む空行は落とす。
func splitLines(s string) []string {
	s = strings.TrimSuffix(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// overlap は prev の末尾と cur の先頭が一致する最大の長さを返す。
//
// 取得のたびに同じ末尾 N 行が返るため、そのままでは同じ行を何度も送ることになる。
// 一致する最大の重なりを求め、その先だけを新しい行として扱う。**行の内容だけで
// 突き合わせるので、同じ文言が連続して出力された場合は重なりを長く取りすぎて
// 数行を出し損ねることがある。** `journalctl` の行は時刻を含むため実際にはまれで
// あり、取りこぼしても次の取得で末尾側は必ず届く。
//
// prev が空（初回）なら 0 を返し、cur の全行を新しい行として送る。
func overlap(prev, cur []string) int {
	n := min(len(prev), len(cur))
	for ; n > 0; n-- {
		if equalLines(prev[len(prev)-n:], cur[:n]) {
			return n
		}
	}
	return 0
}

// equalLines は 2 つの行の並びが等しいかを返す。
func equalLines(a, b []string) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
