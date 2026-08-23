package action

import (
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/svc"
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

// supported はこの版で実装済みの操作を返す。
//
// **一覧をここに書き下す。** meta から引くと被テスト関数で期待値を作ることになり、
// 実装済みの印が丸ごと消えても両辺が一致して落ちない。実装済みなのは internal/svc が
// 担うサービス制御の 6 つ、Logs タブのログを開く操作、そして Setup タブが実装した
// 追加・削除・バージョン更新である。設定編集（e）は後続の Issue が担う。
func supported() map[ID]bool {
	return map[ID]bool{
		Start: true, Stop: true, Kill: true, Drain: true, Restart: true, Enable: true,
		Logs: true, Add: true, Delete: true, Update: true, Edit: true,
	}
}

// 操作の一覧はキー・説明・識別子のすべてを keymap から受け取る。
//
// 集合と並び（screens.md の詳細画面のキー表）は keymap 側の TestDetailMatchesSpec が
// 仕様に固定している。ここでは page がそれに従うことと、識別子がキー定義から引かれて
// いること（キーを差し替えても操作の同一性が保たれること）を見る。
//
// Supported が真なのは実装済みの操作（サービス制御の 6 つ・ログを開く操作・追加・
// 削除・バージョン更新・設定編集）だけである。未実装の操作に真を付けると「押せるが
// 何も起きない」経路ができるため、実装済みの集合を supported() に固定する。
func TestActionsFollowKeymap(t *testing.T) {
	keys := testKeys().Runner
	ids := testActions().byKey
	acts := testActions().List()
	if len(acts) != len(keys.Detail()) {
		t.Fatalf("操作の件数 = %d, want %d", len(acts), len(keys.Detail()))
	}

	want := supported()
	for i, b := range keys.Detail() {
		k := b.Keys()[0]
		switch {
		case acts[i].Key != k:
			t.Errorf("%d 番目のキー = %q, want %q", i, acts[i].Key, k)
		case acts[i].Desc != b.Help().Desc:
			t.Errorf("キー %q の説明 = %q, want %q", k, acts[i].Desc, b.Help().Desc)
		case acts[i].ID != ids[k]:
			t.Errorf("キー %q の識別子 = %d, want %d", k, acts[i].ID, ids[k])
		case acts[i].Supported != want[acts[i].ID]:
			t.Errorf("キー %q の実装状況 = %v, want %v", k, acts[i].Supported, want[acts[i].ID])
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
			keys: []string{"s", "x", "X", "R", "D", "n", "u"}, want: svc.ReasonRoot,
		},
		"systemd が無い": {
			caps: capsWithout(func(c *appconfig.Caps) { c.Systemd = false }), runner: sampleRunner(),
			keys: []string{"s", "x", "X", "R", "E", "d"}, want: svc.ReasonSystemd,
		},
		"run.sh 直起動": {
			caps: fullCaps(), runner: standaloneRunner(),
			keys: []string{"s", "x", "d", "R", "E"}, want: svc.ReasonStandalone,
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
		"非 root が systemd 不在より優先": {noRootNoSystemd, sampleRunner(), "x", svc.ReasonRoot},
		"systemd 不在が管理外より優先":      {noSystemd, standaloneRunner(), "x", svc.ReasonSystemd},
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

// 能力が足りている実装済みの操作は許可され、Allowed はキーから同じ理由を引く。
//
// Allowed は Jobs タブがキーだけから判定を引く入口である。
//
// **期待値は Allow で作らない。** Allowed は Allow の薄いラッパなので、被テスト関数
// 自身で期待値を組むと比較が同語反復になり、どちらが壊れても一致して原理的に落ちない
// （Issue #31）。非 root で塞がれる操作とそうでない操作をここに書き下し、キーから
// 操作への対応（Set.byKey）が壊れたら落ちるようにする。
func TestAllowPermitsAndAllowedAgrees(t *testing.T) {
	// 非 root で塞がる操作は svc.ReasonRoot、実装済みで root を要さない操作
	// （ドレイン停止・enable の切替）は許可され、残りはこの版では未対応の理由になる。
	// 空文字は「許可される」ことを表す。
	want := map[string]string{
		"s": svc.ReasonRoot, "x": svc.ReasonRoot, "X": svc.ReasonRoot, "R": svc.ReasonRoot,
		"u": svc.ReasonRoot, "n": svc.ReasonRoot, "D": svc.ReasonRoot,
		"d": "", "E": "",
		// e（設定を編集）は Config タブが実装した。ラベルの編集のように root を
		// 要さない経路があるため、非 root でも塞がない（screens.md の判定表は
		// 1 段目の root 必須に e を挙げていない）。
		"e": "",
		// l（ログを開く）は実装済みで、管理経路にも権限にも依存しない
		// （screens.md の「無効な操作の表示」）。非 root でも塞がらない。
		"l": "",
	}

	// キー定義が持つ操作を 1 つ足したら、ここも足さないと落ちる（Detail は n を
	// 載せないため List() ではなくキーの表と突き合わせる）。
	set := NewSet(testKeys().Runner)
	if got := len(keyIDs(testKeys().Runner)); got != len(want) {
		t.Fatalf("検証するキーの数 = %d, want %d（キー定義が持つ操作の全件）", len(want), got)
	}

	noRoot := capsWithout(func(c *appconfig.Caps) { c.Root = false })
	for k, wantReason := range want {
		if ok, reason := Allow(action(k, true), sampleRunner(), fullCaps()); !ok || reason != "" {
			t.Errorf("キー %q = %v/%q, want true/空", k, ok, reason)
		}
		ok, reason := set.Allowed(k, sampleRunner(), noRoot)
		if wantOK := wantReason == ""; ok != wantOK || reason != wantReason {
			t.Errorf("Allowed(%q) = %v/%q, want %v/%q", k, ok, reason, wantOK, wantReason)
		}
	}
}

// 管理状態が判定できないときは、systemctl を要する操作を専用の理由で塞ぐ。
//
// 汎用の未対応（page.ReasonUnsupported）に落ちると、利用者は塞がれた原因を知れない。
// 塞ぐ範囲は run.sh 直起動と同じ 5 つで、違うのは理由の文言だけである。強制停止は
// worker のプロセスへ直接シグナルを送るので残す。
func TestAllowBlocksManagedUnknown(t *testing.T) {
	r := standaloneRunner()
	r.Managed = runner.ManagedUnavailable

	for _, k := range []string{"s", "x", "d", "R", "E"} {
		ok, reason := Allow(action(k, true), r, fullCaps())
		if ok || reason != svc.ReasonManagedUnknown {
			t.Errorf("キー %q = %v/%q, want false/%q", k, ok, reason, svc.ReasonManagedUnknown)
		}
	}
	for _, k := range []string{"X", "l"} {
		if _, reason := Allow(action(k, true), r, fullCaps()); reason == svc.ReasonManagedUnknown {
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

	if _, reason := NewSet(keys).Allowed("Q", standaloneRunner(), caps); reason != svc.ReasonStandalone {
		t.Errorf("差し替え後の停止キーの理由 = %q, want %q", reason, svc.ReasonStandalone)
	}
	if _, reason := NewSet(keys).Allowed("x", standaloneRunner(), caps); reason == svc.ReasonStandalone {
		t.Error("差し替え前の x に停止の判定が残っている")
	}
}
