package authz

import (
	"context"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
)

// hidepidRemedy は /proc を隠して再マウントする手順（表示のみ）。
//
// 恒久化には /etc/fstab の編集が要るので両方を出す。実行はしない。
const hidepidRemedy = "sudo mount -o remount,hidepid=2 /proc\n" +
	"# 恒久化: /etc/fstab の proc の行へ hidepid=2 を加える"

// hidepid は /proc の hidepid 設定を判定する。
//
// 未設定だと、runner の登録時にプロセス引数として渡る短命トークンを他ユーザーが
// /proc/<pid>/cmdline から読み取れる（security.md「プロセス引数からのトークン
// 読み取り（既知の制約）」）。本ツールはトークンを引数で渡す経路を持つため、
// この設定は本ツール自身の安全性にも効く。
type hidepid struct{}

func (hidepid) ID() string       { return "authz.hidepid" }
func (hidepid) Category() string { return check.CatAuthz }
func (hidepid) Startup() bool    { return false }

// Run はホスト全体で 1 行を返す。
func (c hidepid) Run(_ context.Context, in check.Input) []check.Result {
	body, err := in.ReadFile("/proc/mounts")
	if err != nil {
		return []check.Result{check.Skipped(c, "/proc の hidepid 設定",
			"/proc/mounts を読めませんでした: "+err.Error())}
	}

	opts, ok := procMountOptions(body)
	if !ok {
		return []check.Result{check.Skipped(c, "/proc の hidepid 設定",
			"/proc/mounts に proc ファイルシステムの行がありません。")}
	}

	value := optionValue(opts, "hidepid")
	switch value {
	case "2", "invisible":
		return []check.Result{check.Of(c, check.Result{
			Status:  check.OK,
			Summary: "/proc は hidepid=" + value + " でマウントされている",
			Detail:  "他ユーザーのプロセス情報は見えません。",
		})}
	case "1", "noaccess":
		return []check.Result{check.Of(c, check.Result{
			Status:  check.Warn,
			Summary: "/proc の hidepid が " + value + " にとどまる",
			Detail: "hidepid=" + value + " では /proc/<pid> の一覧は隠れますが、" +
				"PID を指定した参照までは塞げません。",
			Impact: impactHidepid,
			Remedy: hidepidRemedy,
		})}
	default:
		return []check.Result{check.Of(c, check.Result{
			Status:  check.Warn,
			Summary: "/proc に hidepid が設定されていない",
			Detail:  "/proc のマウントオプション: " + strings.Join(opts, ","),
			Impact:  impactHidepid,
			Remedy:  hidepidRemedy,
		})}
	}
}

// impactHidepid は hidepid が無いことの影響。
const impactHidepid = "runner の登録時にプロセス引数として渡る短命トークンを、" +
	"同じホストの他のユーザーが /proc/<pid>/cmdline から読み取れます。"

// procMountOptions は /proc/mounts から proc ファイルシステムのオプションを返す。
//
// 行の形式は「デバイス マウント先 種別 オプション dump pass」である。
// 種別で突き合わせるのは、マウント先が /proc とは限らないためではなく、
// 同じ /proc に別の種別が重ねられている場合に最後の 1 つを採るためである。
func procMountOptions(body []byte) (opts []string, ok bool) {
	for line := range strings.SplitSeq(string(body), "\n") {
		f := strings.Fields(line)
		if len(f) < 4 || f[2] != "proc" || f[1] != "/proc" {
			continue
		}
		opts, ok = strings.Split(f[3], ","), true
	}
	return opts, ok
}

// optionValue はマウントオプションの並びから name= の値を返す。
// name が値を持たない形（フラグ）なら空文字を返す。
func optionValue(opts []string, name string) string {
	for _, o := range opts {
		if v, found := strings.CutPrefix(o, name+"="); found {
			return v
		}
	}
	return ""
}
