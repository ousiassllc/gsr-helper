package atom

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// Duration は経過時間を一覧の桁に収まる表記で返す。
//
// 1 分未満は秒、1 時間未満は分秒、それ以上は時分にする。桁数を抑えるのは
// ELAPSED 列と JOB 列の幅を固定するためである。負の値は不正な計測結果として
// 記号のみを返す（未取得と同じ扱いにする）。
func Duration(d time.Duration) string {
	if d < 0 {
		return token.IconNoUnit
	}

	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
}

// VersionText は現行バージョンを、素の文字列と表示上の役割の組で返す。
// 最新と異なる場合は注意記号を添える。
//
// latest が空のときは注意記号を付けない。最新バージョンの取得は非同期であり、
// 未取得の状態を「古い」と誤って示さないためである。装飾済みの文字列を
// 返さない理由は StatusText と同じで、列幅に切り詰めてから装飾させるためである。
func VersionText(cur, latest string) (text string, role token.RoleToken) {
	if cur == "" {
		return token.IconNoUnit, token.RoleMuted
	}
	if latest == "" || latest == cur {
		return cur, token.RolePlain
	}
	return cur + " " + token.Icon(token.StateWarn), token.StateWarn.Role()
}

// バイト数の表記に使う定数。
const (
	// byteUnit は 2 進接頭辞の刻み。ディスク使用量は df / du が 1024 刻みで
	// 報告するため、10 進の 1000 刻みにすると照合したときに数字が合わない。
	byteUnit = 1024
	// byteCarry は小数 1 桁に丸めた結果が次の単位へ届く境界。
	//
	// 1023.95 以上は "%.1f" が "1024.0" と出すため、単位を 1 段上げて "1.0M" にする。
	// 境界を置かないと 1MiB に 1 バイト足りない値が "1024.0K" と表示され、
	// 次の行の "1.0M" より大きい値に見える。
	byteCarry = 1023.95
)

// Bytes はバイト数を一覧の桁に収まる 2 進接頭辞の表記で返す（24.1G / 102.4M）。
//
// 1024 未満はバイトそのまま（512B）、それ以上は小数 1 桁を添える。桁数を抑えるのは
// SIZE 列の幅を固定するためであり、丸めた値を足し合わせても合計と一致しない点は
// 許容する（合計は集計側が生の値で計算し、表示のときだけここを通す）。
//
// 負の値は不正な集計結果として「値なし」の記号を返す（Duration と同じ扱い）。
// 集計前・集計不能を -1 で表せるようにするためである。
func Bytes(n int64) string {
	if n < 0 {
		return token.IconNoUnit
	}
	if n < byteUnit {
		return strconv.FormatInt(n, 10) + "B"
	}

	// P まで置く。int64 の最大値（8 EiB 弱）でも 8192.0P の 7 セルに収まり、
	// SIZE 列を最も狭く取る Logs タブの幅（token.SizeColumnWidth）を超えない。
	// T で打ち切ると同じ値が 8388608.0T の 10 セルになり、列から溢れる。
	suffixes := []string{"K", "M", "G", "T", "P"}
	v := float64(n) / byteUnit
	i := 0
	for i < len(suffixes)-1 && v >= byteCarry {
		v /= byteUnit
		i++
	}
	return strconv.FormatFloat(v, 'f', 1, 64) + suffixes[i]
}

// Files はファイル数を 3 桁区切りで返す（412,003）。
//
// 区切りを入れるのは、6 桁を超える件数を桁数で読み取れるようにするためである。
// 負の値は「値なし」の記号を返す。docker のイメージやビルドキャッシュのように
// ファイル数を持たない対象があり、0 件と区別する必要がある。
func Files(n int64) string {
	if n < 0 {
		return token.IconNoUnit
	}

	digits := strconv.FormatInt(n, 10)
	// 先頭から数えると区切りの位置が桁数に依存する。末尾から 3 桁ごとに置く。
	var b strings.Builder
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Ratio は使用率を、素の文字列と表示上の役割の組で返す。warn のときは注意記号を添える。
//
// 装飾済みの文字列を返さない理由は StatusText / VersionText と同じで、呼び出し側が
// 「幅に収めてから装飾する」順序を守れるようにするためである。
//
// 閾値を超えたかの判断はしない。閾値は設定（appconfig.DiskThresholds）で変わるため、
// 判断は値と設定の両方を持つ page の責務であり、atom は結果を描くだけにする。
//
// 範囲外の値は 0〜100 に丸める。統計値の取得元（df の 1 ブロック単位の丸め）によっては
// 101% が返ることがあり、そのまま出すと「100% を超える使用率」という読めない表示になる。
func Ratio(pct int, warn bool) (text string, role token.RoleToken) {
	pct = min(max(pct, 0), 100)

	text = strconv.Itoa(pct) + "%"
	if warn {
		return text + " " + token.Icon(token.StateWarn), token.StateWarn.Role()
	}
	return text, token.RolePlain
}
