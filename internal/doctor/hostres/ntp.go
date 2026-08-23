package hostres

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
)

// ntpRemedy は同期を有効にする手順（表示のみ）。
const ntpRemedy = "timedatectl set-ntp true\n" +
	"systemctl restart systemd-timesyncd"

// impactNTP は時刻がずれたときの影響。
const impactNTP = "runner のトークン認証が失敗し、runner がオフラインになる場合があります。"

// offsetUnavailable はずれを取れなかった理由。
//
// timesyncd 以外の同期デーモンでも同期状態（NTPSynchronized）は取れるので、
// 「ずれだけが分からない」ことを利用者へそのまま伝える。
const offsetUnavailable = "systemd-timesyncd 以外の同期デーモン（chronyd 等）ではオフセットを取得できません"

// ntpCheck は NTP 同期状態と、取れるならシステム時計のずれを判定する。
//
// **ずれは外向きの通信なしに測れる。** systemd-timesyncd は直近に受け取った NTP
// パケットの往復時刻からオフセットを算出済みで保持しており、timedatectl は
// それを整形して出すだけである。doctor から NTP サーバへ問い合わせる必要は無い。
//
// **ただしオフセットは機械可読な出力には出ない。** D-Bus の
// org.freedesktop.timesync1.Manager が持つのは受信パケットそのもの（NTPMessage）で、
// オフセットという名前のプロパティは無く、`timedatectl show-timesync` は
// そのプロパティ一覧をそのまま出すだけなので Offset の行は現れない。計算済みの
// 値を出すのは人間向けの `timedatectl timesync-status` だけなので、そちらを読む。
//
// **取れない環境では同期状態だけを出す。** timesyncd が動いていないホスト
// （chronyd 運用など）では timesync-status 自体が失敗する。ずれが分からないことは
// 同期状態の判定を妨げないので、SKIP にはせず理由を Detail に書いて OK / FAIL を出す。
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

	offset, hasOffset := probeOffset(ctx, in)
	observed := props["TimeUSec"]

	if synced == "yes" {
		return one(check.Of(c, check.Result{
			Status:  check.OK,
			Summary: "NTP 同期済み",
			Detail: "システム時計は同期しています。観測時刻: " + observed +
				offsetDetail(offset, hasOffset),
		}))
	}

	detail := "systemd の時刻同期が有効になっていません（NTPSynchronized=" + synced + "）。"
	if props["NTP"] == "no" {
		detail += " NTP そのものが無効です（NTP=no）。"
	}
	detail += " 観測時刻: " + observed
	detail += offsetDetail(offset, hasOffset)

	summary := "NTP 未同期"
	if hasOffset {
		summary += "（ずれ " + formatOffset(offset) + "）"
	}

	return one(check.Of(c, check.Result{
		Status:  check.Fail,
		Summary: summary,
		Detail:  detail,
		Impact:  impactNTP,
		Remedy:  ntpRemedy,
	}))
}

// probeOffset は systemd-timesyncd が算出済みのオフセットを取る。
//
// 失敗を戻り値の false に畳んで err を返さないのは、呼び出し側にできることが
// 「ずれを出さない」しか無く、失敗の種類（コマンドが無い / timesyncd が動いて
// いない / 書式が違う）で分岐しないためである。理由は offsetUnavailable に
// まとめて Detail へ出す。
func probeOffset(ctx context.Context, in check.Input) (time.Duration, bool) {
	res, err := in.Probe(ctx, "doctor.time.offset", "timedatectl", "timesync-status", "--no-pager")
	if err != nil || res.ExitCode != 0 {
		return 0, false
	}
	return parseTimesyncOffset(res.Stdout)
}

// offsetPrefix は timesync-status がオフセットを出す行の見出し。
const offsetPrefix = "Offset:"

// parseTimesyncOffset は timesync-status の "Offset: +2.352ms" を解釈する。
//
// systemd の時間表記は Go の time.ParseDuration とほぼ同じで、違いは分の綴りが
// "min" であることと単位の間に空白が入ることだけである。そこだけ揃えて標準の
// パーサへ渡す（"+1min 5s" → "+1m5s"）。日以上の単位や spike 検出時の
// "(ignored)" は解釈できないが、いずれも「ずれ不明」に倒せばよいので独自の
// パーサは持たない。
func parseTimesyncOffset(stdout []byte) (time.Duration, bool) {
	for line := range strings.SplitSeq(string(stdout), "\n") {
		rest, found := strings.CutPrefix(strings.TrimSpace(line), offsetPrefix)
		if !found {
			continue
		}
		d, err := time.ParseDuration(strings.ReplaceAll(strings.ReplaceAll(rest, " ", ""), "min", "m"))
		if err != nil {
			return 0, false
		}
		return d, true
	}
	return 0, false
}

// offsetDetail は Detail の末尾へ足すずれの説明を返す。
//
// 向きを文で書いて符号を出さないのは、NTP のオフセットの符号（正ならローカルが
// 遅れている）が読み手に自明でないためである。
func offsetDetail(offset time.Duration, ok bool) string {
	switch {
	case !ok:
		return " ずれの秒数は取得できませんでした（" + offsetUnavailable + "）。"
	case offset > 0:
		return " システム時計は NTP サーバより " + formatOffset(offset) + " 遅れています。"
	case offset < 0:
		return " システム時計は NTP サーバより " + formatOffset(offset) + " 進んでいます。"
	default:
		return " システム時計のずれは検出されませんでした。"
	}
}

// formatOffset はずれの大きさを表示用にする。符号は含めない（向きは文で書く）。
//
// 1 秒未満をミリ秒で出すのは、同期直後の数ミリ秒を「0 秒」と丸めると、測れて
// いないのか合っているのかが区別できなくなるためである。
func formatOffset(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	if d >= time.Second {
		return strconv.FormatInt(int64((d+time.Second/2)/time.Second), 10) + " 秒"
	}
	return strconv.FormatInt(d.Milliseconds(), 10) + " ミリ秒"
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
