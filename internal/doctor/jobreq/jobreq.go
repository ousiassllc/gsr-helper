// Package jobreq は docker（daemon の稼働）と「ジョブ実行の前提」（FR-43）の
// 診断項目を持つ。
//
// ここに集めたのは **gsr-helper 自身の動作には要らないが、欠けると runner 上の
// ワークフローが失敗する**ホスト側の前提である。実運用でジョブ失敗の主因に
// なった 4 点（パスワード不要 sudo / docker / docker buildx / docker グループ
// 所属）を docs/operations/runner-host-setup.md の手順と 1 対 1 で対応させて
// あり、同書の「欠けているものと症状」に挙がった文言をそのまま Impact に出す。
// 利用者が画面の文言でホストのエラーログを検索できるようにするためである。
//
// **検出と手順の提示までしか行わない。** sudoers の書き換え・パッケージの導入・
// usermod は実行しない（docs/architecture/security.md の「実装しないこと」）。
// とりわけ sudoers は壊すと sudo 自体で復旧できなくなるため、対処は Remedy に
// 文字列として出すだけにしてある。
//
// このパッケージの項目は docker.daemon を除いて起動時に自動実行される
// （FR-44）。そのため 1 項目あたりの外部コマンドはホスト内で完結する軽いもの
// （`sudo -l -U` / `docker buildx version`）に限り、docker グループ所属の判定は
// コマンドを使わず /etc/group と /proc の読み取りだけで行う。
package jobreq

import (
	"errors"
	"strconv"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// 各項目が満たす契約をコンパイル時に確かめる。型を外へ出さないため、
// 実装漏れはここでしか検出できない。
var (
	_ check.Check = dockerDaemonCheck{}
	_ check.Check = sudoCheck{}
	_ check.Check = dockerCheck{}
	_ check.Check = buildxCheck{}
	_ check.Check = dockerGroupCheck{}
)

// Checks はこのパッケージが提供する診断項目を表示順に返す。
//
// 型を公開せずこの関数だけを出すのは、レジストリ（internal/doctor）が並べる
// 集合をここ 1 箇所で決めるためである。項目を足しても呼び出し側は変わらない。
//
// 並びは docs/requirements/functional.md のチェック項目一覧表に合わせて
// docker → ジョブ実行の前提（sudo / docker / buildx / グループ）の順にする。
// 一覧の整列は internal/doctor.Run が分類と識別子で行うため表示順はそちらで
// 決まるが、起動時の自動判定（FR-44）が拾う順はこの並びがそのまま出る。
func Checks() []check.Check {
	return []check.Check{
		dockerDaemonCheck{},
		sudoCheck{},
		dockerCheck{},
		buildxCheck{},
		dockerGroupCheck{},
	}
}

// impactDockerSocket は docker グループ所属が欠けたときにジョブへ出る症状。
// docs/operations/runner-host-setup.md の「欠けているものと症状」の表から採る。
const impactDockerSocket = "docker を使うジョブが " +
	"`permission denied while trying to connect to the Docker daemon socket` で失敗します。"

// targets は判定対象になる runner（RunAsUser が分かっているもの）だけを返す。
//
// **RunAsUser が空の runner は行そのものを作らない。** 空はユニットの User= も
// Listener の所有者も取れなかったことを表す（internal/runner.resolveRunAsUser）
// ので、判定できない理由は「ホストの不備」ではなく「対象が特定できていない」で
// ある。SKIP の行を出すと、runner が停止しているだけの平常時に灰色の行が台数分
// 並び、Doctor タブの一覧が読めなくなる。
func targets(runners []runner.Runner) []runner.Runner {
	out := make([]runner.Runner, 0, len(runners))
	for _, r := range runners {
		if r.RunAsUser != "" {
			out = append(out, r)
		}
	}
	return out
}

// numericUser は s が UID の 10 進表記かを返す。
//
// RunAsUser は名前が解決できない環境（静的リンクで NSS が使えない、LDAP 上の
// ユーザー）では UID の 10 進表記になる（data-model.md）。strconv を使わないのは、
// int に収まらない桁数でも「数値である」ことは変わらないためである。数値かどうかで
// 渡し方を変えるだけなので、値そのものは要らない。
func numericUser(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// firstLine は b の 1 行目を前後の空白を落として返す。
//
// `docker info --format` や `docker buildx version` の出力を Detail に載せる
// ために使う。複数行を通すと一覧の 1 行に収まらないので先頭だけを採る。
func firstLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// probeFailure は失敗したコマンド実行の根拠を 1 行にまとめる。
//
// 標準エラー出力を載せるのは、daemon 不応答（`Cannot connect to the Docker
// daemon`）のような原因がそこにしか出ないためである。**標準出力は載せない。**
// `sudo -l -U` の出力のように権限情報を含む経路と同じ関数を通すので、
// 出す側を最初から持たない形にしてある。
func probeFailure(res exec.Result, err error) string {
	if err != nil {
		return "コマンドを実行できませんでした: " + err.Error()
	}
	msg := "終了コード " + strconv.Itoa(res.ExitCode)
	if s := firstLine(res.Stderr); s != "" {
		msg += "、標準エラー出力: " + s
	}
	return msg
}

// noExecutor は Executor が配られていないことによる失敗かを返す。
//
// ホストの不備ではなく組み立ての都合なので、FAIL / WARN ではなく SKIP に倒す
// （check.ErrNoExecutor の doc）。ここを FAIL にすると、Executor を渡し忘れた
// 起動が「ホストが壊れている」という表示になり原因が追えない。
func noExecutor(err error) bool { return errors.Is(err, check.ErrNoExecutor) }
