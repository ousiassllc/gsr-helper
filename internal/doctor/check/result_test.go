package check_test

import (
	"slices"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
)

func TestStatusString(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in   check.Status
		want string
	}{
		"OK":   {in: check.OK, want: "OK"},
		"WARN": {in: check.Warn, want: "WARN"},
		"FAIL": {in: check.Fail, want: "FAIL"},
		"SKIP": {in: check.Skip, want: "SKIP"},
		"未定義":  {in: check.Status(99), want: ""},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := tt.in.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// 対処が要るのは WARN と FAIL だけである。SKIP を含めると、docker の無い
// ホストで起動のたびに警告が出続ける。
func TestStatusBad(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in   check.Status
		want bool
	}{
		"OK は対処不要":    {in: check.OK, want: false},
		"WARN は対処が要る": {in: check.Warn, want: true},
		"FAIL は対処が要る": {in: check.Fail, want: true},
		"SKIP は対処不要":  {in: check.Skip, want: false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := tt.in.Bad(); got != tt.want {
				t.Errorf("Bad() = %v, want %v", got, tt.want)
			}
		})
	}
}

// 分類の並び順は functional.md のチェック項目一覧表と揃える。
func TestCategoryRankFollowsSpecOrder(t *testing.T) {
	t.Parallel()

	cats := check.Categories()
	want := []string{
		check.CatAuthz, check.CatNetwork, check.CatTime, check.CatResource,
		check.CatHistory, check.CatDocker, check.CatJobReq, check.CatDeps,
		check.CatSystemd, check.CatConsistency,
	}
	if !slices.Equal(cats, want) {
		t.Fatalf("Categories() = %q, want %q", cats, want)
	}
	for i, c := range cats {
		if got := check.CategoryRank(c); got != i {
			t.Errorf("CategoryRank(%q) = %d, want %d", c, got, i)
		}
	}
}

// 未知の分類は末尾へ回す。分類を足し忘れても行が消えない。
func TestCategoryRankUnknownGoesLast(t *testing.T) {
	t.Parallel()

	last := check.CategoryRank(check.CatConsistency)
	if got := check.CategoryRank("知らない分類"); got <= last {
		t.Errorf("CategoryRank(未知) = %d, want > %d", got, last)
	}
}

// Categories は内部の並びを複製して返す（呼び出し側が書き換えても影響しない）。
func TestCategoriesReturnsCopy(t *testing.T) {
	t.Parallel()

	got := check.Categories()
	got[0] = "書き換え"
	if again := check.Categories(); again[0] != check.CatAuthz {
		t.Errorf("Categories()[0] = %q, want %q（内部の並びが書き換えられた）", again[0], check.CatAuthz)
	}
}
