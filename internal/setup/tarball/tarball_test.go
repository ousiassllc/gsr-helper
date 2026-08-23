package tarball

import (
	"slices"
	"testing"
)

// FR-21 が挙げる 6 つの名前と過不足なく一致すること。
// 1 つ落ちるだけで、バージョン更新のたびに runner の登録がやり直しになる。
func TestPreservedNames(t *testing.T) {
	want := []string{".runner", ".credentials", ".env", ".path", "_work", "_diag"}

	got := PreservedNames()
	if !slices.Equal(got, want) {
		t.Fatalf("PreservedNames() が %v（期待 %v）", got, want)
	}
}

// 呼び出し側が書き換えても次の呼び出しに影響しないこと。
func TestPreservedNamesReturnsFreshSlice(t *testing.T) {
	first := PreservedNames()
	first[0] = "書き換え"

	if got := PreservedNames()[0]; got != ".runner" {
		t.Errorf("2 回目の PreservedNames()[0] が %q（期待 %q）", got, ".runner")
	}
}
