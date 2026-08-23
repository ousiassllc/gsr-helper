package jobreq

import (
	"context"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
)

// serverVersionFormat は docker info から daemon の版だけを取り出す指定。
//
// 版が取れたことを稼働の条件にするのは、`docker info` が daemon 不応答でも版に
// よっては終了コード 0 を返すことがあるためである（hostcaps.dockerAlive と同じ
// 判断）。
const serverVersionFormat = "{{.ServerVersion}}"

// dockerInstallRemedy は docker と buildx をまとめて入れる手順。
//
// **buildx を同時に入れる。** docker.io 単体では buildx が入らず、直後に
// job.buildx が WARN になる（docs/operations/runner-host-setup.md の手順 2-3）。
const dockerInstallRemedy = "sudo apt-get install -y docker.io docker-buildx"

// dockerDaemonCheck は docker daemon が応答するかを見る（分類「docker」）。
//
// 起動時の自動判定（FR-44）の対象にしない。FR-44 が対象とするのは FR-43 の
// 4 点であり、daemon の稼働はそこに含まれない。`docker info` は daemon が
// 応答しないとき待たされることがあり、起動時間の予算（non-functional.md）に
// 載せたくないという理由もある。
type dockerDaemonCheck struct{}

func (dockerDaemonCheck) ID() string       { return "docker.daemon" }
func (dockerDaemonCheck) Category() string { return check.CatDocker }
func (dockerDaemonCheck) Startup() bool    { return false }

// Run は daemon の稼働を判定する。対象はホスト全体なので Target は空。
func (c dockerDaemonCheck) Run(ctx context.Context, in check.Input) []check.Result {
	// docker が無いことは daemon の異常ではない。docker を使わない運用でも
	// 赤が残り続けないよう SKIP に倒す（FR-32 の SKIP と FAIL の区別）。
	if !in.Has("docker") {
		return one(check.Skipped(c,
			"docker が無いため未判定",
			"docker コマンドが PATH 上にありません。daemon の稼働は判定していません。"))
	}

	res, err := in.Probe(ctx, "doctor.docker", "docker", "info", "--format", serverVersionFormat)
	if noExecutor(err) {
		return one(check.Skipped(c,
			"コマンドを実行できないため未判定",
			"外部コマンドの実行経路が配られていないため `docker info` を発行していません。"))
	}
	if version := firstLine(res.Stdout); err == nil && res.ExitCode == 0 && version != "" {
		return one(check.Of(c, check.Result{
			Status:  check.OK,
			Summary: "docker daemon が応答している",
			Detail:  "`docker info` が daemon の版 " + version + " を返しました。",
		}))
	}
	return one(check.Of(c, check.Result{
		Status:  check.Fail,
		Summary: "docker daemon が応答しない",
		Detail:  "`docker info` から daemon の版を取得できませんでした（" + probeFailure(res, err) + "）。",
		Impact:  "docker を使うジョブが失敗します。",
		Remedy:  "systemctl start docker",
	}))
}

// dockerCheck は docker コマンドの存在を見る（FR-43）。
//
// daemon の稼働（docker.daemon）と分けてあるのは、対処が別物だからである。
// 不在はパッケージの導入、不応答はサービスの起動で、前者は後者を含む。
type dockerCheck struct{}

func (dockerCheck) ID() string       { return "job.docker" }
func (dockerCheck) Category() string { return check.CatJobReq }
func (dockerCheck) Startup() bool    { return true }

// Run は docker の存在を判定する。対象はホスト全体なので Target は空。
func (c dockerCheck) Run(_ context.Context, in check.Input) []check.Result {
	// 存在確認だけならコマンドを発行しない。監査ログに無駄なレコードを残さず、
	// 起動時（FR-44）の所要時間にも乗らない。
	path, err := in.Look("docker")
	if err != nil {
		return one(check.Of(c, check.Result{
			Status:  check.Fail,
			Summary: "docker が無い",
			Detail:  "docker コマンドが PATH 上にありません。",
			Impact:  "docker を使うジョブが `docker: command not found` で失敗します。",
			Remedy:  dockerInstallRemedy,
		}))
	}
	return one(check.Of(c, check.Result{
		Status:  check.OK,
		Summary: "docker がある",
		Detail:  "docker コマンドは " + path + " にあります。",
	}))
}

// buildxCheck は docker buildx の存在を見る（FR-43）。
//
// docker 本体と分けてあるのは、**docker.io パッケージに buildx が含まれない**
// ためである（docs/operations/runner-host-setup.md の手順 2-3）。docker がある
// のに buildx だけ無い状態が実際に起きるので、1 項目にまとめると対処が
// 「docker を入れる」に化けて何も直らない。
type buildxCheck struct{}

func (buildxCheck) ID() string       { return "job.buildx" }
func (buildxCheck) Category() string { return check.CatJobReq }
func (buildxCheck) Startup() bool    { return true }

// Run は buildx の存在を判定する。対象はホスト全体なので Target は空。
//
// 欠落を FAIL にせず WARN にするのは、buildx を要るかどうかが Dockerfile の
// 内容（`COPY --chmod` を使うか）で決まり、ツールが一律に不備と断定できない
// ためである。判定の強さは docs/operations/runner-host-setup.md の
// 「doctor での検出」の表に合わせてある。
func (c buildxCheck) Run(ctx context.Context, in check.Input) []check.Result {
	if !in.Has("docker") {
		return one(check.Skipped(c,
			"docker が無いため未判定",
			"docker コマンドが PATH 上にありません。buildx は docker のサブコマンドなので判定していません。"))
	}

	res, err := in.Probe(ctx, "doctor.buildx", "docker", "buildx", "version")
	if noExecutor(err) {
		return one(check.Skipped(c,
			"コマンドを実行できないため未判定",
			"外部コマンドの実行経路が配られていないため `docker buildx version` を発行していません。"))
	}
	if err == nil && res.ExitCode == 0 {
		return one(check.Of(c, check.Result{
			Status:  check.OK,
			Summary: "docker buildx がある",
			Detail:  "`docker buildx version` の出力: " + buildxVersion(res.Stdout),
		}))
	}
	return one(check.Of(c, check.Result{
		Status:  check.Warn,
		Summary: "docker buildx が無い",
		Detail:  "`docker buildx version` が失敗しました（" + probeFailure(res, err) + "）。",
		Impact: "`COPY --chmod` を含む Dockerfile のビルドが " +
			"`the --chmod option requires BuildKit` で失敗します。",
		Remedy: "sudo apt-get install -y docker-buildx",
	}))
}

// buildxVersion は `docker buildx version` の出力を Detail に出す形にする。
//
// 終了コード 0 でも出力が空になる版があり得るため、空文字をそのまま繋いで
// 「出力: 」で終わる文を作らないようにする。
func buildxVersion(stdout []byte) string {
	if v := firstLine(stdout); v != "" {
		return v
	}
	return "（空。終了コードのみで判定しました）"
}

// one は結果 1 件を戻り値の形に包む。
//
// Run は必ず slice を返すため、1 件しか返さない項目でも同じ包みが要る。
func one(r check.Result) []check.Result { return []check.Result{r} }
