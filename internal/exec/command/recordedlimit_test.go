package command

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ousiassllc/gsr-helper/internal/exec/command/limit"
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

	// 切り位置が上限と一致すること（limit.Tail の切り位置の退行を落とす）。
	if want := long[:documentedLimit] + limit.Suffix; got != want {
		t.Errorf("recordedError = %d バイト, want %d（上限で切って印を付けた形）", len(got), len(want))
	}

	// 日本語のエラー文が上限を超えると、上限は必ず文字の途中に当たる。
	// 監査レコードに載る文が壊れた UTF-8 になっていないことを実地で見る。
	jp := strings.Repeat(recordedMultiByte, maxRecordedErrorBytes/len(recordedMultiByte)+8)
	jpGot := recordedError(errors.New(jp), nil)
	if !utf8.ValidString(jpGot) {
		t.Error("監査レコードの error に壊れた UTF-8 が載った")
	}
	if !strings.HasSuffix(jpGot, limit.Suffix) {
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
	if !strings.HasSuffix(got, limit.Suffix) {
		t.Error("上限を超えたのに省略の印が付いていない")
	}
}
