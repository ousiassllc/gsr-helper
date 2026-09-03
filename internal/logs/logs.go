// Package logs は runner のログの一覧と追従を提供する（FR-23〜FR-26）。
//
// 扱う対象は 2 つある。runner が `_diag` 配下へ書くログファイル（`Runner_*.log` /
// `Worker_*.log`）と、systemd ユニットのログ（`journalctl`）である。前者はファイルの
// 追記を検知して送り（Tail）、後者は Executor 越しに取得して差分を送る（Journal）。
// どちらも同じ Line を同じ形のチャネルへ流すので、UI 側は 2 つの経路を 1 つの
// ビューで扱える（screens.md の Logs タブ）。
//
// **このパッケージは UI を知らない。** 強調表示に使う重大度は Level として値で返し、
// 色を決めるのは表示層である（依存の規則。docs/components/overview.md）。
//
// 型名にパッケージ名を重ねない規約に従い、ログファイル 1 件は File と呼ぶ
// （logs.LogFile とはしない）。
package logs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// DiagDir は runner ディレクトリ配下のログ置き場の名前。
//
// runner 本体が作るディレクトリであり、バージョン更新でも上書きしない対象である
// （FR-21）。
const DiagDir = "_diag"

// Kind はログファイルの種類。
//
// 直近ジョブの Worker ログを特定する（LatestWorker）ために、名前の判定を 1 箇所へ
// 寄せて値で持つ。呼び出し側が毎回ファイル名の接頭辞を書くと、runner 側の命名が
// 変わったときに直す場所が散る。
type Kind int

// Kind の取り得る値。
const (
	// KindRunner は Listener（常駐側）のログ。
	KindRunner Kind = iota
	// KindWorker はジョブ 1 件を実行した Worker のログ。
	KindWorker
)

// ログファイルの名前の規則。runner が `_diag` へ書く名前に合わせる。
const (
	prefixRunner = "Runner_"
	prefixWorker = "Worker_"
	suffixLog    = ".log"
)

// File は `_diag` 配下のログファイル 1 件。
//
// どの runner のものかは持たない。1 台ぶんを列挙する List の戻り値であり、runner と
// 組にするのは呼び出し側（複数 runner を 1 つの一覧に並べる Logs タブ）の仕事である。
// ここに runner を持たせると、同じ runner の値が行数ぶん複製される。
type File struct {
	// Path は絶対パス（runner.Runner.Dir がシンボリックリンク解決済みのため）。
	Path string
	// Name はファイル名。一覧に出す表示名でもある。
	Name string
	Kind Kind
	// Size はバイト数（FR-23 の「サイズ付き」）。
	Size int64
	// ModTime は更新時刻。一覧の並び順の基準である。
	ModTime time.Time
}

// List は runner の `_diag` 配下のログを更新時刻の新しい順に列挙する（FR-23）。
//
// `_diag` が無い runner は空を返し、エラーにしない。ログを 1 度も書いていない
// runner（追加した直後、`run.sh` を起動していない）で一覧全体を失敗させないためで
// ある。読めない理由がそれ以外（権限など）のときはエラーを返す。
//
// 対象は `Runner_*.log` と `Worker_*.log` だけである。`_diag` には runner 本体が
// 使う別形式のファイル（`.json` の診断情報など）も置かれるため、拡張子と接頭辞の
// 両方で絞る。
//
// 並びは更新時刻の降順、同時刻ならファイル名の降順とする。名前には採取時刻が
// 埋まっているので、更新時刻が同じ 1 秒に収まったログでも新しいものが先に来る。
func List(r runner.Runner) ([]File, error) {
	dir := filepath.Join(r.Dir, DiagDir)
	ents, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("%s を読めません: %w", dir, err)
	}

	out := make([]File, 0, len(ents))
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		kind, ok := kindOf(e.Name())
		if !ok {
			continue
		}
		info, err := e.Info()
		if err != nil {
			// 列挙してから情報を取るまでの間に消えたファイル（runner 側のローテーション）
			// は飛ばす。一覧全体を失敗させる理由にはならない。
			continue
		}
		out = append(out, File{
			Path:    filepath.Join(dir, e.Name()),
			Name:    e.Name(),
			Kind:    kind,
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}
	slices.SortStableFunc(out, byNewest)
	return out, nil
}

// LatestWorker は直近ジョブの Worker ログを返す。見つからなければ偽を返す。
//
// 「選択中 runner の直近ジョブの Worker ログを開く」ショートカット（`l`）の宛先で
// ある（FR-23〜FR-26 の補足）。List が更新時刻の降順なので、最初に見つかった
// Worker ログが直近のものである。
//
// 列挙に失敗した場合も偽を返す。呼び出し側はキー 1 打鍵の応答としてこれを使うため、
// 「開けなかった」と「そもそも無い」を分けても打つ手が変わらない。
func LatestWorker(r runner.Runner) (File, bool) {
	files, err := List(r)
	if err != nil {
		return File{}, false
	}
	for _, f := range files {
		if f.Kind == KindWorker {
			return f, true
		}
	}
	return File{}, false
}

// kindOf はファイル名からログの種類を判定する。対象外の名前には偽を返す。
func kindOf(name string) (Kind, bool) {
	if !strings.HasSuffix(name, suffixLog) {
		return KindRunner, false
	}
	switch {
	case strings.HasPrefix(name, prefixRunner):
		return KindRunner, true
	case strings.HasPrefix(name, prefixWorker):
		return KindWorker, true
	default:
		return KindRunner, false
	}
}

// byNewest は更新時刻の降順（同時刻はファイル名の降順）で並べる比較関数。
func byNewest(a, b File) int {
	if !a.ModTime.Equal(b.ModTime) {
		if a.ModTime.After(b.ModTime) {
			return -1
		}
		return 1
	}
	return strings.Compare(b.Name, a.Name)
}
