package authz

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// 判定するファイル。
const (
	credentialsFile = ".credentials"
	runnerFile      = ".runner"
)

// unknownUID は所有者を取得できなかったことを表す。0 は root という有効値なので
// 兼用しない（procs.Process.UID と同じ約束）。
const unknownUID = -1

// perm は runner の資格情報ファイルのパーミッションと所有者を判定する。
//
// **中身は読まない。** .credentials は runner の認証鍵そのものであり、
// data-model.md の「読み書きの対象一覧」が「読まない／パーミッションと所有者の
// み確認する」と定めている。読めば本ツールのメモリとコアダンプに鍵が載る。
type perm struct{}

func (perm) ID() string       { return "authz.perm" }
func (perm) Category() string { return check.CatAuthz }
func (perm) Startup() bool    { return false }

// Run は runner ごとに 1 行を返す。
func (c perm) Run(_ context.Context, in check.Input) []check.Result {
	if len(in.Runners) == 0 {
		return []check.Result{check.Skipped(c, "資格情報のパーミッション", "runner が検出されていません。")}
	}

	out := make([]check.Result, 0, len(in.Runners))
	for _, r := range in.Runners {
		out = append(out, c.judge(in, r))
	}
	return out
}

// finding は 1 つの不備。
type finding struct {
	status check.Status
	detail string
	remedy string
}

// judge は runner 1 台ぶんの判定を返す。
//
// 2 つのファイルの不備を 1 行へまとめるのは、同じ runner の同じ種類の問題を
// 別々の行にすると台数 × 2 行になり一覧が読めなくなるためである。
func (c perm) judge(in check.Input, r runner.Runner) check.Result {
	var found []finding
	var missing int

	for _, f := range []struct {
		name string
		fn   func(mode fs.FileMode, owner, listener int, path string) *finding
	}{
		{name: credentialsFile, fn: credentialsFinding},
		{name: runnerFile, fn: runnerFinding},
	} {
		path := in.Path(filepath.Join(r.Dir, f.name))
		info, err := os.Stat(path)
		if errors.Is(err, fs.ErrNotExist) {
			missing++
			continue
		}
		if err != nil {
			found = append(found, finding{
				status: check.Warn,
				detail: f.name + " の状態を取得できませんでした: " + err.Error(),
				remedy: "",
			})
			continue
		}
		if fd := f.fn(info.Mode().Perm(), ownerUID(info), listenerUID(r), path); fd != nil {
			found = append(found, *fd)
		}
	}

	// 2 つとも無いのは未設定の runner であり、ホストの不備ではない。
	if missing == 2 {
		return check.Of(c, check.Result{
			Target:  r.Name(),
			Status:  check.Skip,
			Summary: "資格情報のパーミッション",
			Detail:  credentialsFile + " も " + runnerFile + " もありません（未設定の runner です）。",
		})
	}

	if len(found) == 0 {
		return check.Of(c, check.Result{
			Target:  r.Name(),
			Status:  check.OK,
			Summary: "資格情報のパーミッションは適切",
			Detail:  credentialsFile + " は所有者だけが読める状態です。",
		})
	}
	return c.report(r, found)
}

// report は見つかった不備をもっとも重い判定でまとめる。
func (c perm) report(r runner.Runner, found []finding) check.Result {
	worst := check.OK
	details := make([]string, 0, len(found))
	remedies := make([]string, 0, len(found))
	for _, f := range found {
		if f.status > worst {
			worst = f.status
		}
		details = append(details, f.detail)
		if f.remedy != "" {
			remedies = append(remedies, f.remedy)
		}
	}

	summary := "資格情報のパーミッションが緩い"
	if worst == check.Warn {
		summary = "資格情報のパーミッションに注意"
	}
	return check.Of(c, check.Result{
		Target:  r.Name(),
		Status:  worst,
		Summary: summary,
		Detail:  strings.Join(details, " "),
		Impact: "同じホストの他のユーザーが runner の資格情報を読み取り、" +
			"この runner になりすまして GitHub からジョブを受け取れます。",
		Remedy: strings.Join(remedies, "\n"),
	})
}

// credentialsFinding は .credentials の不備を返す。無ければ nil。
//
// group / other にビットが 1 つでも立っていれば FAIL にする。認証鍵は
// 所有者だけが読める状態（0600）でなければならない。
func credentialsFinding(mode fs.FileMode, owner, listener int, path string) *finding {
	if mode&0o077 != 0 {
		return &finding{
			status: check.Fail,
			detail: credentialsFile + " が " + octal(mode) + " です（所有者以外に権限があります）。",
			remedy: "chmod 600 " + path,
		}
	}
	// 所有者が runner の実行ユーザーと違うと、runner 自身が読めないか、
	// 別のユーザーが読める状態になっている。どちらも 0600 では防げない。
	if owner != unknownUID && listener != unknownUID && owner != listener {
		return &finding{
			status: check.Warn,
			detail: credentialsFile + " の所有者（UID " + strconv.Itoa(owner) +
				"）が稼働中の Runner.Listener の実行ユーザー（UID " + strconv.Itoa(listener) + "）と違います。",
			remedy: "chown " + strconv.Itoa(listener) + " " + path,
		}
	}
	return nil
}

// listenerUID は稼働中の Runner.Listener の実行ユーザーを返す。
// 稼働していなければ unknownUID。
func listenerUID(r runner.Runner) int {
	if r.Listener == nil {
		return unknownUID
	}
	return r.Listener.UID
}

// runnerFinding は .runner の不備を返す。無ければ nil。
//
// .runner は資格情報ではなく設定なので、読み取りは許す。他人が書き換えられる
// 状態（world-writable）だけを咎める。
func runnerFinding(mode fs.FileMode, _, _ int, path string) *finding {
	if mode&0o022 != 0 {
		return &finding{
			status: check.Warn,
			detail: runnerFile + " が " + octal(mode) + " です（所有者以外が書き換えられます）。",
			remedy: "chmod 644 " + path,
		}
	}
	return nil
}

// octal はパーミッションを 0600 の形の文字列で返す。
func octal(mode fs.FileMode) string {
	return "0" + strconv.FormatUint(uint64(mode.Perm()), 8)
}

// ownerUID はファイルの所有者 UID を返す。取得できなければ unknownUID。
//
// syscall.Stat_t を経由するのは、io/fs が所有者を持たないためである。
func ownerUID(info fs.FileInfo) int {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return unknownUID
	}
	return int(st.Uid)
}
