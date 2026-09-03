package configmodal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/config/edit"
)

// 入力欄と検証の配線の回帰テスト（Issue #182）。
//
// **見るのは「どの欄がどの検証に繋がるか」だけである。** 検証そのものの規則は
// internal/config/edit 側の単体テスト（validate_test.go）が持つ。
//
// 判定はすべて振る舞い——欄へ値を入れて検証エラーになるか——で行い、**関数
// ポインタの比較はしない**。比較にすると、配線を変えずに検証の中身を差し替えた
// ときに落ちなくなる。
//
// **改行は欄からは入らない。** huh の入力欄は bubbles/textinput を使い、値の
// 流し込み（SetValue）と貼り付けの両方で改行を空白へ置き換える。そのため
// edit.ValidateLine が唯一弾く値（改行）はこの経路では作れず、ValidateLine に
// 繋がる欄（`.path` / MemoryMax）は「弾かれないこと」でしか縛れない。代わりに
// **他の検証が揃って弾く値を通すこと**を見て、より強い検証へ繋ぎ替わっていない
// ことを縛る（lenientProbe）。改行を弾く側は上記の単体テストが持つ。

// hookProbe は edit.ValidateLine が通し config.ValidateHookPath が弾く値。
// 絶対パスでないため、hook の検証に繋がる欄だけが弾く。
const hookProbe = "relative/hook.sh"

// lenientProbe は edit.ValidateLine 以外の Validate* がすべて弾く値。絶対パスで
// なく（hook / 走査ルート / 監査ログが弾く）、数値でもなく（間隔 / 閾値が弾く）、
// ラベルに使えない文字を含む（ラベルが弾く）。
const lenientProbe = "relative!path"

// 欄の位置。fields が種類ごとに返す並びの添字である。
const (
	idxPath      = 0 // KindPath の `.path`
	idxLabels    = 0 // KindLabels のラベル
	idxMemoryMax = 1 // KindDropIn の MemoryMax（0 は Restart の選択欄）
	idxScanRoots = 0 // KindSelf の追加の走査ルート
)

// fieldErr は idx 番目の欄を確定させ、検証の結果を返す。
//
// huh の Field は Blur で必ず検証を走らせ、結果を Error に残す。**値は欄を
// 組み立てる前に edit.Values へ入れておくこと**——Value(&s) は組み立てた時点の
// 値を入力欄へ写すので、組み立てた後に書き換えても欄には届かない。
func fieldErr(t *testing.T, v *edit.Values, idx int) error {
	t.Helper()

	all := fields(v)
	if idx >= len(all) {
		t.Fatalf("欄が %d 個しかない（idx=%d）", len(all), idx)
	}

	f := all[idx]
	f.Blur()

	return f.Error()
}

// envValues は .env の i 番目の欄に in を入れた受け皿を返す。
func envValues(i int, in string) *edit.Values {
	v := edit.NewValues()
	v.Env[i] = in

	return v
}

