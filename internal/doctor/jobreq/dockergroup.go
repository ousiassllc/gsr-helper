package jobreq

import (
	"context"
	"strconv"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// dockerGroupName は判定する補助グループの名前。
const dockerGroupName = "docker"

// groupRemedy はグループ追加の手順。**再起動まで含める**のがこの手順の要点で、
// usermod だけでは稼働中の runner に反映されない。
const groupRemedy = "sudo usermod -aG docker <user>\n" +
	"sudo systemctl restart 'actions.runner.*'   # 既存プロセスには反映されないので必須"

// restartRemedy は追加済みだが未反映のときの手順。usermod は要らない。
const restartRemedy = "sudo systemctl restart 'actions.runner.*'"

// dockerGroupCheck は runner 実行ユーザーの docker グループ所属を判定する（FR-43）。
//
// **所属しているだけでは足りない。** グループの変更は既存プロセスに反映されない
// ため、usermod 済みでも runner を再起動していなければ docker socket は
// permission denied のままである（runner-host-setup.md の「4. docker グループ」）。
// 実運用で最も気付きにくい不備なので、「未所属」と「未反映」を別の判定として
// 出し分ける（screens.md の Doctor タブの詳細の例）。
//
// **外部コマンドを使わない。** /etc/group と /proc の読み取りだけで完結するので、
// 起動時の自動判定（FR-44）に入れても起動を待たせない。
type dockerGroupCheck struct{}

func (dockerGroupCheck) ID() string       { return "job.dockergroup" }
func (dockerGroupCheck) Category() string { return check.CatJobReq }
func (dockerGroupCheck) Startup() bool    { return true }

// Run は runner ごとに 1 行を返す。
func (c dockerGroupCheck) Run(_ context.Context, in check.Input) []check.Result {
	body, err := in.ReadFile("/etc/group")
	if err != nil {
		return one(check.Skipped(c, "docker グループ所属",
			"/etc/group を読めませんでした: "+err.Error()))
	}

	gid, members, ok := dockerGroup(body)
	if !ok {
		return one(check.Skipped(c, "docker グループ所属",
			"docker グループが /etc/group にありません（docker が入っていない可能性があります）"))
	}

	tgts := targets(in.Runners)
	if len(tgts) == 0 {
		return one(check.Skipped(c, "docker グループ所属",
			"実行ユーザーの分かる runner がありません"))
	}

	out := make([]check.Result, 0, len(tgts))
	for _, r := range tgts {
		out = append(out, c.judge(in, r, gid, members))
	}
	return out
}

// judge は runner 1 台ぶんの判定を返す。
//
// 実行ユーザーの表記で経路が分かれる。UID の 10 進表記は /etc/group のメンバー欄
// （名前）と突き合わせられないためである。
func (c dockerGroupCheck) judge(in check.Input, r runner.Runner, gid string, members []string) check.Result {
	if numericUser(r.RunAsUser) {
		return c.judgeByProc(in, r, gid)
	}
	return c.judgeByName(in, r, gid, members)
}

// judgeByProc は UID の 10 進表記の runner を、稼働中プロセスの補助グループだけで判定する。
//
// **名前が解決できなくても判定を諦めない。** /proc/<pid>/status の Groups 行は
// GID の並びなので、docker の GID と直接比較できる。ここを一律 SKIP にすると、
// 仕様が最も想定している環境（NSS が使えない・LDAP 上のユーザー。data-model.md）
// でだけ FR-43 の 1 項目が常に未判定になる。
//
// **「未所属」と「未反映」は区別できない。** どちらも Groups 行に GID が現れない
// という同じ形になる。区別できないことを Detail に書いたうえで、対処は usermod を
// 含む側（groupRemedy）を出す。既に所属しているユーザーへの usermod は無害だが、
// 逆に未所属のユーザーを再起動しても直らないためである。
func (c dockerGroupCheck) judgeByProc(in check.Input, r runner.Runner, gid string) check.Result {
	note := numericUserNote(r.RunAsUser)

	if r.Listener == nil {
		return check.Of(c, check.Result{
			ID: "", Category: "", Target: r.Name(), Status: check.Skip,
			Summary: "docker グループ所属",
			Detail:  note + "稼働中の Runner.Listener も無いため、補助グループからも判定できません。",
			Impact:  "", Remedy: "", Startup: false,
		})
	}

	pid := r.Listener.PID
	groups, err := procGroups(in, pid)
	if err != nil {
		return check.Of(c, check.Result{
			ID: "", Category: "", Target: r.Name(), Status: check.Warn,
			Summary: "docker グループの反映を確認できない",
			Detail: note + "稼働中の Runner.Listener（PID " + strconv.Itoa(pid) +
				"）の補助グループも読めませんでした: " + err.Error(),
			Impact:  impactDockerSocket,
			Remedy:  numericUserRemedy(r.RunAsUser),
			Startup: false,
		})
	}

	if contains(groups, gid) {
		return check.Of(c, check.Result{
			ID: "", Category: "", Target: r.Name(), Status: check.OK,
			Summary: "docker グループに所属",
			Detail: note + "稼働中の Runner.Listener（PID " + strconv.Itoa(pid) +
				"）の補助グループに docker の GID " + gid + " があり、反映されています。",
			Impact: "", Remedy: "", Startup: false,
		})
	}

	return check.Of(c, check.Result{
		ID: "", Category: "", Target: r.Name(), Status: check.Fail,
		Summary: "docker グループが未反映",
		Detail: note + "稼働中の Runner.Listener（PID " + strconv.Itoa(pid) +
			"）の補助グループに docker の GID " + gid + " がありません。" +
			"未所属なのか、所属済みで既存プロセスに未反映なのかはここでは区別できません。",
		Impact:  impactDockerSocket,
		Remedy:  numericUserRemedy(r.RunAsUser),
		Startup: false,
	})
}

// numericUserNote は UID の 10 進表記のときに Detail の先頭へ必ず添える但し書き。
//
// どの判定に転んでも「/etc/group との突き合わせはしていない」ことを残す。残さないと、
// 補助グループだけを見た OK が「メンバー欄も確認済み」と読まれる。
func numericUserNote(user string) string {
	return "実行ユーザーが UID の 10 進表記（" + user + "）のため、" +
		"/etc/group のメンバー欄（ユーザー名）との突き合わせはできません。"
}

// numericUserRemedy は UID の 10 進表記のときのグループ追加手順を返す。
//
// usermod はログイン名しか受け付けないので、UID をそのまま埋めた行は貼っても通らない。
// 埋める先は <...> のまま残し、どの UID のユーザー名を書くのかだけを示す。
func numericUserRemedy(user string) string {
	return strings.ReplaceAll(groupRemedy, "<user>", "<UID "+user+" のユーザー名>")
}

// judgeByName は /etc/group のメンバー欄と突き合わせられる runner を判定する。
func (c dockerGroupCheck) judgeByName(in check.Input, r runner.Runner, gid string, members []string) check.Result {
	user := r.RunAsUser

	if !contains(members, user) {
		return check.Of(c, check.Result{
			ID: "", Category: "", Target: r.Name(), Status: check.Fail,
			Summary: "docker グループに未所属",
			Detail:  "ユーザー " + user + " は docker グループ（GID " + gid + "）に所属していません。",
			Impact:  impactDockerSocket,
			Remedy:  strings.ReplaceAll(groupRemedy, "<user>", user),
			Startup: false,
		})
	}

	// ここから先は「所属している」。稼働中のプロセスへ反映されているかを見る。
	if r.Listener == nil {
		return check.Of(c, check.Result{
			ID: "", Category: "", Target: r.Name(), Status: check.OK,
			Summary: "docker グループに所属",
			Detail: "ユーザー " + user + " は docker グループ（GID " + gid + "）に所属しています。" +
				"稼働中の Runner.Listener が無いため、既存プロセスへの反映状況は未確認です。",
			Impact: "", Remedy: "", Startup: false,
		})
	}

	pid := r.Listener.PID
	groups, err := procGroups(in, pid)
	if err != nil {
		return check.Of(c, check.Result{
			ID: "", Category: "", Target: r.Name(), Status: check.Warn,
			Summary: "docker グループの反映を確認できない",
			Detail: "ユーザー " + user + " は docker グループに所属していますが、" +
				"稼働中の Runner.Listener（PID " + strconv.Itoa(pid) + "）の補助グループを読めませんでした: " +
				err.Error(),
			Impact:  impactDockerSocket,
			Remedy:  restartRemedy,
			Startup: false,
		})
	}

	if contains(groups, gid) {
		return check.Of(c, check.Result{
			ID: "", Category: "", Target: r.Name(), Status: check.OK,
			Summary: "docker グループに所属",
			Detail: "ユーザー " + user + " は docker グループ（GID " + gid + "）に所属しており、" +
				"稼働中の Runner.Listener（PID " + strconv.Itoa(pid) + "）にも反映されています。",
			Impact: "", Remedy: "", Startup: false,
		})
	}

	return check.Of(c, check.Result{
		ID: "", Category: "", Target: r.Name(), Status: check.Fail,
		Summary: "docker グループが未反映",
		Detail: "ユーザー " + user + " は docker グループ（GID " + gid + "）に所属していますが、" +
			"稼働中の Runner.Listener（PID " + strconv.Itoa(pid) + "）の補助グループに反映されていません。" +
			"グループの変更は既存プロセスには適用されません。",
		Impact:  impactDockerSocket,
		Remedy:  restartRemedy,
		Startup: false,
	})
}

// dockerGroup は /etc/group から docker グループの GID とメンバーを取り出す。
//
// **os/user を経由せず /etc/group を直接読む。** 本ツールは NSS を使えない環境
// （静的リンク、LDAP 上のユーザー）を明示的に想定しており、RunAsUser が UID の
// 10 進表記になり得るのはそのためである（data-model.md）。同じ環境で
// os/user.LookupGroup は失敗するので、判定できない理由が「ホストの不備」と
// 区別できなくなる。行の形式は name:passwd:gid:member,member である。
func dockerGroup(body []byte) (gid string, members []string, ok bool) {
	for line := range strings.SplitSeq(string(body), "\n") {
		fields := strings.Split(strings.TrimSpace(line), ":")
		if len(fields) < 3 || fields[0] != dockerGroupName {
			continue
		}
		var list []string
		if len(fields) >= 4 {
			for m := range strings.SplitSeq(fields[3], ",") {
				if m = strings.TrimSpace(m); m != "" {
					list = append(list, m)
				}
			}
		}
		return fields[2], list, true
	}
	return "", nil, false
}

// procGroups は /proc/<pid>/status の Groups 行を GID の並びとして返す。
//
// 補助グループの実体はここにしか出ない。/etc/group は「そうあるべき状態」を
// 表すのに対し、こちらは「稼働中のプロセスが実際に持っている状態」である。
func procGroups(in check.Input, pid int) ([]string, error) {
	body, err := in.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return nil, err
	}
	for line := range strings.SplitSeq(string(body), "\n") {
		rest, found := strings.CutPrefix(line, "Groups:")
		if !found {
			continue
		}
		return strings.Fields(rest), nil
	}
	// Groups 行が無いのは補助グループを 1 つも持たない場合であり、読み取りの
	// 失敗ではない。空を返して「反映されていない」と判定させる。
	return nil, nil
}

// contains は list に v があるかを返す。
func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
