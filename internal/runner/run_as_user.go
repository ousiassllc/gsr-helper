package runner

import (
	"os/user"
	"strconv"
)

// resolveRunAsUser は runner の実行ユーザーを決める。
// lookup は UID からユーザー名を引く関数（テストで差し替える）。
// systemd ユニットの User= を第一の情報源とし、ユニットの実体が無い場合は
// Listener プロセスの所有者から引く。どちらも取れなければ空を返す。
func resolveRunAsUser(r Runner, lookup func(uid int) string) string {
	// show に失敗した場合（Unit だけのプレースホルダ）と not-found は
	// systemd.State.Exists が偽になる。どちらも User= が埋まっていないため、
	// ここで弾いてプロセス側にフォールバックする。
	if r.Svc != nil && r.Svc.Exists() {
		if r.Svc.User != "" {
			return r.Svc.User
		}
		return "root" // systemd の User= 未指定は root で起動する
	}
	if r.Listener != nil && r.Listener.UID >= 0 {
		return lookup(r.Listener.UID)
	}
	return ""
}

// newUserLookup は UID からユーザー名を引く関数を返す。
// 多数の runner が同じユーザーで動くため、NSS への問い合わせが UID ごとに 1 回で
// 済むよう結果を memo する。引けない場合（静的リンクで NSS が使えない、LDAP 上の
// ユーザーなど）は UID の 10 進表記を返す。
// memo は排他しないため、返した関数は 1 回の探索の中で直列に使う。
func newUserLookup() func(uid int) string {
	cache := map[int]string{}
	return func(uid int) string {
		if name, ok := cache[uid]; ok {
			return name
		}
		name := strconv.Itoa(uid)
		if u, err := user.LookupId(name); err == nil && u.Username != "" {
			name = u.Username
		}
		cache[uid] = name
		return name
	}
}
