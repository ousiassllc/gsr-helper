// Package valid は runner の追加・削除・更新に渡す入力の検証を担う。
//
// docs/architecture/security.md「入力を検証してから渡す」の表を実装する。
// internal/setup から分離しているのは、行数上限（1 ディレクトリ 2000 行）の
// ためと、検証が「外部コマンドを一切起動しない純粋な判定」であって計画の
// 組み立てとは独立した責務だからである。
package valid

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// 入力検証のエラー。docs/architecture/security.md「入力を検証してから渡す」の表に対応する。
var (
	// ErrEmptyName は runner 名が空の場合のエラー。
	ErrEmptyName = errors.New("runner 名が空です")
	// ErrLeadingDash は先頭が - の値を拒否した場合のエラー。
	ErrLeadingDash = errors.New("先頭が - の値は使えません（コマンドのオプションと解釈されます）")
	// ErrBadNameChar は runner 名に使えない文字が含まれる場合のエラー。
	ErrBadNameChar = errors.New("runner 名に使える文字は英数字と - _ . だけです")
	// ErrNameTooLong は runner 名が長すぎる場合のエラー。
	ErrNameTooLong = errors.New("runner 名が長すぎます")
	// ErrDuplicateName は既存 runner と名前が重複する場合のエラー。
	ErrDuplicateName = errors.New("同じ名前の runner が既にあります")
	// ErrReservedLabel は予約ラベルを指定した場合のエラー。
	ErrReservedLabel = errors.New("self-hosted / linux / x64 は自動で付くため指定できません")
	// ErrBadLabelChar はラベルに使えない文字が含まれる場合のエラー。
	ErrBadLabelChar = errors.New("ラベルに使える文字は英数字と - _ . だけです")
	// ErrNotAbs は絶対パスでない場合のエラー。
	ErrNotAbs = errors.New("絶対パスを指定してください")
	// ErrHasDotDot はパスに .. が含まれる場合のエラー。
	ErrHasDotDot = errors.New(".. を含むパスは使えません")
	// ErrNotGitHub は GitHub 以外のホストを指定した場合のエラー。
	ErrNotGitHub = errors.New("GitHub の URL を指定してください")
	// ErrURLUserInfo は URL に認証情報を埋め込んだ場合のエラー。
	//
	// `https://x:<PAT>@github.com/...` は config.sh --url にそのまま渡り、確認
	// プレビューにも監査ログにも平文で載る。利用者が打ち込んだ PAT は
	// gh.Secrets に登録されないため、値一致マスク（exec/mask の段 2）では
	// 消せない（internal/exec/command/command.go が言う「URL 埋め込み」）。
	// 消せない以上、そもそも受け取らない。
	ErrURLUserInfo = errors.New("URL に認証情報を埋め込まないでください")
	// ErrBadCount は台数が範囲外の場合のエラー。
	ErrBadCount = errors.New("追加する台数は 1〜50 の範囲で指定してください")
)

const (
	// maxNameLen は runner 名の長さの上限。GitHub 側の上限に合わせる。
	maxNameLen = 64
	// maxLabelLen はラベル 1 つの長さの上限。
	maxLabelLen = 64
	// maxAddCount は 1 回の一括追加で作れる台数の上限。
	//
	// 想定は 1 ホスト 20 台程度（docs/requirements/non-functional.md）だが、
	// 打ち間違いで 3 桁の台数を登録してしまう事故を防ぐために上限を置く。
	maxAddCount = 50
)

// reservedLabels は runner が自動で付けるため指定できないラベル。
func reservedLabels() []string { return []string{"self-hosted", "linux", "x64"} }

// Name は runner 名を検証する。
//
// existing はホスト内の既存 runner 名。重複を弾くために渡す。
func Name(name string, existing []string) error {
	if name == "" {
		return ErrEmptyName
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("runner 名 %q: %w", name, ErrLeadingDash)
	}
	if len(name) > maxNameLen {
		return fmt.Errorf("runner 名 %q: %w（%d 文字まで）", name, ErrNameTooLong, maxNameLen)
	}
	if !isSafeToken(name) {
		return fmt.Errorf("runner 名 %q: %w", name, ErrBadNameChar)
	}
	for _, e := range existing {
		if e == name {
			return fmt.Errorf("runner 名 %q: %w", name, ErrDuplicateName)
		}
	}
	return nil
}