// hook のキーだけが強い検証（edit.ValidateHook）に繋がること。
//
// **この配線（form.go の `if k.Hook`）が逆になっても消えても、検証そのものの
// テストは緑のままである。** hook の値は runner がジョブごとにシェルで実行する
// ため、他のキーより強く見ると docs/architecture/security.md が定めている。
func TestOnlyHookEnvFieldsUseHookValidation(t *testing.T) {
	t.Parallel()

	hooks := 0
	for i, k := range edit.EnvKeys {
		err := fieldErr(t, envValues(i, hookProbe), i)

		if k.Hook {
			hooks++
			if err == nil {
				t.Errorf("%s は hook だが %q を通した: edit.ValidateHook に繋がっていない", k.Key, hookProbe)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s は hook でないのに %q を弾いた: %v", k.Key, hookProbe, err)
		}
	}

	// hook のキーが 1 つも無いと、上の検査は 1 度も本題を見ずに緑になる。
	if hooks == 0 {
		t.Fatal("edit.EnvKeys に hook のキーが無い")
	}
}

// hook の欄が正しい値を通すこと（何を入れても弾く欄になっていないこと）。
func TestHookEnvFieldsAcceptExecutablePath(t *testing.T) {
	t.Parallel()

	sh := filepath.Join(t.TempDir(), "hook.sh")
	if err := os.WriteFile(sh, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("hook の作成に失敗: %v", err)
	}
	missing := sh + ".none"

	for i, k := range edit.EnvKeys {
		if !k.Hook {
			continue
		}
		// 空は「設定しない」、実行できる絶対パスは設定できる値である。
		for _, in := range []string{"", sh} {
			if err := fieldErr(t, envValues(i, in), i); err != nil {
				t.Errorf("%s に %q を入れて弾かれた: %v", k.Key, in, err)
			}
		}
		// 絶対パスでも実在しなければ弾くこと。通す側と hookProbe（相対パス）だけでは
		// 「絶対パスかどうか」しか見ておらず、パスを整えるだけで stat しない検証
		// （edit.ValidateRoots）が代役に立ててしまう。存在と実行可否まで見る検証へ
		// 繋がっていることをここで縛る（docs/architecture/security.md）。
		if err := fieldErr(t, envValues(i, missing), i); err == nil {
			t.Errorf("%s に実在しない絶対パス %q を入れて通した", k.Key, missing)
		}
	}
}

// ラベルの欄が edit.ValidateLabelInput に繋がること（FR-36）。
//
// 通す側（カンマ区切りのラベル）は hook / 走査ルート / 間隔 / 閾値のどれもが
// 弾く値なので、弾く側と併せて配線が 1 本に定まる。
func TestLabelsFieldUsesLabelValidation(t *testing.T) {
	t.Parallel()

	v := edit.NewValues()
	v.Kind = edit.KindLabels
	v.Labels = "gpu!"

	if err := fieldErr(t, v, idxLabels); err == nil {
		t.Errorf("ラベルに使えない文字を含む %q を通した", v.Labels)
	}

	ok := edit.NewValues()
	ok.Kind = edit.KindLabels
	ok.Labels = "gpu, build"

	if err := fieldErr(t, ok, idxLabels); err != nil {
		t.Errorf("カンマ区切りのラベル %q を弾いた: %v", ok.Labels, err)
	}
}

// 追加の走査ルートの欄が edit.ValidateRoots に繋がること。
//
// エラー文の `scan_roots` で他の検証と見分ける（hook は `job hook`、監査ログは
// `監査ログ` と言う）。
func TestScanRootsFieldUsesRootsValidation(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"relative", "/opt/../etc"} {
		v := selfValues(in)

		err := fieldErr(t, v, idxScanRoots)
		if err == nil {
			t.Errorf("走査ルートに %q を通した", in)
			continue
		}
		if !strings.Contains(err.Error(), "scan_roots") {
			t.Errorf("走査ルート %q のエラー = %v, want scan_roots を含む（別の検証に繋がっている）", in, err)
		}
	}

	if err := fieldErr(t, selfValues("/opt/runners, /srv/runners"), idxScanRoots); err != nil {
		t.Errorf("絶対パスの走査ルートを弾いた: %v", err)
	}
}

// selfValues は自身の設定の受け皿を返す。走査ルート以外は通る値で埋める。
func selfValues(roots string) *edit.Values {
	v := edit.NewValues()
	v.Kind = edit.KindSelf
	v.Self = edit.SelfValues{
		ScanRoots: roots, Refresh: "60", Warn: "80", Critical: "90",
		AuditLog: "/var/log/gsr-helper/audit.log",
	}

	return v
}

// `.path` と MemoryMax が 1 行の検証（edit.ValidateLine）のままであること。
//
// 上の doc のとおり改行は欄から入れられないので、**より強い検証へ繋ぎ替わって
// いない**ことを、他の Validate* が揃って弾く値を通すことで見る。
func TestLineFieldsAreNotWiredToStricterValidation(t *testing.T) {
	t.Parallel()

	path := edit.NewValues()
	path.Kind = edit.KindPath
	path.Path = lenientProbe

	if err := fieldErr(t, path, idxPath); err != nil {
		t.Errorf(".path が %q を弾いた: %v（1 行の検証より強いものに繋がっている）", lenientProbe, err)
	}

	mem := edit.NewValues()
	mem.Kind = edit.KindDropIn
	mem.MemoryMax = lenientProbe

	if err := fieldErr(t, mem, idxMemoryMax); err != nil {
		t.Errorf("MemoryMax が %q を弾いた: %v（1 行の検証より強いものに繋がっている）", lenientProbe, err)
	}

	// 本来の値も通ること。
	size := edit.NewValues()
	size.Kind = edit.KindDropIn
	size.MemoryMax = "4G"

	if err := fieldErr(t, size, idxMemoryMax); err != nil {
		t.Errorf("MemoryMax が %q を弾いた: %v", size.MemoryMax, err)
	}
}
