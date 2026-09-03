package command

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ousiassllc/gsr-helper/internal/exec/mask"
)

const (
	// recordedMultiByte は 1 文字 3 バイトの文字だけを並べた素材。
	// 上限をバイト数で切ると必ず文字の途中に当たるので、断片を落とす経路を通る。
	recordedMultiByte = "あ"
	// recordedSecret は切り詰めの境界にまたがる位置へ置く秘密値。
	recordedSecret = "gsr-secret-SSSSSSSSSSSSSSSSSSSSSSSSSSSSS"
	// recordedSecretHead は recordedSecret のうち上限の内側に入る先頭。
	// 切り詰めをマスクより先に行うと、この断片が監査ログへそのまま載る。
	recordedSecretHead = "gsr-secret-SSSSSSSSS"
)

func TestTruncateTailMarksOnlyWhenCutting(t *testing.T) {
	// 印を必ず残すのは、短いメッセージと「切られた長いメッセージ」を読み手が
	// 区別できるようにするためである（limit.go の elisionSuffix の理由）。
	tests := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{"上限より短い", "abc", 8, "abc"},
		{"上限と同じ", "abcdefgh", 8, "abcdefgh"},
		{"上限を 1 バイト超える", "abcdefghi", 8, "abcdefgh" + elisionSuffix},
		{"上限が 0", "abc", 0, elisionSuffix},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateTail(tt.s, tt.n); got != tt.want {
				t.Errorf("truncateTail(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
			}
		})
	}

	// 切っていないのに印が付くと、読み手は続きがあると誤解する。
	if got := truncateTail("だめでした: 権限がありません", maxRecordedErrorBytes); strings.Contains(got, elisionSuffix) {
		t.Errorf("上限以下なのに省略の印が付いている: %q", got)
	}
}

func TestTruncateTailNeverLeavesBrokenUTF8(t *testing.T) {
	// バイト数で切ると多バイト文字の途中で切れる。壊れた文字を JSON に載せないため、
	// 末尾に残った不完全な断片は落としてから印を付ける（dropPartialRuneAtEnd）。
	const runes = 8
	s := strings.Repeat(recordedMultiByte, runes)
	size := len(recordedMultiByte)

	for n := range len(s) + 1 {
		got := truncateTail(s, n)
		if !utf8.ValidString(got) {
			t.Errorf("truncateTail(_, %d) が壊れた UTF-8 を残した: %q", n, got)
			continue
		}
		if strings.ContainsRune(got, utf8.RuneError) {
			t.Errorf("truncateTail(_, %d) に置換文字が混じった: %q", n, got)
		}
		if n >= len(s) {
			if got != s {
				t.Errorf("truncateTail(_, %d) = %q, want 素通り", n, got)
			}
			continue
		}
		// 切った場合は「上限に収まる最大の文字数」だけが残る。断片を落とす処理が
		// 無いと、ここが 1〜2 バイト多い壊れた文字列になる。
		want := strings.Repeat(recordedMultiByte, n/size) + elisionSuffix
		if got != want {
			t.Errorf("truncateTail(_, %d) = %q, want %q", n, got, want)
		}
	}
}

func TestRecordedErrorCapsAtMaxRecordedErrorBytes(t *testing.T) {
	// 監査ログは 1 レコード 1 行の JSONL なので、上限を置かないと 1 回の失敗が
	// 数 MB の 1 行になる。上限で切ることと、印が付くことを固定する。
	//
	// 上限の値そのものはリテラルで縛る。入力と期待値の両方を maxRecordedErrorBytes から
	// 導くと同じ定数で動くので、切り詰め長を縮める退行（例: 4096 → 64）でも緑のまま
	// 通ってしまう。4096 は docs/architecture/data-model.md が仕様として定めた値である。
	const documentedLimit = 4096
	if maxRecordedErrorBytes != documentedLimit {
		t.Fatalf("maxRecordedErrorBytes = %d, want %d（docs/architecture/data-model.md）",
			maxRecordedErrorBytes, documentedLimit)
	}

	long := strings.Repeat("x", documentedLimit+1)

	got := recordedError(errors.New(long), nil)

	// 切り位置が上限と一致すること（truncateTail の切り位置の退行を落とす）。
	if want := long[:documentedLimit] + elisionSuffix; got != want {
		t.Errorf("recordedError = %d バイト, want %d（上限で切って印を付けた形）", len(got), len(want))
	}

	// 日本語のエラー文が上限を超えると、上限は必ず文字の途中に当たる。
	// 監査レコードに載る文が壊れた UTF-8 になっていないことを実地で見る。
	jp := strings.Repeat(recordedMultiByte, maxRecordedErrorBytes/len(recordedMultiByte)+8)
	jpGot := recordedError(errors.New(jp), nil)
	if !utf8.ValidString(jpGot) {
		t.Error("監査レコードの error に壊れた UTF-8 が載った")
	}
	if !strings.HasSuffix(jpGot, elisionSuffix) {
		t.Error("日本語のエラー文で省略の印が付いていない")
	}
}

func TestRecordedErrorMasksBeforeTruncating(t *testing.T) {
	// マスクと切り詰めの順序が入れ替わると、上限にまたがった秘密値は先に断片へ
	// 割られ、値一致マスクでは拾えなくなって断片が監査ログへ残る。
	msg := strings.Repeat("P", maxRecordedErrorBytes-len(recordedSecretHead)) +
		recordedSecret + strings.Repeat("T", 64)

	got := recordedError(errors.New(msg), []string{recordedSecret})

	if strings.Contains(got, recordedSecret) {
		t.Error("秘密値がそのまま監査レコードに載っている")
	}
	if strings.Contains(got, recordedSecretHead) {
		t.Error("マスクより先に切り詰めている（秘密値の断片が残っている）")
	}
	if !strings.Contains(got, mask.Placeholder) {
		t.Error("マスクが効いていない（置換の跡が無い）")
	}
	if !strings.HasSuffix(got, elisionSuffix) {
		t.Error("上限を超えたのに省略の印が付いていない")
	}
}
