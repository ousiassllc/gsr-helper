package action

import (
	"slices"
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// スコープ不足（admin:org）の判定はここに無い。トークンの保有スコープが必要で
// appconfig.Caps にその情報が無く、gh.TokenScopes も未実装であるためである
// （screens.md の「無効な操作の表示」の 5 行目は、gh パッケージができてから
// Allow に足す）。

// action は Allow の検証に使う操作を返す。識別子は既定のキー定義から引く（判定はキー
// ではなく識別子で行うため、キーだけを渡すテストも本体と同じ対応表を通す）。
func action(k string, supported bool) Def {
	return Def{ID: testActions().byKey[k], Key: k, Desc: k, Supported: supported}
}

// testActions は既定のキー定義から組んだ操作の表を返す。
func testActions() Set { return NewSet(testKeys().Runner) }

// 操作の一覧はキー・説明・識別子のすべてを keymap から受け取る。
//
// 集合と並び（screens.md の詳細画面のキー表）は keymap 側の TestDetailMatchesSpec が
// 仕様に固定している。ここでは page がそれに従うことと、識別子がキー定義から引かれて
// いること（キーを差し替えても操作の同一性が保たれること）を見る。
//
// Supported はこの版ではすべて false である。操作の実装は後続の Issue が担うため、
// 「押せるが何も起きない」経路を作らない。
func TestActionsFollowKeymap(t *testing.T) {
	keys := testKeys().Runner
	ids := testActions().byKey
	acts := testActions().List()
	if len(acts) != len(keys.Detail()) {
		t.Fatalf("操作の件数 = %d, want %d", len(acts), len(keys.Detail()))
	}

	for i, b := range keys.Detail() {
		k := b.Keys()[0]
		switch {
		case acts[i].Key != k:
			t.Errorf("%d 番目のキー = %q, want %q", i, acts[i].Key, k)
		case acts[i].Desc != b.Help().Desc:
			t.Errorf("キー %q の説明 = %q, want %q", k, acts[i].Desc, b.Help().Desc)
		case acts[i].ID != ids[k]:
			t.Errorf("キー %q の識別子 = %d, want %d", k, acts[i].ID, ids[k])
		case acts[i].Supported:
			t.Errorf("キー %q が実装済みになっている（この版では未対応のはず）", k)
		}
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

	hints := testActions().Hints(sampleRunner(), fullCaps(), testKeys().Runner)
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
	acts := testActions().List()
	want := map[ID]bool{Stop: true, Kill: true, Delete: true}

	first := len(acts)
	for i, a := range acts {
		if a.Destructive != want[a.ID] {
			t.Errorf("キー %q の Destructive = %v, want %v", a.Key, a.Destructive, want[a.ID])
		}
		if a.Destructive && i < first {
			first = i
		}
		if !a.Destructive && i > first {
			t.Errorf("安全な操作 %q が区切り線より後にある", a.Key)
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
		"能力が足りていれば未対応の理由になる":      {fullCaps(), sampleRunner(), "x", page.ReasonUnsupported},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// 未対応の判定が最後に来ることを見るため、実装済みとして渡す
			// （最後のケースだけは未実装として渡す）。
			supported := tt.want != page.ReasonUnsupported
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

// 能力が足りている実装済みの操作は許可され、Allowed も Allow と同じ判定を返す。
//
// Allowed は Jobs タブがキーだけから判定を引く入口である。理由まで一致することを見る
// ため、能力を欠いた Caps（非 root）も通す。
func TestAllowPermitsAndAllowedAgrees(t *testing.T) {
	keys := testKeys().Runner
	noRoot := capsWithout(func(c *appconfig.Caps) { c.Root = false })
	for _, k := range []string{"s", "x", "X", "d", "R", "E", "e", "u", "n", "D", "l"} {
		if ok, reason := Allow(action(k, true), sampleRunner(), fullCaps()); !ok || reason != "" {
			t.Errorf("キー %q = %v/%q, want true/空", k, ok, reason)
		}
		wantOK, wantReason := Allow(action(k, false), sampleRunner(), noRoot)
		gotOK, gotReason := NewSet(keys).Allowed(k, sampleRunner(), noRoot)
		if gotOK != wantOK || gotReason != wantReason {
			t.Errorf("Allowed(%q) = %v/%q, want %v/%q", k, gotOK, gotReason, wantOK, wantReason)
		}
	}
}

// フッタのキーヒントは可否と理由を持つ。
func TestHintsCarryReasons(t *testing.T) {
	for _, h := range testActions().Hints(sampleRunner(), fullCaps(), testKeys().Runner) {
		if h.Enabled {
			t.Errorf("キー %q が有効になっている（この版では未対応のはず）", h.Key)
		}
		if h.Reason == "" {
			t.Errorf("キー %q に理由が無い", h.Key)
		}
	}
}

// 操作リストは区切り線を 1 本だけ持ち、最初の破壊的な操作の前に置く。
func TestChoicesDivider(t *testing.T) {
	items := testActions().Choices(sampleRunner(), fullCaps())
	acts := testActions().List()
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
	for _, c := range testActions().Choices(sampleRunner(), fullCaps()) {
		if c.Impact != "" {
			withImpact = append(withImpact, c.Key)
		}
	}
	slices.Sort(withImpact)
	if want := []string{"D", "X"}; !slices.Equal(withImpact, want) {
		t.Errorf("影響を併記する操作 = %v, want %v", withImpact, want)
	}
}

// 管理状態が判定できないときは、systemd 経路に依存する操作だけを専用の理由で塞ぐ。
//
// 汎用の未対応（page.ReasonUnsupported）に落ちると、利用者は塞がれた原因を知れない。
// 強制停止とドレインは worker のプロセスに作用するので残す。
func TestAllowBlocksManagedUnknown(t *testing.T) {
	r := standaloneRunner()
	r.Managed = runner.ManagedUnavailable

	for _, k := range []string{"s", "x", "R", "E"} {
		ok, reason := Allow(action(k, true), r, fullCaps())
		if ok || reason != reasonManagedUnknown {
			t.Errorf("キー %q = %v/%q, want false/%q", k, ok, reason, reasonManagedUnknown)
		}
	}
	for _, k := range []string{"X", "d", "l"} {
		if _, reason := Allow(action(k, true), r, fullCaps()); reason == reasonManagedUnknown {
			t.Errorf("キー %q が管理状態不明で塞がれている", k)
		}
	}
}

// 可否の判定は keymap のキーに追従する（キーストロークのリテラルに結び付かない）。
//
// 停止のキーを差し替えたら、run.sh 直起動で塞ぐ対象も新しいキーへ移る。判定表がキーの
// リテラルだと、差し替えでこの判定が黙って消える（コンパイルエラーにならない）。
func TestAllowFollowsReboundKey(t *testing.T) {
	keys := testKeys().Runner
	keys.Stop = key.NewBinding(key.WithKeys("Q"), key.WithHelp("Q", "停止"))
	caps := fullCaps()

	if _, reason := NewSet(keys).Allowed("Q", standaloneRunner(), caps); reason != reasonStandalone {
		t.Errorf("差し替え後の停止キーの理由 = %q, want %q", reason, reasonStandalone)
	}
	if _, reason := NewSet(keys).Allowed("x", standaloneRunner(), caps); reason == reasonStandalone {
		t.Error("差し替え前の x に停止の判定が残っている")
	}
}
