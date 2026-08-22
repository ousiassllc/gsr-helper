package audit

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// dirMode / fileMode は監査ログのパーミッション。
// 監査ログには実行したコマンド全文が残るため、root 以外に読ませない。
const (
	dirMode  os.FileMode = 0o700
	fileMode os.FileMode = 0o600
)

// Open は path に追記する Logger を返す。opts は New と同じ設定を受け付ける。
//
// 可変長の設定を受けるのは、識別子（uid / sudo_user）を呼び出し側が明示できる
// ようにするためである。既定は環境変数の生値だが、cmd は検証済みの SUDO_USER を
// 持っているので、それを渡せる経路が必要になる。
//
// 親ディレクトリが無ければ 0o700 で作る。既存ディレクトリのモードは変更しない
// （/var/log 配下など、本ツールが作っていないディレクトリの権限を勝手に変えない）。
//
// ファイルの扱いは新規と既存で分ける。
//   - 新規に作れた場合: 自分が作ったファイルなので 0o600 に締める。
//   - 既存ファイルだった場合: 本ツールの監査ログとして妥当か検証したうえで開き、
//     モードは変更しない（validateAuditFile と openExistingAuditFile を参照）。
//   - 検証に落ちた場合: 開かずにエラーを返す。
//
// 返した Logger は使い終わったら Close すること。logrotate が path を差し替えた
// 場合は書き込み時に開き直す（rotationAwareWriter を参照）。
func Open(path string, opts ...Option) (*Logger, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return nil, fmt.Errorf("%s の作成に失敗しました: %w", dir, err)
	}

	f, err := openAuditFile(path)
	if err != nil {
		return nil, err
	}

	w, err := newRotationAwareWriter(path, f)
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	l := New(w, opts...)
	l.closer = w
	return l, nil
}

// openAuditFile は path を監査ログとして追記用に開く。
//
// まず O_EXCL 付きで新規作成を試し、成功した場合だけ Chmod する。既存ファイルへ
// 回った場合は検証のみを行い Chmod しない。自分が作っていないファイルの権限を
// root 権限で変えてやることが、まさに攻撃者の狙いだからである（audit_log のパスは
// 非特権の SUDO_USER が所有する設定ファイル由来で、実質的に外部入力である）。
func openAuditFile(path string) (*os.File, error) {
	// O_EXCL により「今この呼び出しで作った」ことが保証される。先置きされた
	// ファイルやリンクを掴む余地がないため、この経路だけは無条件に信頼できる。
	//nolint:gosec // path は非特権ユーザーが持つ設定ファイル（audit_log）由来の実質的な外部入力。O_EXCL / O_NOFOLLOW と openExistingAuditFile の検証で安全性を担保している。
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, fileMode)
	switch {
	case err == nil:
		// OpenFile のモード引数は umask で削られるため、作成直後に明示的に締める。
		if cerr := f.Chmod(fileMode); cerr != nil {
			_ = f.Close()
			return nil, fmt.Errorf("%s のパーミッション設定に失敗しました: %w", path, cerr)
		}
		return f, nil
	case errors.Is(err, os.ErrExist):
		return openExistingAuditFile(path)
	default:
		return nil, fmt.Errorf("%s のオープンに失敗しました: %w", path, err)
	}
}

// openExistingAuditFile は既存ファイルを開き、本ツールの監査ログとして妥当かを
// 検証する。妥当でなければ開いた fd を閉じてエラーを返す。
//
// O_NOFOLLOW で最終要素がシンボリックリンクなら開かずに失敗させる。本ツールは
// sudo 前提で動くため、親ディレクトリが他ユーザーに書けるときに攻撃者が
// audit.jsonl を任意ファイルへのリンクとして先置きすると、root が追記を
// 代行させられる。docs/architecture/security.md が「シンボリックリンク経由の
// 逸脱」を脅威に挙げているため、ここで断つ。
//
// 検証は開いた fd 上の Fstat だけで行い、パスからの再 stat はしない。パスを
// 見直すと検証した対象と書き込む対象がずれ、TOCTOU を作り込むことになる。
//
// 検証に落ちた場合のエラーは cmd/gsr-helper 側で audit.Discard() への降格
// （stderr に警告）として扱われるため、ここで失敗させるのが意図した挙動である。
func openExistingAuditFile(path string) (*os.File, error) {
	//nolint:gosec // path は非特権ユーザーが持つ設定ファイル（audit_log）由来の実質的な外部入力。O_NOFOLLOW と直後の validateAuditFile による fd 上の検証で安全性を担保している。
	f, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("%s のオープンに失敗しました: %w", path, err)
	}
	if err := validateAuditFile(f); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("%s は本ツールの監査ログではありません: %w", path, err)
	}
	return f, nil
}

// validateAuditFile は開いた fd が本ツールの監査ログとして妥当かを検証する。
//
// 通常ファイルであること・ハードリンクが 1 本だけであること・実効 uid の所有で
// あること・内容が JSON Lines であること（空か先頭が '{'）を確認する。攻撃者が
// 書ける親ディレクトリに /etc/passwd へのハードリンクや空ファイルを先置きしても、
// これらのいずれかで弾かれる。
func validateAuditFile(f *os.File) error {
	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("状態の取得に失敗しました: %w", err)
	}
	if !fi.Mode().IsRegular() {
		return errors.New("通常ファイルではありません")
	}

	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("ファイルの所有者を確認できません")
	}
	// 別名で置かれたハードリンクは、追記先が監査ログ以外にも見えることを意味する。
	if st.Nlink != 1 {
		return errors.New("ハードリンクが張られています")
	}
	euid := os.Geteuid()
	if euid < 0 || uint64(st.Uid) != uint64(euid) {
		return errors.New("本プロセスの所有ファイルではありません")
	}

	// JSON Lines の先頭バイトは必ず '{'。/etc/passwd や unit ファイル、シェルの
	// プロファイルなどはこれで弾ける。追記位置を動かさないよう ReadAt で読む。
	var head [1]byte
	n, err := f.ReadAt(head[:], 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("先頭の読み取りに失敗しました: %w", err)
	}
	if n == 1 && head[0] != '{' {
		return errors.New("JSON Lines ではありません")
	}
	return nil
}
