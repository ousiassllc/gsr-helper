package setup

import (
	"strconv"
	"strings"
)

// NextIndex は既存 runner 名から次に振る連番を返す（FR-11）。
//
// `<prefix>-<連番>` の形の名前だけを見て、その最大値の次を返す。1 件も無ければ 1。
// 純粋関数であり、ホストの状態を一切読まない（docs/components/overview.md）。
//
// 連番として扱うのは 10 進の数字だけからなる部分に限る。`build01-1a` や
// `build01-01` のような表記を数として拾うと、次に作る名前が既存と衝突しうる。
// ゼロ埋め（`build01-007`）は 7 として読む——`strconv.Atoi` が受け付ける形であり、
// 既存が 007 のときに 8 を返さないと衝突するためである。
func NextIndex(existing []string, prefix string) int {
	if prefix == "" {
		return 1
	}

	head := prefix + "-"
	maxIdx := 0
	for _, name := range existing {
		rest, ok := strings.CutPrefix(name, head)
		if !ok || !allDigits(rest) {
			continue
		}
		n, err := strconv.Atoi(rest)
		if err != nil || n < 1 {
			continue
		}
		if n > maxIdx {
			maxIdx = n
		}
	}
	return maxIdx + 1
}

// allDigits は s が 1 文字以上の 10 進数字だけで構成されるかを返す。
//
// strconv.Atoi は符号（`-1` / `+1`）を受け付けるため、それだけでは
// `build01--1` のような名前を連番と誤読する。先に文字種で弾く。
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// RunnerName は連番から runner 名を組み立てる（FR-11）。
func RunnerName(prefix string, index int) string {
	return prefix + "-" + strconv.Itoa(index)
}

// Names は開始連番から count 台分の名前を返す。
func Names(prefix string, start, count int) []string {
	out := make([]string, 0, count)
	for i := range count {
		out = append(out, RunnerName(prefix, start+i))
	}
	return out
}
