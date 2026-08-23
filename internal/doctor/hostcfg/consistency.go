package hostcfg

import (
	"context"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/systemd"
)

// notFound は systemctl が「ユニットファイルが無い」ことを表す LoadState。
const notFound = "not-found"

// orphanCheck は孤児ユニット（FR-05）を判定する。
//
// 孤児は runner.Discover も検出しているが、check.Input が配られるのは runner
// 一覧だけで孤児は入っていない。**入れる（＝ Input に runner.Result を渡す）
// 方は採らない。** 診断の入力を検出結果の形に縛ると、doctor 側の項目を足す
// たびに検出側の戻り値を広げることになる。ここで systemd を引き直す。
type orphanCheck struct{}

func (orphanCheck) ID() string       { return "consistency.orphan" }
func (orphanCheck) Category() string { return check.CatConsistency }
func (orphanCheck) Startup() bool    { return false }

// Run は孤児ユニットごとに 1 行を返す。
func (c orphanCheck) Run(ctx context.Context, in check.Input) []check.Result {
	if !in.Caps.Systemd {
		return one(check.Skipped(c, "孤児ユニット", "systemctl がありません。"))
	}

	units, skip := scanUnits(ctx, c, in, "孤児ユニット")
	if skip != nil {
		return one(*skip)
	}

	dirs := runnerDirs(in.Runners)
	out := make([]check.Result, 0, 1)
	for _, u := range units {
		if !isOrphan(u, dirs) {
			continue
		}
		out = append(out, check.Of(c, check.Result{
			Target:  u.Unit,
			Status:  check.Warn,
			Summary: "対応する runner の無いユニット",
			Detail: u.Unit + " の WorkingDirectory（" + orNone(u.WorkingDir) +
				"）に対応する runner がありません。状態: " + orNone(u.Active) + "。",
			Impact: "svc.sh uninstall の残骸が残っており、runner の一覧と systemd の実態が食い違います。",
			Remedy: "systemctl disable --now " + u.Unit + "\n" +
				"# 確認のうえ unit ファイルを削除してください",
		}))
	}

	if len(out) == 0 {
		return one(check.Of(c, check.Result{
			Status:  check.OK,
			Summary: "孤児ユニットなし",
			Detail:  "すべての actions.runner.* ユニットが検出済みの runner に対応しています。",
		}))
	}
	return out
}

// scanUnits は判定に使う systemd のユニット一覧を引く。一覧が得られない場合は
// 第 2 戻り値に SKIP の結果を返す（呼び出し側はそれをそのまま返すこと）。
//
// **「一覧が 0 件」と「一覧が取れていない」を混同しない。** systemctl が在っても
// list-units は失敗しうる（コンテナ内・dbus 停止）。取れなかったものを 0 件として
// 扱うと、検査していないのに「問題なし」の緑が全 runner ぶん並ぶ。
//
// Exec の有無を別に見るのは、systemd.Scan が ex == nil のとき警告を出さずに
// nil を返すためである（internal/runner/systemd.Scan の doc）。Caps.Systemd は
// LookPath 由来で Exec とは独立に真になりうるので、警告の有無だけでは
// Exec 未配布を取りこぼす。
func scanUnits(ctx context.Context, c check.Check, in check.Input, summary string) ([]systemd.State, *check.Result) {
	if in.Exec == nil {
		r := check.Skipped(c, summary,
			"外部コマンドの実行経路が配られていないため systemctl を発行していません。")
		return nil, &r
	}

	units, warns := systemd.Scan(ctx, in.Exec)
	if len(units) == 0 && len(warns) > 0 {
		r := check.Skipped(c, summary, "systemd のユニット一覧を取得できませんでした。")
		return nil, &r
	}
	return units, nil
}

// isOrphan はユニットが孤児かを返す。
//
// 2 つを除外する。**LoadState=not-found** は FR-05 の孤児の定義から外れる
// （ユニットファイルが既に無い）。**Load が空**のものは systemctl show に
// 失敗した「状態不明」であり、screens.md 1.6 が孤児区画に出さないと定めて
// いる。どちらも孤児として出すと誤報になる。
func isOrphan(u systemd.State, dirs map[string]bool) bool {
	if u.Load == "" || u.Load == notFound {
		return false
	}
	if u.WorkingDir == "" {
		// 対応先を判断する材料が無い。状態不明と同じ理由で出さない。
		return false
	}
	return !dirs[u.WorkingDir]
}

