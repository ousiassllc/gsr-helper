package page

import (
	"slices"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
)

// スコープ不足（admin:org）の判定はここに無い。トークンの保有スコープが必要で
// appconfig.Caps にその情報が無く、gh.TokenScopes も未実装であるためである
// （screens.md の「無効な操作の表示」の 5 行目は、gh パッケージができてから
// Allow に足す）。

// action は Allow の検証に使う操作を返す。実装済みかどうかは呼び出し側が決める。
func action(k string, supported bool) Action {
	return Action{Key: k, Desc: k, Impact: "", Destructive: false, Supported: supported}
}

// 操作の一覧は keymap の並び（安全な操作が先）をそのまま使う。
func TestActionsFollowKeymapOrder(t *testing.T) {
	keys := testKeys().Runner
	acts := Actions(keys)
	if len(acts) != len(keys.Detail()) {
		t.Fatalf("操作の件数 = %d, want %d", len(acts), len(keys.Detail()))
	}

	for i, b := range keys.Detail() {
		if got := acts[i].Key; got != b.Keys()[0] {
			t.Errorf("%d 番目のキー = %q, want %q", i, got, b.Keys()[0])
		}
		if acts[i].Desc != b.Help().Desc {
			t.Errorf("キー %q の説明 = %q, want %q", acts[i].Key, acts[i].Desc, b.Help().Desc)
		}
	}
}

// 詳細画面の操作リストは screens.md の詳細画面の並びそのものである。
//
// 期待値を keymap から計算せずここに書き写すのは、並びと集合の両方を仕様側に
// 固定するためである。n（追加）は対象となる runner を持たない操作なので詳細には
// 出さない。E（切替）はフッタに出せないため、この操作リストが利用者の辿れる経路
// になる（screens.md の Runners タブの操作）。
func TestActionsMatchSpecList(t *testing.T) {
	want := []string{"l", "s", "d", "R", "E", "e", "u", "x", "X", "D"}

	got := make([]string, 0, len(want))
	for _, a := range Actions(testKeys().Runner) {
		got = append(got, a.Key)
	}
	if !slices.Equal(got, want) {
		t.Errorf("詳細画面の操作 = %v, want %v", got, want)
	}
}

// フッタ 1 行目は screens.md の共通レイアウトの並びと短い表記そのものである。
//
// 幅 80 に 9 個 + ?:ヘルプ が収まる表記でなければ設計原則 1 を満たせないため、
// 文言も含めて仕様側に固定する。
func TestHintsMatchSpecFooter(t *testing.T) {
	want := []atom.Hint{
		{Key: "s", Desc: "開始"}, {Key: "x", Desc: "停止"}, {Key: "X", Desc: "強制"},
		{Key: "d", Desc: "ドレイン"}, {Key: "D", Desc: "削除"}, {Key: "n", Desc: "追加"},
		{Key: "u", Desc: "更新"}, {Key: "e", Desc: "設定"}, {Key: "l", Desc: "ログ"},
	}

	hints := Hints(sampleRunner(), fullCaps(), testKeys().Runner)
	if len(hints) != len(want) {
		t.Fatalf("ヒントの件数 = %d, want %d", len(hints), len(want))
	}
	for i, w := range want {
		if hints[i].Key != w.Key || hints[i].Desc != w.Desc {
			t.Errorf("%d 番目のヒント = %q/%q, want %q/%q",
				i, hints[i].Key, hints[i].Desc, w.Key, w.Desc)
		}
	}
}

// 破壊的な操作は停止・強制停止・削除の 3 つで、いずれも安全な操作より後にある。
func TestActionsMarkDestructiveLast(t *testing.T) {
	acts := Actions(testKeys().Runner)
	want := map[string]bool{"x": true, "X": true, "D": true}

	first := len(acts)
	for i, a := range acts {
		if a.Destructive != want[a.Key] {
			t.Errorf("キー %q の Destructive = %v, want %v", a.Key, a.Destructive, want[a.Key])
		}
		if a.Destructive && i < first {
			first = i
		}
		if !a.Destructive && i > first {
			t.Errorf("安全な操作 %q が区切り線より後にある", a.Key)
		}
	}
}

// 操作の実装は後続の Issue が担うため、この版では実装済みの操作が無い。
func TestActionsAreAllUnsupported(t *testing.T) {
	for _, a := range Actions(testKeys().Runner) {
		if a.Supported {
			t.Errorf("キー %q が実装済みになっている（この版では未対応のはず）", a.Key)
		}
	}
}

// screens.md「無効な操作の表示」の 5 状況で、対象キーに期待どおりの理由が返る。
func TestAllowReasons(t *testing.T) {
	tests := map[string]struct {
		caps   appconfig.Caps
		runner runner.Runner
		keys   []string
		want   string
	}{
		"非 root": {
			caps: capsWithout(func(c *appconfig.Caps) { c.Root = false }), runner: sampleRunner(),
			keys: []string{"s", "x", "X", "R", "D", "n", "u"}, want: reasonRoot,
		},
		"systemd が無い": {
			caps: capsWithout(func(c *appconfig.Caps) { c.Systemd = false }), runner: sampleRunner(),
			keys: []string{"s", "x", "X", "R", "E", "d"}, want: reasonSystemd,
		},
		"run.sh 直起動": {
			caps: fullCaps(), runner: standaloneRunner(),
			keys: []string{"s", "x", "R"}, want: reasonStandalone,
		},
		"gh 未認証": {
			caps: capsWithout(func(c *appconfig.Caps) { c.GitHubToken = false }), runner: sampleRunner(),
			keys: []string{"n", "D", "u"}, want: reasonToken,
		},
		"ジョブ実行中": {
			caps: fullCaps(), runner: busyRunner(),
			keys: []string{"D"}, want: reasonBusy,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			for _, k := range tt.keys {
				// 実装済みの操作でも能力不足の理由が優先される。
				ok, reason := Allow(action(k, true), tt.runner, tt.caps)
				if ok {
					t.Errorf("キー %q が許可されている", k)
				}
				if reason != tt.want {
					t.Errorf("キー %q の理由 = %q, want %q", k, reason, tt.want)
				}
			}
		})
	}
}

