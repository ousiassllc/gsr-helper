package jobreq

import (
	"bytes"
	"context"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// nopasswd は `sudo -l -U <user>` の出力に現れる、パスワード不要の印。
const nopasswd = "NOPASSWD"

// sudoCheck は runner 実行ユーザーのパスワード不要 sudo を見る（FR-43）。
//
// `setup-*` 系アクションが `sudo install` で /usr/local/bin へバイナリを置く
// ため、欠けるとジョブが `sudo: パスワードが必要です` で失敗する。
//
// runner ごとに 1 行出す。実行ユーザーは runner ごとに違いうるので、ホスト
// 全体で 1 行にまとめると、どの runner のジョブが落ちるのかが読めなくなる。
type sudoCheck struct{}

func (sudoCheck) ID() string       { return "job.sudo" }
func (sudoCheck) Category() string { return check.CatJobReq }
func (sudoCheck) Startup() bool    { return true }

// Run は runner ごとにパスワード不要 sudo の有無を判定する。
//
// **欠落は WARN であり FAIL にしない。** `NOPASSWD: ALL` の付与はその runner で
// 走る任意のワークフローに実質 root を与えることを意味し、必要かどうかは
// ワークフローの内容で決まる。ツールが一律に「不備」と断定せず可否は運用者に
// 委ねる、と docs/architecture/security.md の「判定の強さ」が定めている。
func (c sudoCheck) Run(ctx context.Context, in check.Input) []check.Result {
	rs := targets(in.Runners)
	if len(rs) == 0 {
		return nil
	}
	if !in.Has("sudo") {
		return one(check.Skipped(c,
			"sudo が無いため未判定",
			"sudo コマンドが PATH 上にありません。パスワード不要 sudo の有無は判定していません。"))
	}

	out := make([]check.Result, 0, len(rs))
	for _, r := range rs {
		out = append(out, c.judge(ctx, in, r))
	}
	return out
}

// judge は runner 1 台を判定する。
func (c sudoCheck) judge(ctx context.Context, in check.Input, r runner.Runner) check.Result {
	// **出力は監査ログに残らない。** Probe は SkipAudit を立てないが、
	// audit.Record が出力の欄を持たないため、記録されるのは実行の事実と
	// 終了コードだけである（check.Input.Probe の doc、security.md の
	// 「パスワード不要 sudo の要求への対応」の末尾）。権限情報を含む
	// `sudo -l -U` の出力をここへ通せるのはその保証があるからである。
	res, err := in.Probe(ctx, "doctor.sudo", "sudo", "-l", "-U", sudoUser(r.RunAsUser))
	if noExecutor(err) {
		return check.Of(c, check.Result{
			Target:  r.Name(),
			Status:  check.Skip,
			Summary: "コマンドを実行できないため未判定",
			Detail:  "外部コマンドの実行経路が配られていないため `sudo -l -U` を発行していません。",
		})
	}
	if err != nil || res.ExitCode != 0 {
		return check.Of(c, check.Result{
			Target:  r.Name(),
			Status:  check.Warn,
			Summary: "パスワード不要 sudo を判定できなかった",
			Detail: "ユーザー " + r.RunAsUser + " について `sudo -l -U` が失敗しました（" +
				probeFailure(res, err) + "）。",
			Impact: impactSudo,
			Remedy: "`sudo -l -U " + sudoUser(r.RunAsUser) + "` を手で実行し、失敗の理由を確認してください。",
		})
	}
	// 出力そのものは Detail に載せない。画面にもログにも権限の一覧を写さず、
	// 判定の結果だけを出す。
	if bytes.Contains(res.Stdout, []byte(nopasswd)) {
		return check.Of(c, check.Result{
			Target:  r.Name(),
			Status:  check.OK,
			Summary: "パスワード不要 sudo がある",
			Detail:  "ユーザー " + r.RunAsUser + " の sudo 設定に " + nopasswd + " の指定があります。",
		})
	}
	return check.Of(c, check.Result{
		Target:  r.Name(),
		Status:  check.Warn,
		Summary: "パスワード不要 sudo が無い",
		Detail: "ユーザー " + r.RunAsUser + " の sudo 設定に " + nopasswd +
			" の指定がありません。",
		Impact: impactSudo,
		Remedy: sudoRemedy(r.RunAsUser),
	})
}

// impactSudo はパスワード不要 sudo が欠けたときにジョブへ出る症状。
// docs/operations/runner-host-setup.md の「欠けているものと症状」から採る。
const impactSudo = "`sudo install` を使うアクション（setup-atlas など）が " +
	"`sudo: パスワードが必要です`（`sudo: a password is required`）で失敗します。"

// sudoUser は `sudo -l -U` に渡すユーザーの表記を返す。
//
// **RunAsUser はユーザー名とは限らない。** 名前を解決できない環境（静的リンクで
// NSS が使えない、LDAP 上のユーザー）では UID の 10 進表記になる
// （docs/architecture/data-model.md）。`sudo -l -U` は UID を `#1001` の形式で
// しか受け付けないため、数値には `#` を付ける。付けずに渡すと「そんなユーザーは
// 居ない」という失敗になり、**NOPASSWD が無い場合と区別できなくなる**
// （FR-43、security.md「パスワード不要 sudo の要求への対応」）。
func sudoUser(runAsUser string) string {
	if numericUser(runAsUser) {
		return "#" + runAsUser
	}
	return runAsUser
}

// sudoRemedy は付与の手順を返す。**表示するだけで実行はしない。**
//
// sudoers を壊すと sudo 自体が使えなくなり、その端末からの復旧手段を失う。
// だから `visudo -c` による検証を手順に含め、書き換えは運用者の手に委ねる
// （security.md「対処」、docs/operations/runner-host-setup.md の手順 1）。
// user は表示専用の文字列であり、ここから外部コマンドを起動する経路は無い。
func sudoRemedy(user string) string {
	return "echo '" + user + " ALL=(ALL) NOPASSWD: ALL' | sudo tee /etc/sudoers.d/github-runner\n" +
		"sudo chmod 0440 /etc/sudoers.d/github-runner\n" +
		"sudo visudo -c        # parsed OK を確認してから端末を閉じる\n" +
		"\n" +
		"付与はこの runner で走る任意のワークフローに実質 root を与えます。" +
		"可能なら ALL ではなく必要なコマンドに限定してください。"
}