// Labels はラベル列を検証し、trim と重複除去を済ませたものを返す。
func Labels(labels []string) ([]string, error) {
	out := make([]string, 0, len(labels))
	seen := make(map[string]struct{}, len(labels))

	for _, raw := range labels {
		l := strings.TrimSpace(raw)
		if l == "" {
			continue
		}
		if strings.HasPrefix(l, "-") {
			return nil, fmt.Errorf("ラベル %q: %w", l, ErrLeadingDash)
		}
		if len(l) > maxLabelLen {
			return nil, fmt.Errorf("ラベル %q: 長すぎます（%d 文字まで）", l, maxLabelLen)
		}
		if !isSafeToken(l) {
			return nil, fmt.Errorf("ラベル %q: %w", l, ErrBadLabelChar)
		}
		for _, r := range reservedLabels() {
			if strings.EqualFold(l, r) {
				return nil, fmt.Errorf("ラベル %q: %w", l, ErrReservedLabel)
			}
		}
		if _, dup := seen[strings.ToLower(l)]; dup {
			continue
		}
		seen[strings.ToLower(l)] = struct{}{}
		out = append(out, l)
	}
	return out, nil
}

// Dir は絶対パスであることと .. を含まないことを検証し、Clean 済みを返す。
//
// .. の判定を Clean の前に行うのは、`/opt/../etc` のような指定が Clean 後には
// 正当な絶対パスに見えてしまうためである（docs/architecture/data-model.md）。
func Dir(field, path string) (string, error) {
	p := strings.TrimSpace(path)
	if p == "" || !filepath.IsAbs(p) {
		return "", fmt.Errorf("%s: %w", field, ErrNotAbs)
	}
	for _, seg := range strings.Split(p, string(filepath.Separator)) {
		if seg == ".." {
			return "", fmt.Errorf("%s: %w", field, ErrHasDotDot)
		}
	}
	return filepath.Clean(p), nil
}

// URL は登録先の URL を検証する。ホストが GitHub であることを求める。
//
// 認証情報を埋め込んだ URL も拒否する。エラー文言に入力の一部でも載せない。
// 載せると、埋め込まれた認証情報を守るための検証がその認証情報を監査ログへ
// 書き出すことになる。
//
// 認証情報の判定は url.Parse より前に行う。`https://x:pat%@github.com/...` の
// ように解析自体が失敗する入力では u.User を見る機会がなく、解析エラーの側で
// 弾くことになるためである。どちらの経路でも同じ ErrURLUserInfo を返し、
// 利用者に「認証情報を外せば通る」と伝わるようにする。
func URL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", ErrNotGitHub
	}
	if hasUserInfo(s) {
		return "", fmt.Errorf("登録先の URL: %w", ErrURLUserInfo)
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("登録先の URL: %w", ErrNotGitHub)
	}
	if u.User != nil {
		return "", fmt.Errorf("登録先の URL: %w", ErrURLUserInfo)
	}
	if u.Scheme != "https" || !isGitHubHost(u.Host) {
		return "", fmt.Errorf("登録先の URL: %w", ErrNotGitHub)
	}
	return strings.TrimSuffix(s, "/"), nil
}

// hasUserInfo は URL の authority 部に @ があるか、つまり利用者名やパスワードが
// 埋め込まれているかを返す。
//
// url.Parse に頼らないのは、解析が失敗する入力でも認証情報を検出するためである。
// 判定対象を "://" から最初の / ? # までに限るのは、パスやクエリに含まれる @
// （`https://github.com/orgs/a@b` など）を認証情報と誤認しないためである。
func hasUserInfo(s string) bool {
	const sep = "://"
	i := strings.Index(s, sep)
	if i < 0 {
		return false
	}
	authority := s[i+len(sep):]
	if j := strings.IndexAny(authority, "/?#"); j >= 0 {
		authority = authority[:j]
	}
	return strings.Contains(authority, "@")
}

// isGitHubHost はホスト名が GitHub のものかを返す。
//
// GitHub Enterprise Server の独自ホストは対象外である。判定を緩めると、
// 登録先を取り違えたまま短命トークンを送ってしまう。
func isGitHubHost(host string) bool {
	h := strings.ToLower(host)
	return h == "github.com" || h == "www.github.com"
}

// Count は一括追加の台数を検証する。
func Count(n int) error {
	if n < 1 || n > maxAddCount {
		return fmt.Errorf("台数 %d: %w", n, ErrBadCount)
	}
	return nil
}

// isSafeToken は英数字と - _ . だけで構成されるかを返す。
func isSafeToken(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.':
		default:
			return false
		}
	}
	return s != ""
}
