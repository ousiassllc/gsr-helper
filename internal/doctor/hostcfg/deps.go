package hostcfg

import (
	"context"
	"errors"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
)

// dependency は存在とバージョンを確かめるコマンド 1 つ。
type dependency struct {
	name string
	// missing は無かったときの判定。必須かどうかがコマンドごとに違う。
	missing check.Status
	// why は何に要るか。Detail と Impact に出す。
	why string
	// remedy は導入の手順（表示のみ）。
	remedy string
}

// dependencies は確かめるコマンド。
//
// **gcc は含めない。** runner-host-setup.md の「doctor での検出」は FR-43 の
// 4 点だけを検査対象と定めており、C コンパイラを明示的に外している。ここへ
// 足すと、同書が「検査しない」と書いたものを検査することになる。
var dependencies = []dependency{
	{
		name: "git", missing: check.Fail,
		why:    "リポジトリのチェックアウトに使います",
		remedy: "sudo apt-get install -y git",
	},
	{
		name: "docker", missing: check.Warn,
		why: "コンテナを使うジョブに要ります（docker を使わないワークフローだけなら不要です）。" +
			"daemon の稼働とグループ所属は「ジョブ実行の前提」の項目で別に判定します",
		remedy: "sudo apt-get install -y docker.io docker-buildx",
	},
	{
		name: "node", missing: check.Warn,
		why: "JavaScript のアクションに使います（actions/* の多くはリポジトリが同梱する " +
			"Node を使うため必須ではありません）",
		remedy: "sudo apt-get install -y nodejs",
	},
}

// depsCheck は依存コマンドの存在とバージョンを判定する。
type depsCheck struct{}

func (depsCheck) ID() string       { return "deps.commands" }
func (depsCheck) Category() string { return check.CatDeps }
func (depsCheck) Startup() bool    { return false }

// Run はコマンドごとに 1 行を返す。
//
// 1 行へ畳まないのは、どれが欠けているかで対処が変わるためである。
func (c depsCheck) Run(ctx context.Context, in check.Input) []check.Result {
	out := make([]check.Result, 0, len(dependencies))
	for _, d := range dependencies {
		out = append(out, c.judge(ctx, in, d))
	}
	return out
}

// judge はコマンド 1 つぶんの判定を返す。
func (c depsCheck) judge(ctx context.Context, in check.Input, d dependency) check.Result {
	path, err := in.Look(d.name)
	if err != nil {
		return check.Of(c, check.Result{
			Status:  d.missing,
			Summary: d.name + " が無い",
			Detail:  d.name + " が PATH 上にありません。",
			Impact:  d.name + " は" + d.why + "。",
			Remedy:  d.remedy,
		})
	}

	res, perr := in.Probe(ctx, "doctor.deps", d.name, "--version")
	if noExecutor(perr) {
		return check.Skipped(c, d.name+" のバージョンを判定していない",
			d.name+" は "+path+" にありますが、外部コマンドの実行経路が配られていないため "+
				d.name+" --version を発行していません。")
	}
	if perr != nil || res.ExitCode != 0 {
		return check.Of(c, check.Result{
			Status:  check.Warn,
			Summary: d.name + " のバージョンを取得できない",
			Detail:  d.name + " は " + path + " にありますが --version が失敗しました。",
			Impact:  "導入されているが正常に起動しない可能性があります。",
			Remedy:  d.name + " --version を手で実行し、失敗の理由を確認してください。",
		})
	}

	return check.Of(c, check.Result{
		Status:  check.OK,
		Summary: d.name + " がある",
		Detail:  firstLine(res.Stdout) + "（" + path + "）",
	})
}

// noExecutor は Executor が配られていないことによる失敗かを返す。
//
// 能力不足で実行できなかっただけであり、ホストの不備ではない。WARN に倒すと
// 「コマンドは在るが壊れている」と読めてしまうので SKIP と区別する
// （check.ErrNoExecutor の doc。internal/doctor/jobreq も同じ判定を使う）。
func noExecutor(err error) bool { return errors.Is(err, check.ErrNoExecutor) }

// firstLine は出力の 1 行目を前後の空白を落として返す。
//
// --version は複数行を出すことがある（docker は Client / Server を並べる）。
// 一覧の 1 行に収まらないので先頭だけを採る。
func firstLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
