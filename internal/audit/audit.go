// Package audit は本ツールが実行した外部コマンドを JSON Lines で追記する。
//
// 呼び出すのは internal/exec の実行実装だけとし、ドメイン層からは直接使わない。
// 記録の起点を外部プロセス実行の 1 箇所に寄せることで、記録漏れが構造的に
// 発生しないようにしている。
//
// マスクは exec 側の責務であり、audit は受け取った値をそのまま書く。
// マスクを 2 箇所に置くと片方が古くなるため、実装を exec に一本化している。
package audit

import (
	"fmt"
	"time"
)

// Record は監査ログの 1 行。
//
// フィールドの宣言順は仕様（docs/architecture/data-model.md）の表と一致させる。
// encoding/json は宣言順に出力するため、この順序がそのまま 1 行のキー順になる。
type Record struct {
	TS       Timestamp `json:"ts"`
	UID      int       `json:"uid"`
	SudoUser string    `json:"sudo_user"`
	Action   string    `json:"action"`
	Runner   string    `json:"runner"`
	Dir      string    `json:"dir"`
	// Command は実行したコマンドと引数。マスク済みの値を渡すこと（audit はマスクしない）。
	Command    []string `json:"command"`
	ExitCode   int      `json:"exit_code"`
	DurationMS int64    `json:"duration_ms"`
	// Error は失敗時のメッセージ。成功時に空のキーを並べないため omitempty を付ける。
	Error string `json:"error,omitempty"`
}

// Timestamp は Record の ts フィールド。
//
// RFC 3339（小数秒なし・ローカルオフセット）で出力する。ログを人が grep する
// 前提のため、UTC への正規化や小数秒は付けない。
type Timestamp time.Time

// MarshalJSON は Timestamp を RFC 3339 の文字列として出力する。
func (t Timestamp) MarshalJSON() ([]byte, error) {
	// 書式に JSON のエスケープ対象文字は現れないため、そのまま引用符で囲む。
	b := make([]byte, 0, len(time.RFC3339)+2)
	b = append(b, '"')
	b = time.Time(t).AppendFormat(b, time.RFC3339)
	b = append(b, '"')
	return b, nil
}

// UnmarshalJSON は RFC 3339 の文字列を Timestamp として読み戻す。
//
// MarshalJSON と対にしておくことで、監査ログを Record へ読み戻す機能を将来
// 作ったときに json.Unmarshal がそのまま使える。
func (t *Timestamp) UnmarshalJSON(b []byte) error {
	// JSON の null は「値なし」なのでゼロ値のまま受け入れる（time.Time と同じ流儀）。
	if string(b) == "null" {
		return nil
	}
	v, err := time.Parse(`"`+time.RFC3339+`"`, string(b))
	if err != nil {
		return fmt.Errorf("ts の解釈に失敗しました: %w", err)
	}
	*t = Timestamp(v)
	return nil
}