// capsWithout は 1 つの能力だけを欠いた Caps を返す。
func capsWithout(drop func(*appconfig.Caps)) appconfig.Caps {
	c := fullCaps()
	drop(&c)
	return c
}

// 能力の問題を実装状況で隠さないため、判定順は「root → systemd → 管理外 →
// 認証 → ジョブ実行中 → 未対応」である。
func TestAllowPrecedence(t *testing.T) {
	noRootNoSystemd := fullCaps()
	noRootNoSystemd.Root, noRootNoSystemd.Systemd = false, false

	noSystemd := capsWithout(func(c *appconfig.Caps) { c.Systemd = false })
	noToken := capsWithout(func(c *appconfig.Caps) { c.GitHubToken = false })

	tests := map[string]struct {
		caps   appconfig.Caps
		runner runner.Runner
		key    string
		want   string
	}{
		"非 root が systemd 不在より優先": {noRootNoSystemd, sampleRunner(), "x", reasonRoot},
		"systemd 不在が管理外より優先":      {noSystemd, standaloneRunner(), "x", reasonSystemd},
		"認証がジョブ実行中より優先":           {noToken, busyRunner(), "D", reasonToken},
		"ジョブ実行中が未対応より優先":          {fullCaps(), busyRunner(), "D", reasonBusy},
		"能力が足りていれば未対応の理由になる":      {fullCaps(), sampleRunner(), "x", reasonUnsupported},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// 未対応の判定が最後に来ることを見るため、実装済みとして渡す
			// （最後のケースだけは未実装として渡す）。
			supported := tt.want != reasonUnsupported
			ok, reason := Allow(action(tt.key, supported), tt.runner, tt.caps)
			if ok {
				t.Fatalf("キー %q が許可されている", tt.key)
			}
			if reason != tt.want {
				t.Errorf("理由 = %q, want %q", reason, tt.want)
			}
		})
	}
}

// 能力が足りている実装済みの操作は許可され、理由を持たない。
func TestAllowPermits(t *testing.T) {
	for _, k := range []string{"s", "x", "X", "d", "R", "E", "e", "u", "n", "D", "l"} {
		ok, reason := Allow(action(k, true), sampleRunner(), fullCaps())
		if !ok {
			t.Errorf("キー %q が許可されない（理由: %s）", k, reason)
		}
		if reason != "" {
			t.Errorf("キー %q の理由 = %q, want 空", k, reason)
		}
	}
}

// フッタのキーヒントは可否と理由を持つ。
func TestHintsCarryReasons(t *testing.T) {
	for _, h := range Hints(sampleRunner(), fullCaps(), testKeys().Runner) {
		if h.Enabled {
			t.Errorf("キー %q が有効になっている（この版では未対応のはず）", h.Key)
		}
		if h.Reason == "" {
			t.Errorf("キー %q に理由が無い", h.Key)
		}
	}
}

// Allowed は Allow と同じ判定を返す（Jobs タブが判定を作り直さないための入口）。
func TestAllowedMatchesAllow(t *testing.T) {
	caps := capsWithout(func(c *appconfig.Caps) { c.Root = false })
	for _, k := range []string{"s", "x", "X", "d", "R", "E", "e", "u", "n", "D", "l"} {
		wantOK, wantReason := Allow(action(k, false), sampleRunner(), caps)
		gotOK, gotReason := Allowed(k, sampleRunner(), caps)
		if gotOK != wantOK || gotReason != wantReason {
			t.Errorf("Allowed(%q) = %v/%q, want %v/%q", k, gotOK, gotReason, wantOK, wantReason)
		}
	}
}

// 操作リストは区切り線を 1 本だけ持ち、最初の破壊的な操作の前に置く。
func TestChoicesDivider(t *testing.T) {
	items := Choices(sampleRunner(), fullCaps(), testKeys().Runner)
	acts := Actions(testKeys().Runner)
	if len(items) != len(acts) {
		t.Fatalf("項目の件数 = %d, want %d", len(items), len(acts))
	}

	dividers := 0
	for i, c := range items {
		if !c.DividerBefore {
			continue
		}
		dividers++
		if !acts[i].Destructive {
			t.Errorf("区切り線が破壊的でない操作 %q の前にある", c.Key)
		}
		if i > 0 && acts[i-1].Destructive {
			t.Errorf("区切り線が最初の破壊的な操作より後（%q の前）にある", c.Key)
		}
	}
	if dividers != 1 {
		t.Errorf("区切り線の本数 = %d, want 1", dividers)
	}
}

// 影響の併記は強制停止と削除だけが持つ（screens.md の詳細画面）。
func TestChoicesImpact(t *testing.T) {
	withImpact := []string{}
	for _, c := range Choices(sampleRunner(), fullCaps(), testKeys().Runner) {
		if c.Impact != "" {
			withImpact = append(withImpact, c.Key)
		}
	}
	slices.Sort(withImpact)
	if want := []string{"D", "X"}; !slices.Equal(withImpact, want) {
		t.Errorf("影響を併記する操作 = %v, want %v", withImpact, want)
	}
}
