package disk

import (
	"fmt"
	"math"
	"syscall"
)

// FSStats は path が載っているファイルシステムの容量と inode の残量を返す（FR-29）。
//
// df / du といった外部コマンドを呼ばずに statfs(2) を直接使うのは、外部プロセスの
// 起動と出力解析を挟まずに済み、ロケールや df の実装差で表示が変わらないためである。
// 取得できない場合は日本語のエラーを返す（呼び出し側はファイルシステム行を出さずに縮退する）。
func FSStats(path string) (Stats, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return Stats{}, fmt.Errorf("%s のファイルシステム情報の取得に失敗しました: %w", path, err)
	}

	bsize := int64(st.Bsize)
	blocks := toInt64(st.Blocks)
	bfree := toInt64(st.Bfree)
	bavail := toInt64(st.Bavail)
	files := toInt64(st.Files)
	ffree := toInt64(st.Ffree)

	return Stats{
		Path:       path,
		TotalBytes: blocks * bsize,
		// Bfree（root を含めた空き）を引くことで、予約ブロックを使用済みに数えない。
		// 使用率の分母は AvailBytes との和になるので df と同じ値になる（UsedPercent）。
		UsedBytes:   (blocks - bfree) * bsize,
		AvailBytes:  bavail * bsize,
		TotalInodes: files,
		UsedInodes:  files - ffree,
		FreeInodes:  ffree,
	}, nil
}

// toInt64 は statfs の符号なし値を int64 に収める。
// int64 に収まらない値は上限で飽和させる。負値に化けさせると使用率が 0 や負になり、
// 枯渇しているのに正常に見えるという最も避けたい誤表示になるためである。
func toInt64(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}
