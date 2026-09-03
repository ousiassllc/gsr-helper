// Package tarball は GitHub Actions runner の tarball の取得・検証・展開を扱う。
//
// 取得元とチェックサムは downloads エンドポイントが返す値をそのまま使う
// （internal/gh の Download を Info に詰め替える）。自前でハッシュ一覧は持たない。
//
// 展開の前に SHA-256 を必ず検証する。検証に失敗した場合は展開せず中止する
// （docs/architecture/security.md）。root 権限で動くため、展開は tar の中身を
// 信用せず、os.Root を根に固定した上で経路の逸脱を明示的に弾く。
package tarball

import "errors"

// ErrNoURL は取得先 URL が空の場合のエラー。
var ErrNoURL = errors.New("tarball の URL が指定されていません")

// ErrNoChecksum は期待する SHA-256 が空の場合のエラー。
//
// 検証を省いた展開を許すと security.md の前提が崩れるため、チェックサム無しの
// 取得は成功させずにここで止める。
var ErrNoChecksum = errors.New("期待する SHA-256 が指定されていません")

// ErrChecksumMismatch は SHA-256 が期待値と一致しない場合のエラー。
var ErrChecksumMismatch = errors.New("tarball の SHA-256 が期待値と一致しません")

// ErrUnexpectedStatus は tarball の取得が 200 以外で終了した場合のエラー。
var ErrUnexpectedStatus = errors.New("tarball の取得が想定外の HTTP ステータスで終了しました")

// ErrUnsafePath は tar のエントリが展開先の外を指している場合のエラー。
var ErrUnsafePath = errors.New("tar のエントリが展開先の外を指しています")

// ErrTooLarge は展開後のサイズが上限を超えた場合のエラー。
var ErrTooLarge = errors.New("tar の展開サイズが上限を超えています")

// ErrPreservedLink は保持対象へリンクで潜り込むエントリを拒否した場合のエラー（FR-21）。
//
// `.runner` を指すリンクとその配下のエントリを並べれば、保持対象の中身を書き換え
// られる。展開先の外へは出ないため os.Root では止まらず、runner の登録情報
// （.runner / .credentials）がバージョン更新のたびに壊れる。正規の tarball が
// こうしたリンクを含むことは無いので、読み飛ばさず展開そのものを中止する。
var ErrPreservedLink = errors.New("tar のエントリが保持対象へリンクで潜り込んでいます")

// Info は取得する tarball の情報。internal/gh の Download から詰め替えて渡す。
type Info struct {
	// URL は tarball の取得先。
	URL string
	// Filename は保存するファイル名。空なら URL の末尾要素を使う。
	Filename string
	// SHA256 は期待するチェックサム（16 進小文字）。空は許容しない。
	SHA256 string
}

// Progress はダウンロードの進捗。Total が 0 なら総量不明。
type Progress struct {
	// Downloaded は現在までに受け取ったバイト数。
	Downloaded int64
	// Total は総バイト数。Content-Length が無ければ 0。
	Total int64
}

// PreservedNames はバージョン更新時に上書きしてはならない名前を返す（FR-21）。
//
// runner 自身が持つ登録情報・設定・作業領域であり、tarball 側の同名エントリで
// 潰すと登録がやり直しになる。呼び出し側が書き換えても影響しないよう、
// 毎回新しいスライスを返す。
func PreservedNames() []string {
	return []string{".runner", ".credentials", ".env", ".path", "_work", "_diag"}
}
