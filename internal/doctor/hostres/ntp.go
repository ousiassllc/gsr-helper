package hostres

import (
	"context"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
)

// ntpRemedy は同期を有効にする手順（表示のみ）。
const ntpRemedy = "timedatectl set-ntp true\n" +
	"systemctl restart systemd-timesyncd"

// impactNTP は時刻がずれたときの影響。
const impactNTP = "runner のトークン認証が失敗し、runner がオフラインになる場合があります。"

// ntpCheck は NTP 同期状態を判定する。
//
// **ずれの秒数は出さない。** screens.md のモックは「NTP 未同期（ずれ 42 秒）」と
// 書いているが、ずれを測るには外部の基準時刻が要る。doctor から NTP サーバへ
// 問い合わせるのは診断の責務を超える（外向きの通信を増やす）し、システム時計を
// システム時計と比べても常に 0 にしかならない。したがって同期状態だけを判定し、
// 観測時刻は Detail に出して利用者が自分の時計と突き合わせられるようにする。
type ntpCheck struct{}

func (ntpCheck) ID() string       { return "time.ntp" }
func (ntpCheck) Category() string { return check.CatTime }
func (ntpCheck) Startup() bool    { return false }

// Run はホスト全体で 1 行を返す。
func (c ntpCheck) Run(ctx context.Context, in check.Input) []check.Result {
	if !in.Has("timedatectl") {
		return one(check.Skipped(c, "NTP 同期状態",
			"timedatectl がありません（systemd 以外の時刻同期は判定できません）。"))
	}

	res, err := in.Probe(ctx, "doctor.time", "timedatectl", "show",
		"-p", "NTPSynchronized", "-p", "NTP", "-p", "TimeUSec")
	if err != nil || res.ExitCode != 0 {
		return one(check.Skipped(c, "NTP 同期状態",
			"timedatectl show を実行できませんでした（"+probeFailure(res, err)+"）。"))
	}

	props := parseProperties(res.Stdout)
	synced, ok := props["NTPSynchronized"]
	if !ok {
		return one(check.Skipped(c, "NTP 同期状態",
			"timedatectl show の出力に NTPSynchronized がありません。"))
	}

	observed := props["TimeUSec"]
	if synced == "yes" {
		return one(check.Of(c, check.Result{
			Status:  check.OK,
			Summary: "NTP 同期済み",
			Detail:  "システム時計は同期しています。観測時刻: " + observed,
		}))
	}

	detail := "systemd の時刻同期が有効になっていません（NTPSynchronized=" + synced + "）。"
	if props["NTP"] == "no" {
		detail += " NTP そのものが無効です（NTP=no）。"
	}
	detail += " 観測時刻: " + observed
	detail += " ずれの秒数は基準となる外部の時刻が無いため測っていません。"

	return one(check.Of(c, check.Result{
		Status:  check.Fail,
		Summary: "NTP 未同期",
		Detail:  detail,
		Impact:  impactNTP,
		Remedy:  ntpRemedy,
	}))
}

// parseProperties は KEY=value 形式の出力を map にする。
//
// systemctl show / timedatectl show はどちらもこの形式で返す。値に = を含む
// 場合があるので最初の 1 つだけで分割する。
func parseProperties(stdout []byte) map[string]string {
	out := make(map[string]string, 8)
	for line := range strings.SplitSeq(string(stdout), "\n") {
		k, v, found := strings.Cut(strings.TrimSpace(line), "=")
		if found && k != "" {
			out[k] = v
		}
	}
	return out
}
