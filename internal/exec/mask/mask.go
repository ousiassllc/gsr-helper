// Package mask は監査ログとエラー文に残す値から秘密情報を落とす。
//
// internal/exec から分離しているのは次の 2 点による。
//   - マスクは「引数列と文字列を受けて置換した値を返す」純粋な文字列処理であり、
//     プロセス起動・タイムアウト・監査記録とは独立した責務である。
//   - 実行前プレビュー（docs/ui/screens.md）は Executor を持たない場所からも
//     マスク結果だけを必要とする。実行実装に同居させると、表示のためだけに
//     プロセス起動の実装を import することになる。
package mask

import (
	"slices"
	"strings"
)

// Placeholder はマスク後の値。
const Placeholder = "***"

// minSecretLen は値一致マスクの対象とする secret の最小長。
//
// 空文字や 1 文字の値をそのまま扱うと全引数が潰れて監査ログが読めなくなる。
// registration token / PAT はいずれも十分長いため、実運用の下限を大きく下回る
// 8 文字を閾値にして短い値は無視する。この下限に届かない値は段 2 では拾えないが、
// 段 1（キー名ベース）は長さを見ないため --token の値は長さに関わらずマスクされる。
const minSecretLen = 8

// secretKeys は完全一致で秘密情報を伴うと判断するオプション名。
//
// 可変なグローバル map を持たず固定の並びにする。実行時に増減させる必要が無く、
// 判定の全体が 1 箇所を読むだけで分かるようにする。
// token / pat / jitconfig は仕様が挙げるもの。password / secret は runner 以外の
// コマンドを将来通す際に取りこぼさないための一般的なキー名として加えている。
var secretKeys = []string{"token", "pat", "jitconfig", "password", "secret"}

// secretSubstrings は部分一致で秘密情報を伴うと判断する語。
//
// これらは --api-key や "Authorization: Bearer x" のように前後に語を伴う形で
// 現れるため、完全一致では拾えない。逆に token を部分一致にはしない。
// --tokens のような無関係なキーまで拾い、監査ログの追跡可能性を削るためである。
var secretSubstrings = []string{"authorization", "bearer", "apikey", "api-key", "api_key"}

// headerKeys は「次の要素が HTTP ヘッダ 1 行」であることを示すオプション名。
// gh api -H "Authorization: Bearer x" のような形を拾う。
var headerKeys = []string{"h", "header"}

// Args は引数列の秘密情報を *** に置換した新しいスライスを返す。
// 入力スライスは変更しない。
//
// マスクは 2 段で、キー名ベース（段 1）の結果に値一致ベース（段 2）をかける。
// 段 1 だけではオプション名が想定外のときに漏れ、段 2 だけでは記録時点で
// 解放済みのトークンに対応できないため、両方を必ず通す
// （docs/architecture/security.md「監査ログでのマスク」）。
//
// 何度適用しても結果が変わらない（冪等）。監査ログの記録と実行前プレビュー表示の
// 両方から呼ばれ、マスク済みの値に再度かかることがあるためである。
func Args(args []string, secrets ...string) []string {
	out := byKey(args)
	byValue(out, secrets)
	return out
}

// String は文字列中の secret を置換する。
//
// エラーメッセージにはオプション名と値の並びが保たれないため、値一致マスクのみをかける。
func String(s string, secrets []string) string {
	for _, secret := range secrets {
		if len(secret) < minSecretLen {
			continue
		}
		s = strings.ReplaceAll(s, secret, Placeholder)
	}
	return s
}

// normalizeKey は先頭のハイフンを除いて小文字化する。
// -token / --Token / --TOKEN を同じキーとして扱うため。
func normalizeKey(name string) string {
	return strings.ToLower(strings.TrimLeft(name, "-"))
}

