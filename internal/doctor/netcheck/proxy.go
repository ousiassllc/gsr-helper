package netcheck

import (
	"context"
	"net/url"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
)

// proxyVar は同じ意味を持つ小文字・大文字の環境変数の対。
//
// 対で持つのは、**どちらが優先されるかがライブラリごとに違う**ためである。
// Go の net/http は小文字を先に見るが、curl や docker は大文字を見る。値が
// 食い違うと runner 本体とジョブが起動する道具で別のプロキシを使い、片方だけ
// 通るという追いにくい状態になる。
type proxyVar struct {
	lower string
	upper string
}

// proxyVars は整合を確かめる環境変数。
var proxyVars = []proxyVar{
	{lower: "http_proxy", upper: "HTTP_PROXY"},
	{lower: "https_proxy", upper: "HTTPS_PROXY"},
	{lower: "no_proxy", upper: "NO_PROXY"},
}

// localBypass は no_proxy に入っていてほしい宛先。
//
// self-hosted runner は自分自身やホスト内のサービスへも繋ぐ。これがプロキシへ
// 流れると、外向きには正常なのに内部通信だけが失敗する。
var localBypass = []string{"localhost", "127.0.0.1"}

// proxyCheck はプロキシ環境変数の整合を確かめる（security.md「プロキシ環境変数を
// 尊重し、doctor で設定の整合性を確認する」）。
//
// **値そのものは資格情報を含みうる。** http://user:pass@proxy:3128 の形が使われる
// ため、画面に出す URL は必ず redact() を通す。パースできない値やホストを取り出せ
// ない値は伏せる位置を特定できないので、値ごと maskedValue へ潰して一切出さない。
// エラー文言も出さない。*url.Error は生の URL を埋め込むため、そのまま連結すると
// パスワードが画面へ写る。
type proxyCheck struct{}

func (proxyCheck) ID() string       { return "net.proxy" }
func (proxyCheck) Category() string { return check.CatNetwork }
func (proxyCheck) Startup() bool    { return false }

// Run はホスト全体で 1 行を返す。
func (c proxyCheck) Run(_ context.Context, in check.Input) []check.Result {
	set := readProxyVars(in)
	if len(set) == 0 {
		return []check.Result{check.Of(c, check.Result{
			Status:  check.OK,
			Summary: "プロキシ設定なし",
			Detail:  "プロキシの環境変数は設定されていません。",
		})}
	}

	if r, bad := c.invalid(set); bad {
		return []check.Result{r}
	}
	if r, bad := c.conflicting(in); bad {
		return []check.Result{r}
	}
	if r, bad := c.missingBypass(in); bad {
		return []check.Result{r}
	}

	return []check.Result{check.Of(c, check.Result{
		Status:  check.OK,
		Summary: "プロキシ設定は整合している",
		Detail:  "設定値: " + strings.Join(describe(set), " / "),
	})}
}

// invalid は URL として読めない値があれば FAIL の行を返す。
//
// 読めない値は「プロキシが効いていない」ではなく「全ての通信が失敗する」に
// 直結するので、整合の乱れ（WARN）より重い。
//
// 文言には変数名しか出さない。値が URL として壊れている時点で userinfo の位置を
// 特定できず、値も url.Parse のエラー文言も生のパスワードを含みうるためである。
func (c proxyCheck) invalid(set map[string]string) (check.Result, bool) {
	for _, name := range sortedNames(set) {
		if isNoProxy(name) {
			continue // 宛先の並びであって URL ではない。
		}
		u, err := url.Parse(set[name])
		if err != nil {
			return check.Of(c, check.Result{
				Status:  check.Fail,
				Summary: name + " が URL として読めない",
				Detail:  name + " の値を URL として解釈できません。値は資格情報を含みうるため伏せています。",
				Impact:  "プロキシを経由する通信がすべて失敗します。",
				Remedy:  "http://host:port の形式で設定し直してください。",
			}), true
		}
		if u.Scheme == "" || u.Host == "" {
			return check.Of(c, check.Result{
				Status:  check.Fail,
				Summary: name + " にスキームかホストが無い",
				Detail:  name + " = " + redact(set[name]) + " にはスキームまたはホストがありません。",
				Impact:  "プロキシを経由する通信がすべて失敗します。",
				Remedy:  "http://host:port の形式で設定し直してください。",
			}), true
		}
	}
	return check.Result{}, false
}

// conflicting は小文字と大文字で値が食い違う対があれば WARN の行を返す。
func (c proxyCheck) conflicting(in check.Input) (check.Result, bool) {
	for _, v := range proxyVars {
		lo, up := in.Env(v.lower), in.Env(v.upper)
		if lo == "" || up == "" || lo == up {
			continue
		}
		return check.Of(c, check.Result{
			Status:  check.Warn,
			Summary: v.lower + " と " + v.upper + " の値が違う",
			Detail: v.lower + " = " + display(v.lower, lo) + " / " + v.upper + " = " + display(v.upper, up) +
				"。どちらが使われるかは道具ごとに異なります。",
			Impact: "runner 本体とジョブが起動する道具で別のプロキシを使い、片方だけ通る状態になります。",
			Remedy: "小文字と大文字に同じ値を設定してください。",
		}), true
	}
	return check.Result{}, false
}

