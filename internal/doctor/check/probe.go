package check

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/gh"
)

// ErrNoExecutor は Executor が配られていないことを表す。
//
// 診断項目は失敗ではなく SKIP を返す判断にこれを使う。Executor が無いのは
// ホストの不備ではなく組み立ての都合だからである。
var ErrNoExecutor = errors.New("外部コマンドを実行できません")

// Clock は現在時刻を返す。Now が nil なら time.Now を使う。
func (in Input) Clock() time.Time {
	if in.Now == nil {
		return time.Now()
	}
	return in.Now()
}

// Env は環境変数を返す。Getenv が nil なら os.Getenv を使う。
func (in Input) Env(name string) string {
	if in.Getenv == nil {
		return os.Getenv(name)
	}
	return in.Getenv(name)
}

// Look はコマンドの絶対パスを返す。LookPath が nil なら exec.LookPath を使う。
//
// 存在確認は監査ログに残らない（exec.LookPath の doc）。
func (in Input) Look(name string) (string, error) {
	if in.LookPath == nil {
		path, err := exec.LookPath(name)
		if err != nil {
			return "", err
		}
		return path, nil
	}
	return in.LookPath(name)
}

// Has はコマンドが存在するかを返す。
func (in Input) Has(name string) bool {
	_, err := in.Look(name)
	return err == nil
}

// Path は FSRoot を起点にしたパスを返す。FSRoot が空なら p をそのまま返す。
//
// /proc や /etc を読む項目をテストから実ファイルシステムに触らせないための
// 差し替え口である。読むだけの項目にモックの層を挟むより、起点を差し替える
// 方が実装と検査の距離が近い。
func (in Input) Path(p string) string {
	if in.FSRoot == "" {
		return p
	}
	return filepath.Join(in.FSRoot, p)
}

// ReadFile は FSRoot を起点にファイルを読む。
func (in Input) ReadFile(p string) ([]byte, error) {
	body, err := os.ReadFile(in.Path(p))
	if err != nil {
		return nil, err
	}
	return body, nil
}

// Probe は 1 本のコマンドを実行する。
//
// **SkipAudit は立てない。** doctor が叩くコマンドは診断 1 回につき 1 本で
// あり、記録を押し流さない（security.md の記録対象外とする読み取りコマンド）。
// 出力そのものは監査ログに残らない（audit.Record が出力の欄を持たない）ので、
// `sudo -l -U` のような権限情報を含む出力もここを通して安全に扱える。
func (in Input) Probe(ctx context.Context, action, name string, args ...string) (exec.Result, error) {
	if in.Exec == nil {
		return exec.Result{Stdout: nil, Stderr: nil, ExitCode: -1}, ErrNoExecutor
	}
	ctx, cancel := context.WithTimeout(ctx, ProbeTimeout)
	defer cancel()
	ctx = exec.WithOptions(ctx, exec.Options{
		Action:    action,
		Runner:    "",
		Dir:       "",
		Env:       nil,
		SkipAudit: false,
	})
	res, err := in.Exec.Run(ctx, name, args...)
	if err != nil {
		return res, err
	}
	return res, nil
}

// DialAddr は addr へ TCP で到達できるかを確かめる。Dial が nil なら
// net.Dialer を使う。呼び出し側で期限を切ること。
func (in Input) DialAddr(ctx context.Context, addr string) error {
	dial := in.Dial
	if dial == nil {
		var d net.Dialer
		dial = d.DialContext
	}
	conn, err := dial(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	return conn.Close()
}

// Client は GitHub API クライアントを返す。NewClient が nil なら
// トークンを取得して組み立てる。
func (in Input) Client(ctx context.Context) (*gh.Client, error) {
	if in.NewClient != nil {
		return in.NewClient(ctx)
	}
	token, err := gh.Token(ctx, in.Exec)
	if err != nil {
		return nil, err
	}
	c, err := gh.New(token)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// Of は c の識別子・分類・起動時対象を r へ写して返す。
//
// 3 つを各項目が手で埋めると、ID() の戻りと Result.ID が食い違う形を作れて
// しまう（一覧の再実行が別の項目を指す）。写しをここに 1 つ置いて防ぐ。
func Of(c Check, r Result) Result {
	r.ID = c.ID()
	r.Category = c.Category()
	r.Startup = c.Startup()
	return r
}

// Skipped は能力不足で実行できなかったことを表す Result を返す。
//
// detail に「何が無いから実行できなかったか」を書く。SKIP は失敗ではないので
// Impact と Remedy は持たない（対処すべき不備が見つかっていない）。
func Skipped(c Check, summary, detail string) Result {
	return Of(c, Result{
		ID:       "",
		Category: "",
		Target:   "",
		Status:   Skip,
		Summary:  summary,
		Detail:   detail,
		Impact:   "",
		Remedy:   "",
		Startup:  false,
	})
}
