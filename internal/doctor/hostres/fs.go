package hostres

import (
	"context"
	"strconv"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/disk"
	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
)

// tmpPath はジョブが一時ファイルを置く場所。runner のディレクトリとは
// 別のファイルシステムであることが多いので独立に見る。
const tmpPath = "/tmp"

// impactFS は残量が尽きたときの影響。
const impactFS = "ジョブのチェックアウトやビルドが `no space left on device` で失敗します。" +
	"inode の枯渇は容量に余裕があっても起きます。"

// fsCheck はディスク残量と inode 残量を判定する。
//
// statfs は stat を差し替えて検査する。実ホストの空き容量に判定を依存させると、
// 同じコードが CI の環境次第で OK にも FAIL にもなる。
type fsCheck struct {
	stat func(path string) (disk.Stats, error)
}

func (fsCheck) ID() string       { return "resource.fs" }
func (fsCheck) Category() string { return check.CatResource }
func (fsCheck) Startup() bool    { return false }

// target は集計するパスと、それを持つ runner。
type target struct {
	path   string
	runner string
}

// Run はファイルシステムごとに 1 行を返す。
//
// 閾値は先頭で 1 度だけ解決して全パスへ配る。行ごとに既定へ落とす判断を散らすと、
// 1 回の実行の中で行によって違う閾値を見る形を許してしまう。
func (c fsCheck) Run(_ context.Context, in check.Input) []check.Result {
	th := in.DiskThresholds.OrDefault()
	targets := fsTargets(in)
	out := make([]check.Result, 0, len(targets))
	for _, t := range targets {
		out = append(out, c.judge(t, th))
	}
	return out
}

// judge はパス 1 つぶんの判定を返す。th は解決済みの閾値。
func (c fsCheck) judge(t target, th appconfig.DiskThresholds) check.Result {
	st, err := c.stat(t.path)
	if err != nil {
		return check.Of(c, check.Result{
			Target:  t.runner,
			Status:  check.Skip,
			Summary: t.path + " の残量",
			Detail:  "ファイルシステム情報を取得できませんでした: " + err.Error(),
		})
	}

	used, inodes := st.UsedPercent(), st.InodePercent()
	status := worse(band(used, th), band(inodes, th))
	detail := t.path + " の使用率 " + pct(used) + "（空き " + formatBytes(st.AvailBytes) + "）、" +
		"inode 使用率 " + pct(inodes) + "（空き " + strconv.FormatInt(st.FreeInodes, 10) + "）。"

	if status == check.OK {
		return check.Of(c, check.Result{
			Target:  t.runner,
			Status:  check.OK,
			Summary: t.path + " の残量は十分",
			Detail:  detail,
		})
	}
	return check.Of(c, check.Result{
		Target:  t.runner,
		Status:  status,
		Summary: t.path + " の使用率 " + pct(max(used, inodes)),
		Detail:  detail,
		Impact:  impactFS,
		Remedy:  "Disk タブ（[3]）で不要な _work / _tool / _diag を整理してください。",
	})
}

// fsTargets は集計するパスを重複なく返す。
//
// 並びは runner の検出順に従い、/tmp を最後に置く。map の走査順に任せると
// 実行のたびに行が入れ替わる。
func fsTargets(in check.Input) []target {
	seen := make(map[string]bool, len(in.Runners)+1)
	out := make([]target, 0, len(in.Runners)+1)

	add := func(path, name string) {
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		out = append(out, target{path: path, runner: name})
	}
	for _, r := range in.Runners {
		add(r.Dir, r.Name())
		add(r.WorkDir, r.Name())
	}
	add(tmpPath, "")
	return out
}

// band は使用率を判定に写す。th はゼロ値を含まない解決済みの閾値。
//
// **設定ファイルの disk_thresholds に従う。** ここに独立の閾値を持つと、設定を
// 変えた運用者が Disk タブの `⚠ 警告閾値超過` と doctor の判定で違うことを言われ、
// 設定に書いた数字が何を動かすのか読み取れなくなる（Issue #89）。
//
// 既定値（80 / 90。appconfig の defaultDiskWarn / defaultDiskCritical）がこの値
// なのは、ジョブ 1 本のチェックアウトとビルドで数 GiB を消費することがあり、
// 80% を切った時点で手を打たないと次のジョブで詰まるためである。
//
// 比較を「以上」にしているのは Disk タブの要約行（page/disk の warnExceeded）に
// 合わせるためで、同じ使用率に対して 2 つの画面が違うことを言わないようにしている。
func band(percent int, th appconfig.DiskThresholds) check.Status {
	switch {
	case percent >= th.Critical:
		return check.Fail
	case percent >= th.Warn:
		return check.Warn
	default:
		return check.OK
	}
}

// worse は重い方の判定を返す。Status は深刻度の昇順に並べてある。
func worse(a, b check.Status) check.Status {
	if a > b {
		return a
	}
	return b
}

// pct は百分率の表記を返す。
func pct(v int) string { return strconv.Itoa(v) + "%" }
