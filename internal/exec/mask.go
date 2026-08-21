package exec

import "strings"

// maskPlaceholder はマスク後の値。
const maskPlaceholder = "***"

// minSecretLen は値一致マスクの対象とする secret の最小長。
//
// 空文字や 1 文字の値をそのまま扱うと全引数が潰れて監査ログが読めなくなる。
// registration token / PAT はいずれも十分長いため、実運用の下限を大きく下回る
// 8 文字を閾値にして短い値は無視する。
const minSecretLen = 8

// MaskArgs は引数列の秘密情報を *** に置換した新しいスライスを返す。
// 入力スライスは変更しない。
//
// マスクは 2 段で、キー名ベース（段 1）の結果に値一致ベース（段 2）をかける。
// 段 1 だけではオプション名が想定外のときに漏れ、段 2 だけでは記録時点で
// 解放済みのトークンに対応できないため、両方を必ず通す
// （docs/architecture/security.md「監査ログでのマスク」）。
//
// 何度適用しても結果が変わらない（冪等）。監査ログの記録と実行前プレビュー表示の
// 両方から呼ばれ、マスク済みの値に再度かかることがあるためである。
func MaskArgs(args []string, secrets ...string) []string {
	out := maskByKey(args)
	maskByValue(out, secrets)
	return out
}

// MaskArgs は c に設定された secret 提供元の値でマスクする。
//
// パッケージ関数 MaskArgs と同名だが、実行前プレビュー（docs/ui/screens.md）では
// Executor を持っている側が provider を意識せずに呼べる方が自然なため改名しない。
func (c *Command) MaskArgs(args []string) []string {
	return MaskArgs(args, c.secrets()...)
}

// maskString は文字列中の secret を置換する。
//
// エラーメッセージにはオプション名と値の並びが保たれないため、値一致マスクのみをかける。
func maskString(s string, secrets []string) string {
	for _, secret := range secrets {
		if len(secret) < minSecretLen {
			continue
		}
		s = strings.ReplaceAll(s, secret, maskPlaceholder)
	}
	return s
}

// secrets は登録された提供元から secret を取り出す。
// New に nil が渡された場合は NoSecrets と同等に扱う。
func (c *Command) secrets() []string {
	if c.secretsFn == nil {
		return nil
	}
	return c.secretsFn()
}

// isSecretKey はオプション名が秘密情報を伴うものかを判定する。
//
// 可変なグローバル map を持たず switch で固定する。実行時に増減させる必要が無く、
// 判定の全体が 1 箇所を読むだけで分かるようにする。
//
// 先頭の - を除いて小文字化して比べるため、-token / --Token / --TOKEN も拾う。
// ハイフン 0 個でも真を返すため、「次の要素をマスクする」判定に使う側では
// ハイフンを別途必須にすること（maskByKey を参照）。
// token / pat / jitconfig は仕様が挙げるもの。password / secret は runner 以外の
// コマンドを将来通す際に取りこぼさないための一般的なキー名として加えている。
func isSecretKey(name string) bool {
	switch strings.ToLower(strings.TrimLeft(name, "-")) {
	case "token", "pat", "jitconfig", "password", "secret":
		return true
	default:
		return false
	}
}

// maskByKey は段 1。オプション名から値を特定してマスクした新しいスライスを返す。
func maskByKey(args []string) []string {
	out := make([]string, len(args))
	copy(out, args)

	for i := 0; i < len(out); i++ {
		key, _, hasValue := strings.Cut(out[i], "=")
		switch {
		case hasValue && isSecretKey(key):
			// --token=VALUE はキー名を残して値だけ置換する。何のオプションが
			// 渡されたかは監査ログとして残す必要がある。
			// ここはハイフンなしのキーも許す。TOKEN=xxx のような env 形式の引数を
			// 拾えるのは利点であり、= の右側は必ず値なので無関係な引数を潰さない。
			out[i] = key + "=" + maskPlaceholder
		case isSecretKey(out[i]) && strings.HasPrefix(out[i], "-") && i+1 < len(out):
			// --token VALUE の形。次の要素が --name のようにオプションに見えても
			// マスクする（値なのかオプションなのかを判定するより安全側を採る）。
			// 一方でハイフンは必須にする。gh auth token --hostname X や
			// gh secret set NAME のような位置引数にまで反応すると、秘密でない次の
			// 引数を潰して監査ログの追跡可能性（実行コマンドの全文）を削るためである。
			out[i+1] = maskPlaceholder
			i++
		}
	}
	return out
}

// maskByValue は段 2。保持中の secret と一致する部分を args 上で置換する。
// 呼び出し元が用意した新しいスライスを直接書き換える。
func maskByValue(args, secrets []string) {
	for _, secret := range secrets {
		if len(secret) < minSecretLen {
			continue
		}
		for i, arg := range args {
			// URL などに埋め込まれた場合に備えて部分一致で置換する。
			// 完全一致は全体が置換されるため同じ経路で扱える。
			if strings.Contains(arg, secret) {
				args[i] = strings.ReplaceAll(arg, secret, maskPlaceholder)
			}
		}
	}
}
