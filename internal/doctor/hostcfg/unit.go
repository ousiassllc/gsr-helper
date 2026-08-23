package hostcfg

import (
	"context"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// unitCheck は systemd ユニットと runner の整合を判定する。
//
// **Restart と Environment は systemd.State に無い。** 検出（internal/runner）が
// 一覧と状態表示のために引いているのは Load / Active / Sub / FileState /
// WorkingDirectory / User / MainPID だけである。診断のためだけに検出側の
// systemctl show の項目を増やすと、3 秒ごとのポーリングが全 runner ぶん重くなる。
// 診断は利用者が明示的に起こすものなので、要る値はここで別に引く。
type unitCheck struct{}

func (unitCheck) ID() string       { return "systemd.unit" }
func (unitCheck) Category() string { return check.CatSystemd }
func (unitCheck) Startup() bool    { return false }

// Run は systemd 管理の runner ごとに 1 行を返す。
func (c unitCheck) Run(ctx context.Context, in check.Input) []check.Result {
	if !in.Caps.Systemd {
		return one(check.Skipped(c, "ユニットの整合", "systemctl がありません。"))
	}

	managed := systemdManaged(in.Runners)
	if len(managed) == 0 {
		return one(check.Skipped(c, "ユニットの整合",
			"systemd で管理されている runner がありません。"))
	}

	out := make([]check.Result, 0, len(managed))
	for _, r := range managed {
		out = append(out, c.judge(ctx, in, r))
	}
	return out
}

// judge は runner 1 台ぶんの判定を返す。
func (c unitCheck) judge(ctx context.Context, in check.Input, r runner.Runner) check.Result {
	unit := unitName(r)

	res, err := in.Probe(ctx, "doctor.systemd", "systemctl", "show", unit, "--no-pager",
		"-p", "Restart", "-p", "WorkingDirectory", "-p", "Environment")
	if err != nil || res.ExitCode != 0 {
		return check.Of(c, check.Result{
			Target:  r.Name(),
			Status:  check.Skip,
			Summary: "ユニットの整合",
			Detail:  unit + " の設定を取得できませんでした。",
		})
	}

	props := parseProperties(res.Stdout)
	restart := props["Restart"]
	workDir := strings.TrimPrefix(props["WorkingDirectory"], "-")

	// WorkingDirectory の食い違いは、ユニットが別の runner のディレクトリで
	// 起動していることを意味する。一覧の紐付けと実態がずれるので FAIL。
	if workDir != "" && workDir != r.Dir {
		return check.Of(c, check.Result{
			Target:  r.Name(),
			Status:  check.Fail,
			Summary: "ユニットの WorkingDirectory が違う",
			Detail: unit + " の WorkingDirectory は " + workDir +
				" ですが、runner のディレクトリは " + r.Dir + " です。",
			Impact: "ユニットを再起動すると別のディレクトリの runner が起動します。",
			Remedy: "ユニットの WorkingDirectory を " + r.Dir + " に直すか、runner を登録し直してください。",
		})
	}

	// Restart が無いと、落ちた runner がそのまま戻らない。
	if restart == "" || restart == "no" {
		return check.Of(c, check.Result{
			Target:  r.Name(),
			Status:  check.Warn,
			Summary: "ユニットに Restart が設定されていない",
			Detail:  unit + " の Restart は " + orNone(restart) + " です。" + environmentNote(props),
			Impact:  "runner が異常終了したまま復帰せず、ジョブが queued のまま滞留します。",
			Remedy: "drop-in で Restart=always を設定してください" +
				"（本体ユニットは編集せず drop-in を作ります）。",
		})
	}

	return check.Of(c, check.Result{
		Target:  r.Name(),
		Status:  check.OK,
		Summary: "ユニットの設定は整合している",
		Detail:  unit + ": Restart=" + restart + ", WorkingDirectory=" + orNone(workDir) + "。" + environmentNote(props),
	})
}

// environmentNote はユニットの Environment を Detail へ添える文を返す。
//
// **Environment を理由に WARN / FAIL へ倒す経路は意図的に持たない。** 値の是非
// （プロキシ設定や PATH が runner の想定と噛み合っているか）は runner の .env と
// 突き合わせないと決まらず、.env の読み取りと解釈は internal/config の責務で
// 本パッケージのスコープ外である。突合の材料を持たないまま判定を出すと誤報に
// なるので、ここは値を提示するだけに留め、是非は運用者に委ねる。
// 判定を足すのは .env を読む側（設定編集）に材料が揃ってからである。
func environmentNote(props map[string]string) string {
	env := strings.TrimSpace(props["Environment"])
	if env == "" {
		return " Environment は設定されていません。"
	}
	return " Environment: " + env + "（内容の是非は判定していません）。"
}

// systemdManaged は systemd で管理されていてユニット名の分かる runner を返す。
func systemdManaged(runners []runner.Runner) []runner.Runner {
	out := make([]runner.Runner, 0, len(runners))
	for _, r := range runners {
		if r.Managed == runner.ManagedSystemd && unitName(r) != "" {
			out = append(out, r)
		}
	}
	return out
}

// unitName は実際に紐付いたユニット名を返す。
//
// Svc を先に見るのは、.service に記録された名前（UnitName）が古いことが
// あるためである。実態は紐付いた側にある。
func unitName(r runner.Runner) string {
	if r.Svc != nil && r.Svc.Unit != "" {
		return r.Svc.Unit
	}
	return r.UnitName
}

// orNone は空文字を「未設定」と表記する。
func orNone(v string) string {
	if v == "" {
		return "未設定"
	}
	return v
}

// parseProperties は KEY=value 形式の出力を map にする。
// 値に = を含む場合があるので最初の 1 つだけで分割する。
func parseProperties(stdout []byte) map[string]string {
	out := make(map[string]string, 4)
	for line := range strings.SplitSeq(string(stdout), "\n") {
		k, v, found := strings.Cut(strings.TrimSpace(line), "=")
		if found && k != "" {
			out[k] = v
		}
	}
	return out
}
