package hostres

import (
	"context"
	"strconv"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
)

// 空きメモリの割合の閾値。
//
// swap が無いホストでは空きが尽きた瞬間に OOM Killer が走るため、10% を切ったら
// FAIL、20% で WARN とする。ビルドの一時的な山を吸収できる余地の目安である。
const (
	memFailPercent = 10
	memWarnPercent = 20
)

// impactMem はメモリが足りないときの影響。
const impactMem = "メモリ不足でジョブが OOM Killer に停止され、原因の分かりにくい失敗になります。"

// memCheck はメモリと swap の余裕を判定する。
type memCheck struct{}

func (memCheck) ID() string       { return "resource.mem" }
func (memCheck) Category() string { return check.CatResource }
func (memCheck) Startup() bool    { return false }

// Run はホスト全体で 1 行を返す。
//
// メモリと swap を 1 行にまとめるのは、判断が「このホストでジョブを走らせて
// 大丈夫か」の 1 つだからである。swap の有無だけを別の行にすると、余裕のある
// ホストでも常に 1 行の注意が residual に残る。
func (c memCheck) Run(_ context.Context, in check.Input) []check.Result {
	body, err := in.ReadFile("/proc/meminfo")
	if err != nil {
		return one(check.Skipped(c, "メモリと swap の余裕",
			"/proc/meminfo を読めませんでした: "+err.Error()))
	}

	info := parseMeminfo(body)
	total, okTotal := info["MemTotal"]
	avail, okAvail := info["MemAvailable"]
	if !okTotal || !okAvail || total <= 0 {
		return one(check.Skipped(c, "メモリと swap の余裕",
			"/proc/meminfo に MemTotal / MemAvailable がありません。"))
	}

	free := int(avail * 100 / total)
	swapTotal := info["SwapTotal"]

	status := check.OK
	switch {
	case free < memFailPercent:
		status = check.Fail
	case free < memWarnPercent:
		status = check.Warn
	}

	detail := "空きメモリ " + formatBytes(avail*kiB) + " / " + formatBytes(total*kiB) +
		"（" + pct(free) + "）。"
	if swapTotal == 0 {
		// swap が無いこと自体は設定の選択だが、余裕が無いときの緩衝が無い。
		// 判定を 1 段引き上げるだけにして、行は増やさない。
		detail += " swap がありません（空きが尽きた時点で OOM Killer が走ります）。"
		status = worse(status, check.Warn)
	} else {
		detail += " swap 空き " + formatBytes(info["SwapFree"]*kiB) +
			" / " + formatBytes(swapTotal*kiB) + "。"
	}

	if status == check.OK {
		return one(check.Of(c, check.Result{
			Status:  check.OK,
			Summary: "メモリに余裕がある",
			Detail:  detail,
		}))
	}
	return one(check.Of(c, check.Result{
		Status:  status,
		Summary: "空きメモリ " + pct(free),
		Detail:  detail,
		Impact:  impactMem,
		Remedy:  "メモリの増設、ジョブの並列度の削減、swap の追加のいずれかを検討してください。",
	}))
}

// parseMeminfo は /proc/meminfo を kB 単位の値の map にする。
//
// 行の形式は「MemTotal:       16316412 kB」である。単位は kB 固定なので
// 読み替えず、表示のときに 1024 倍する。
func parseMeminfo(body []byte) map[string]int64 {
	out := make(map[string]int64, 8)
	for line := range strings.SplitSeq(string(body), "\n") {
		key, rest, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		v, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			continue
		}
		out[key] = v
	}
	return out
}