// missingBypass は no_proxy にホスト内の宛先が無ければ WARN の行を返す。
func (c proxyCheck) missingBypass(in check.Input) (check.Result, bool) {
	if in.Env("https_proxy") == "" && in.Env("HTTPS_PROXY") == "" {
		return check.Result{}, false
	}
	noProxy := in.Env("no_proxy") + "," + in.Env("NO_PROXY")

	var missing []string
	for _, want := range localBypass {
		if !hasBypass(noProxy, want) {
			missing = append(missing, want)
		}
	}
	if len(missing) == 0 {
		return check.Result{}, false
	}
	return check.Of(c, check.Result{
		Status:  check.Warn,
		Summary: "no_proxy に " + strings.Join(missing, " / ") + " が無い",
		Detail:  "no_proxy = " + strings.Trim(noProxy, ",") + " にホスト内の宛先が含まれていません。",
		Impact:  "runner 自身やホスト内のサービスへの通信までプロキシへ流れ、内部通信だけが失敗します。",
		Remedy:  "no_proxy / NO_PROXY に " + strings.Join(localBypass, ",") + " を加えてください。",
	}), true
}

// readProxyVars は設定されている環境変数だけを名前付きで返す。
func readProxyVars(in check.Input) map[string]string {
	out := make(map[string]string, len(proxyVars)*2)
	for _, v := range proxyVars {
		for _, name := range []string{v.lower, v.upper} {
			if val := in.Env(name); val != "" {
				out[name] = val
			}
		}
	}
	return out
}

// proxyConfigured はプロキシが 1 つでも設定されているかを返す。
//
// 到達性の判定（reachCheck）が重みを変えるのに使う。プロキシ配下では直接の
// TCP が塞がれているのが正常なこともあるためである。
func proxyConfigured(in check.Input) bool {
	for _, v := range proxyVars {
		if v.lower == "no_proxy" {
			continue // 除外リストが有るだけではプロキシ配下とは言えない。
		}
		if in.Env(v.lower) != "" || in.Env(v.upper) != "" {
			return true
		}
	}
	return false
}

// describe は画面に出す設定値の一覧を返す。値は必ず display() を通す。
func describe(set map[string]string) []string {
	out := make([]string, 0, len(set))
	for _, name := range sortedNames(set) {
		out = append(out, name+"="+display(name, set[name]))
	}
	return out
}

// display は環境変数 name の値のうち、画面に出してよい表記を返す。
//
// no_proxy / NO_PROXY はホスト名の並びであって URL ではなく、資格情報を含まない。
// これまで url.Parse がホストを取り出せないことに頼って素通ししていたが、redact()
// が伏せるようになった以上、伏せると宛先の一覧という判断材料が丸ごと消えてしまう。
// URL として扱う変数だけを redact() へ通す。
func display(name, value string) string {
	if isNoProxy(name) {
		return value
	}
	return redact(value)
}

// isNoProxy は変数名が no_proxy 系かを返す。
func isNoProxy(name string) bool {
	return name == "no_proxy" || name == "NO_PROXY"
}

// sortedNames は proxyVars の宣言順に、設定されている名前だけを返す。
//
// map の走査順に任せると、同じ設定でも実行のたびに Detail の並びが変わる。
func sortedNames(set map[string]string) []string {
	out := make([]string, 0, len(set))
	for _, v := range proxyVars {
		for _, name := range []string{v.lower, v.upper} {
			if _, ok := set[name]; ok {
				out = append(out, name)
			}
		}
	}
	return out
}

// maskedValue は値を安全に出せないときの固定表記。
//
// 「設定はされているが中身は見せられない」ことが伝わるよう、空文字ではなく
// 目に見える印を出す。
const maskedValue = "(表示できない値)"

// redact は URL の userinfo を伏せた表記を返す。
//
// プロキシの認証情報は http://user:pass@host の形で環境変数に入る。診断の画面は
// スクリーンショットで共有されることがあるので、値をそのまま出さない。
//
// パースできない値やホストを取り出せない値は生のまま返さない。url.Parse が
// 失敗する値（例: パスワードに % を含む）や、スキームを書き忘れて
// user:pass@host:3128 が Opaque になった値は userinfo の位置を特定できず、
// 素通しするとパスワードごと画面へ写るためである。
func redact(v string) string {
	u, err := url.Parse(v)
	if err != nil || u.Host == "" {
		return maskedValue
	}
	return u.Redacted()
}

// hasBypass は no_proxy の並びに want が含まれるかを返す。
func hasBypass(noProxy, want string) bool {
	for e := range strings.SplitSeq(noProxy, ",") {
		if strings.TrimSpace(e) == want {
			return true
		}
	}
	return false
}