// duplicateCheck は .service に記録されたユニット名と実際に紐付いたユニットの
// 食い違い、および同一 runner に複数のユニットが対応する状態を判定する。
//
// 突き合わせる 2 つは **`<runner ディレクトリ>/.service` の記録値**（svc.sh が
// install 時に書く）と **検出が実際に紐付けたユニット**である。.runner は
// 見ていない（記録しているのは runner の登録情報でユニット名ではない）。
//
// **後者は internal/runner が意図的に捨てている情報である。** discover.go の
// attachUnits は 2 パス目で「WorkingDirectory は一致するが既に別のユニットが
// 紐付いている」ユニットを黙って捨てる。FR-05 の孤児の定義（対応する runner
// ディレクトリが見つからないもの）に当てはまらず、孤児として報告すると誤報に
// なるためであり、attach_test.go の TestAttachDuplicateUnitIsNotOrphan が
// その振る舞いを固定している。捨てられた事実を診断として拾い直すのがここの
// 役目である。
type duplicateCheck struct{}

func (duplicateCheck) ID() string       { return "consistency.units" }
func (duplicateCheck) Category() string { return check.CatConsistency }
func (duplicateCheck) Startup() bool    { return false }

// Run は runner ごとに 1 行を返す。
func (c duplicateCheck) Run(ctx context.Context, in check.Input) []check.Result {
	if !in.Caps.Systemd {
		return one(check.Skipped(c, "ユニット名の整合", "systemctl がありません。"))
	}
	if len(in.Runners) == 0 {
		return one(check.Skipped(c, "ユニット名の整合", "runner が検出されていません。"))
	}

	// **1 回だけ引く。** 孤児の項目（orphanCheck）も別に引くので、診断 1 回で
	// list-units は 2 本発行される。項目どうしが結果を共有しない代わりに、
	// 項目を足しても既存の項目に触れずに済む（レジストリが加算的であることの
	// 対価であり、読み取りのみなので副作用は無い）。
	units, skip := scanUnits(ctx, c, in, "ユニット名の整合")
	if skip != nil {
		return one(*skip)
	}

	out := make([]check.Result, 0, len(in.Runners))
	for _, r := range in.Runners {
		out = append(out, c.judge(r, units))
	}
	return out
}

// judge は runner 1 台ぶんの判定を返す。
func (c duplicateCheck) judge(r runner.Runner, units []systemd.State) check.Result {
	attached := unitName(r)

	if extra := extraUnits(r, attached, units); len(extra) > 0 {
		return check.Of(c, check.Result{
			Target:  r.Name(),
			Status:  check.Warn,
			Summary: "同一 runner に複数のユニットが対応",
			Detail: r.Dir + " を WorkingDirectory に持つユニットが複数あります。" +
				"紐付いているのは " + orNone(attached) + " で、ほかに " +
				strings.Join(extra, " / ") + " があります。",
			Impact: "どちらのユニットで起動するかが分からず、片方を止めても runner が動き続けます。",
			Remedy: "不要なユニットを systemctl disable --now で止め、unit ファイルを削除してください。",
		})
	}

	// .service に記録された名前と実際に紐付いたユニットの食い違い。
	if r.UnitName != "" && attached != "" && r.UnitName != attached {
		return check.Of(c, check.Result{
			Target:  r.Name(),
			Status:  check.Warn,
			Summary: ".service とユニット名が一致しない",
			Detail: ".service に記録されたユニット名は " + r.UnitName +
				" ですが、実際に紐付いているのは " + attached + " です。",
			Impact: "svc.sh が .service の名前で操作するため、意図した runner を止められないことがあります。",
			Remedy: "svc.sh uninstall / install で登録し直すか、.service の内容を実態に合わせてください。",
		})
	}

	// 突き合わせる材料が片方でも欠けていれば判定していない。**OK にしない。**
	// .service を持たない runner（svc.sh を使わず登録したもの）は記録値が空に
	// なるが、それは「一致している」ことの根拠にならない。重複ユニットの検出は
	// .service の有無と無関係に成立するので、上の分岐は先に通してある。
	if r.UnitName == "" || attached == "" {
		return check.Of(c, check.Result{
			Target:  r.Name(),
			Status:  check.Skip,
			Summary: "ユニット名の整合",
			Detail: ".service に記録されたユニット名は " + orNone(r.UnitName) +
				"、紐付いているユニットは " + orNone(attached) +
				" で、突き合わせる材料が揃いません。",
		})
	}

	return check.Of(c, check.Result{
		Target:  r.Name(),
		Status:  check.OK,
		Summary: "ユニット名は整合している",
		Detail: ".service に記録されたユニット名（" + r.UnitName +
			"）と実際に紐付いているユニットが一致しています。",
	})
}

// extraUnits は runner のディレクトリを指すが紐付いていないユニットを返す。
func extraUnits(r runner.Runner, attached string, units []systemd.State) []string {
	var extra []string
	for _, u := range units {
		if u.WorkingDir == "" || u.WorkingDir != r.Dir {
			continue
		}
		if u.Unit == attached {
			continue
		}
		extra = append(extra, u.Unit)
	}
	return extra
}

// runnerDirs は検出済み runner のディレクトリの集合を返す。
func runnerDirs(runners []runner.Runner) map[string]bool {
	out := make(map[string]bool, len(runners))
	for _, r := range runners {
		out[r.Dir] = true
	}
	return out
}
