// Package hostres は環境診断（doctor）の 時刻 / リソース / 障害履歴 の 3 分類を
// 実装する（functional.md のチェック項目一覧表）。
//
// 3 分類を 1 つのパッケージにまとめてあるのは、いずれも「ホストそのものの状態」を
// 読むだけの項目であり、GitHub との通信も runner の構成の解釈も要らないためである。
// 分類ごとにパッケージを割ると、同じ /proc の読み方・同じ閾値の考え方・同じ
// 「測れなかったときは FAIL ではなく SKIP」という判断が 3 箇所へ散る。
//
// **本パッケージの項目はすべて Startup() == false である。** FR-44 の起動時判定は
// ホスト内の読み取りと軽量なコマンドで完結する項目に限られる。runner ごとの
// statfs（resource.fs）と journalctl の起動（history.oom）はそれを超え、起動の
// たびに走らせると runner 一覧が出るまでの時間がホストの状態に引きずられる。
package hostres

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/disk"
	"github.com/ousiassllc/gsr-helper/internal/exec"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
)

// Checks は本パッケージの診断項目を一覧の順で返す。
//
// 順を固定するのは、レジストリ（internal/doctor）が受け取った並びをそのまま
// 保持するためである。呼ぶたびに順が変わると、レジストリ側の一覧の並びが
// 実行ごとに揺れる（表示順の整列は doctor.Run が別途行うが、同順の項目の
// 相対順はここの並びで決まる）。
func Checks() []check.Check {
	return []check.Check{ntpCheck{}, fsCheck{stat: disk.FSStats}, memCheck{}, oomCheck{}}
}

// 各項目が check.Check を満たすことをコンパイル時に確かめる。
// 項目の型は非公開なので、満たしていないことに気付けるのはここだけである。
var (
	_ check.Check = ntpCheck{}
	_ check.Check = fsCheck{stat: nil}
	_ check.Check = memCheck{}
	_ check.Check = oomCheck{}
)

// バイト表記の単位。df / free と突き合わせられるよう 2 進接頭辞を使う。
const (
	kiB = int64(1024)
	miB = 1024 * kiB
	giB = 1024 * miB
)

// formatBytes は詳細（Detail）に出すバイト数を MiB / GiB の表記で返す。
//
// 生のバイト数のままだと桁が読めず、「あと何 GiB 空ければ閾値を下回るか」を
// 目で追えない。丸めた値なので足し合わせても合計と一致しないが、Detail は
// 判断材料であって集計ではないため、読みやすさを採る。
func formatBytes(n int64) string {
	switch {
	case n >= giB:
		return fmt.Sprintf("%.1f GiB", float64(n)/float64(giB))
	case n >= miB:
		return fmt.Sprintf("%.1f MiB", float64(n)/float64(miB))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// truncate は s を limit 文字までに切り詰める。切った場合は末尾に省略記号を付ける。
//
// バイトではなくルーンで数えるのは、途中で切ったマルチバイト文字が壊れた
// バイト列として詳細画面に出るのを避けるためである。
func truncate(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit]) + "…"
}

// one は結果 1 件をスライスにして返す。Run の戻りが複数を許す形なので、
// ホスト全体で 1 行の項目でも包む必要がある。
func one(r check.Result) []check.Result { return []check.Result{r} }

// probeFailure は失敗したコマンド実行の根拠を 1 行にまとめる。
//
// **標準出力は載せない。** 診断のコマンドの出力には権限情報や資格情報が
// 混ざりうるので、載せる側を最初から持たない形にしてある。
func probeFailure(res exec.Result, err error) string {
	if err != nil {
		return "コマンドを実行できませんでした: " + err.Error()
	}
	msg := "終了コード " + strconv.Itoa(res.ExitCode)
	if s := strings.TrimSpace(string(res.Stderr)); s != "" {
		msg += "、標準エラー出力: " + truncate(s, detailLineLimit)
	}
	return msg
}