// isSecretKey はオプション名が秘密情報を伴うものかを判定する。
//
// ハイフン 0 個でも真を返すため、「次の要素をマスクする」判定に使う側では
// ハイフンを別途必須にすること（byKey を参照）。
func isSecretKey(name string) bool {
	key := normalizeKey(name)
	if slices.Contains(secretKeys, key) {
		return true
	}
	for _, w := range secretSubstrings {
		if strings.Contains(key, w) {
			return true
		}
	}
	return false
}

// isHeaderKey はオプション名が HTTP ヘッダを伴うものかを判定する。
func isHeaderKey(name string) bool {
	return slices.Contains(headerKeys, normalizeKey(name))
}

// isSecretFlag はハイフン付きの秘密情報キー（= 値ではなくオプション名）かを返す。
func isSecretFlag(arg string) bool {
	return strings.HasPrefix(arg, "-") && (isSecretKey(arg) || isHeaderKey(arg))
}

// byKey は段 1。オプション名から値を特定してマスクした新しいスライスを返す。
//
// 判定は必ず入力の args を見る。書き込み先の out を見ると、直前に *** で
// 潰した要素をキーとして読み直すことになり、判定が結果に依存してしまう。
func byKey(args []string) []string {
	out := make([]string, len(args))
	copy(out, args)

	for i := 0; i < len(args); i++ {
		if key, val, ok := strings.Cut(args[i], "="); ok {
			out[i] = maskInline(key, val)
			continue
		}
		// 次の要素を値として扱うのはハイフン付きのキーのときだけ。
		// gh auth token --hostname X や gh secret set NAME のような位置引数に
		// まで反応すると、秘密でない次の引数を潰して監査ログの追跡可能性
		// （実行コマンドの全文）を削るためである。
		if !strings.HasPrefix(args[i], "-") || i+1 >= len(args) {
			continue
		}
		switch {
		case isHeaderKey(args[i]):
			out[i+1] = maskHeader(args[i+1])
		case isSecretKey(args[i]):
			// --token VALUE の形。次の要素が --name のようにオプションに見えても
			// マスクする（値なのかオプションなのかを判定するより安全側を採る）。
			out[i+1] = Placeholder
		default:
			continue
		}
		if !isSecretFlag(args[i+1]) {
			// マスクした値は読み飛ばす。ただし飛ばす先が自身も秘密情報キーの形
			// （--token --token SECRET）なら飛ばさない。飛ばすと本物の値である
			// その次の要素が素のまま監査ログに残る。
			i++
		}
	}
	return out
}

// maskInline は KEY=VALUE 形式の 1 要素をマスクする。
// キー名は残して値だけ置換する。何のオプションが渡されたかは監査ログとして
// 残す必要があるためである。ハイフンなしのキーも許す（TOKEN=xxx のような env
// 形式の引数を拾えるのは利点であり、= の右側は必ず値なので誤爆しない）。
func maskInline(key, val string) string {
	switch {
	case isHeaderKey(key):
		return key + "=" + maskHeader(val)
	case isSecretKey(key):
		return key + "=" + Placeholder
	default:
		return key + "=" + val
	}
}

// maskHeader は "Name: value" 形式のヘッダ 1 行をマスクする。
//
// ヘッダ名が秘密情報を示すものだけを対象にし、値だけを置換する。
// -H "Accept: application/vnd.github+json" のような無関係なヘッダを潰すと
// 何を送ったのか追跡できなくなるためである。
func maskHeader(v string) string {
	name, _, ok := strings.Cut(v, ":")
	if !ok || !isSecretKey(name) {
		return v
	}
	return name + ": " + Placeholder
}

// byValue は段 2。保持中の secret と一致する部分を args 上で置換する。
// 呼び出し元が用意した新しいスライスを直接書き換える。
func byValue(args, secrets []string) {
	for _, secret := range secrets {
		if len(secret) < minSecretLen {
			continue
		}
		for i, arg := range args {
			// URL などに埋め込まれた場合に備えて部分一致で置換する。
			// 完全一致は全体が置換されるため同じ経路で扱える。
			if strings.Contains(arg, secret) {
				args[i] = strings.ReplaceAll(arg, secret, Placeholder)
			}
		}
	}
}
