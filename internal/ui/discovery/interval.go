package discovery

import "time"

// Interval は自動更新間隔を決める。フラグ → 設定ファイル → 既定値の順に採る。
//
// 検出の deadline には使わない（Budget を参照）。表示を更新する間隔と、
// 1 回の検出に許す時間は別の関心事である。
func Interval(flag, conf time.Duration) time.Duration {
	d := flag
	if d <= 0 {
		d = conf
	}
	if d <= 0 {
		d = DefaultRefresh
	}
	return max(d, MinRefresh)
}
