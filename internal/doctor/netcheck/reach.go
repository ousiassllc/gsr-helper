package netcheck

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
)

// reachTimeout は宛先 1 つあたりの待ち時間。
//
// 3 秒は check.ProbeTimeout（外部コマンド 1 本ぶん）に揃えた値である。TCP の
// 接続だけなので健全な回線なら 1 秒もかからず、これ以上待っても結果は変わら
// ない。逆にこれより短いと、DNS の応答が遅い環境やプロキシ配下の遅い経路で
// 正常な宛先まで到達不能に見えてしまう。
const reachTimeout = 3 * time.Second

// endpoint は到達性を確かめる 1 つの宛先。
type endpoint struct {
	// addr は接続先（host:port）。
	addr string
	// role は「何のための通信か」。画面の 1 行に添えて、遮断された宛先から
	// 何が壊れるかを利用者が読み取れるようにする。
	role string
	// impact は到達できないときに起きること（Result.Impact）。
	impact string
}

// reachEndpoints は runner の動作に要る宛先。functional.md のチェック項目
// 一覧表（`github.com:443`、`api.github.com`、`*.actions.githubusercontent.com`、
// `pkg-containers`、results-receiver への到達性）に対応する。
//
// ワイルドカードの `*.actions.githubusercontent.com` は接続できないので、
// runner が実際に使うホスト名を代表として置いている。
//
// **並び順は画面の並び順そのものである。** 到達確認は並行に走らせるが、結果は
// この順に戻す。完了の早い順に並べると、再実行のたびに行が入れ替わって
// カーソルの位置が意味を失う。
var reachEndpoints = []endpoint{
	{
		// runner の登録とリポジトリの取得
		addr:   "github.com:443",
		role:   "runner の登録とリポジトリの取得",
		impact: "runner を登録できず、登録済みの runner もオフラインになる",
	},
	{
		// REST API（runner の状態やジョブの一覧の取得）
		addr:   "api.github.com:443",
		role:   "REST API",
		impact: "runner の状態を取得できず、runner がオフラインになる",
	},
	{
		// ジョブの受信。`*.actions.githubusercontent.com` の代表
		addr:   "pipelines.actions.githubusercontent.com:443",
		role:   "ジョブの受信",
		impact: "ジョブを受け取れない",
	},
	{
		// ジョブ結果の送信
		addr:   "results-receiver.actions.githubusercontent.com:443",
		role:   "ジョブ結果の送信",
		impact: "ジョブの結果を送れない",
	},
	{
		// パッケージ / コンテナの取得
		addr:   "pkg-containers.githubusercontent.com:443",
		role:   "パッケージ / コンテナの取得",
		impact: "パッケージやコンテナを取得できず、それを使うジョブが失敗する",
	},
}

// reachRemedy は到達できないときの対処。**表示するだけで実行はしない**
// （security.md）。
const reachRemedy = "ファイアウォールとプロキシの設定を確認する（この画面は表示するだけで設定は変更しない）"

// proxyBypassNote はプロキシ配下のときに各行へ添える断り書き。
//
// ここでの確認は in.DialAddr による素の TCP 接続で、プロキシを経由しない。
// 正しくプロキシを設定したホストでは直接の外向き通信が塞がれているのが普通
// なので、断りなしに並べると設定が正しいことの証跡が不備に見える。
const proxyBypassNote = "この確認はプロキシを経由せず TCP で直接つないでいる。プロキシ経由の環境では直接の到達が塞がれているのが正常なこともある。"

// reachCheck は GitHub 側の各エンドポイントへ TCP で到達できるかを確かめる。
type reachCheck struct{}

// ID はチェックの識別子を返す。
func (reachCheck) ID() string { return "net.reach" }

// Category は分類を返す。
func (reachCheck) Category() string { return check.CatNetwork }

// Startup は偽を返す。
//
// FR-44 の起動時の自動判定はネットワーク到達性を含めない。外向きの接続を
// 起動経路に挟むと、回線の状態次第で起動が待たされるためである。
func (reachCheck) Startup() bool { return false }

// Run は宛先ごとに 1 行を返す。
//
// 宛先を 1 行へ畳まないのは、1 つ落ちているだけで他の宛先の判定が隠れると、
// どこが遮断されているのかが画面から読めなくなるためである。ホスト全体の
// 確認なので Target は空にし、どの宛先かは Summary に書く。
func (c reachCheck) Run(ctx context.Context, in check.Input) []check.Result {
	// プロキシ配下かどうかで判定の重みを変えるため、先に一度だけ読む。
	proxied := proxyConfigured(in)

	// 宛先ぶん直列に待つと最悪 宛先数 × reachTimeout かかる。互いに独立な
	// 接続なので並行に張り、添字で書き戻して宣言順を保つ。
	results := make([]check.Result, len(reachEndpoints))
	var wg sync.WaitGroup
	for i, ep := range reachEndpoints {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = c.dialOne(ctx, in, ep, proxied)
		}()
	}
	wg.Wait()

	return results
}

// dialOne は 1 つの宛先へ接続し、その結果を 1 行にする。
func (c reachCheck) dialOne(ctx context.Context, in check.Input, ep endpoint, proxied bool) check.Result {
	// 全体の期限は呼び出し側の ctx が持つ。ここで宛先ごとの上限も切って、
	// 応答の返らない 1 宛先が診断全体を止めないようにする。
	ctx, cancel := context.WithTimeout(ctx, reachTimeout)
	defer cancel()

	err := in.DialAddr(ctx, ep.addr)
	if err == nil {
		return check.Of(c, check.Result{
			ID:       "",
			Category: "",
			Target:   "",
			Status:   check.OK,
			Summary:  fmt.Sprintf("%s へ到達できます（%s）", ep.addr, ep.role),
			Detail:   detailWithNote(fmt.Sprintf("%s へ TCP で接続できた。", ep.addr), proxied),
			Impact:   "",
			Remedy:   "",
			Startup:  false,
		})
	}

	// プロキシ配下では直接の到達が塞がれているのが正常なこともある。ここを
	// 一律 FAIL にすると、正しく設定されたプロキシ環境では全宛先が赤くなり、
	// 本当の不備が埋もれて Doctor タブそのものが読めなくなる。
	status := check.Fail
	if proxied {
		status = check.Warn
	}

	return check.Of(c, check.Result{
		ID:       "",
		Category: "",
		Target:   "",
		Status:   status,
		Summary:  fmt.Sprintf("%s へ到達できません（%s）", ep.addr, ep.role),
		Detail:   detailWithNote(fmt.Sprintf("%s への TCP 接続に失敗した: %v", ep.addr, err), proxied),
		Impact:   ep.impact,
		Remedy:   reachRemedy,
		Startup:  false,
	})
}

// detailWithNote は本文にプロキシの断り書きを添える。
func detailWithNote(body string, proxied bool) string {
	if !proxied {
		return body
	}
	return strings.Join([]string{body, proxyBypassNote}, "\n")
}
