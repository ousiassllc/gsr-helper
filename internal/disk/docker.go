package disk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/exec"
)

// actionDF は docker の使用量取得を表す監査ログの action。
// 読み取り専用だが SkipAudit は付けない。再検出のように繰り返し発行されるものではなく、
// 記録を落とす理由が無いためである（docs/architecture/security.md「監査ログ」）。
const actionDF = "disk.df"

// dockerLabels は docker system df の Type に対する表示名。
// 画面には日本語の内訳を出すため、docker の英語の種別をここで訳す。
var dockerLabels = map[string]string{
	"Images":        "docker / イメージ",
	"Containers":    "docker / コンテナ",
	"Local Volumes": "docker / ボリューム",
	"Build Cache":   "docker / ビルドキャッシュ",
}

// dfLine は docker system df --format {{json .}} が 1 行ごとに出す JSON。
// Size / Reclaimable は "5.2GB" / "3.1GB (59%)" のような人間可読の文字列である。
type dfLine struct {
	Type        string
	Size        string
	Reclaimable string
}

// DockerUsage は docker の使用量の内訳を返す（FR-27）。
//
// docker が無い環境で呼ばないのは呼び出し側（能力判定 Caps.Docker）の責務だが、
// ここでも panic せずエラーを返すだけにしてある。呼び出し側は docker の行を出さずに
// 縮退できる。
func DockerUsage(ctx context.Context, ex exec.Executor) ([]DockerItem, error) {
	ctx = exec.WithOptions(ctx, exec.Options{
		Action: actionDF, Runner: "", Dir: "", Env: nil, SkipAudit: false,
	})

	res, err := ex.Run(ctx, "docker", "system", "df", "--format", "{{json .}}")
	if err != nil {
		return nil, fmt.Errorf("docker system df の実行に失敗しました: %w", err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("docker system df が終了コード %d で終了しました", res.ExitCode)
	}
	if strings.TrimSpace(string(res.Stdout)) == "" {
		return nil, errors.New("docker system df の出力が空です")
	}

	items := parseDF(string(res.Stdout))
	if len(items) == 0 {
		return nil, errors.New("docker system df の出力を解析できませんでした")
	}
	return items, nil
}

// parseDF は docker system df の出力を 1 行 1 JSON として解析する。
// 解析できない行は飛ばす。docker の版によって行が増えても、読める行だけで内訳を出せる。
func parseDF(stdout string) []DockerItem {
	var items []DockerItem
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var l dfLine
		if err := json.Unmarshal([]byte(line), &l); err != nil || l.Type == "" {
			continue
		}
		items = append(items, DockerItem{
			Type:        l.Type,
			Label:       dockerLabel(l.Type),
			Size:        parseDockerSize(l.Size),
			Reclaimable: parseDockerSize(l.Reclaimable),
		})
	}
	return items
}

// dockerLabel は種別の表示名を返す。未知の種別は docker の表記をそのまま添える。
// 訳が無いことを理由に行ごと落とすと、使用量の合計が画面と合わなくなるためである。
func dockerLabel(typ string) string {
	if label, ok := dockerLabels[typ]; ok {
		return label
	}
	return "docker / " + typ
}

// sizeUnits は docker が出す単位と倍率。接尾辞の長い順に並べる。
// docker は 10 進接頭辞（kB/MB/GB）と 2 進接頭辞（KiB/MiB/GiB）の両方を出しうるため、
// どちらも扱う。判定は大文字化して行うので "kB" と "KB" は同じ。
var sizeUnits = []struct {
	suffix string
	mul    float64
}{
	{"KIB", 1 << 10}, {"MIB", 1 << 20}, {"GIB", 1 << 30}, {"TIB", 1 << 40}, {"PIB", 1 << 50},
	{"KB", 1e3}, {"MB", 1e6}, {"GB", 1e9}, {"TB", 1e12}, {"PB", 1e15},
	{"B", 1},
}

// parseDockerSize は "5.2GB" / "3.1GB (59%)" / "0B" / "-" のような表記をバイト数にする。
// 解析できない値は 0 を返す。docker の表記が変わっても内訳の行自体は残したいためで、
// 数えられなかった容量を推測で埋めるよりは 0 と出すほうが誤解が少ない。
func parseDockerSize(s string) int64 {
	// Reclaimable は "3.1GB (59%)" のように割合が続く。割合は使わないので落とす。
	if i := strings.IndexRune(s, '('); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return 0
	}

	upper := strings.ToUpper(s)
	for _, u := range sizeUnits {
		if !strings.HasSuffix(upper, u.suffix) {
			continue
		}
		num := strings.TrimSpace(s[:len(s)-len(u.suffix)])
		v, err := strconv.ParseFloat(num, 64)
		if err != nil || v < 0 {
			return 0
		}
		return int64(v * u.mul)
	}
	return 0
}
