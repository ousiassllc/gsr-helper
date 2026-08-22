// Package chrome は本体以外の領域（ヘッダ・タブ行・状態行・フッタ）の中身を組み立てる。
//
// 領域の配分は template.Frame が行い、ここでは中身の文字列だけを作る。
//
// 親 Model（ui.App）の型も bubbletea も知らない純粋関数だけを置く。1 フレーム分の
// 入力（View）を渡せば親 Model を組み立てずに検証でき、親側には「App の値を View へ
// 写す」1 メソッドだけが残る（Issue #35 の ui 直下の分割）。
//
// molecule 階層の一部なので、**ドメインの型は受け取らない**（atomic-design.md の
// 依存の規則）。`appconfig.Caps` や `runner.Result` ではなく、ヘッダと状態行が実際に
// 使う真偽値と件数だけを View に持たせる。写し替えは親側の `ui.App.chromeView` が
// 行う。
package chrome

import (
	"strconv"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// View は 1 フレーム分の入力。親 Model が持つ値を表示用の値へ落としたものだけを
// 受け取る。
type View struct {
	// Host はヘッダに出すホスト名。
	Host string
	// Root / Systemd / HasToken はヘッダのバッジ（能力判定の結果）。
	// appconfig.Caps をそのまま受け取らないのは、molecule 以下がドメインの型を
	// 扱わないためである。
	Root     bool
	Systemd  bool
	HasToken bool
	// Tabs はタブの並び。選択中かどうかは各要素の Active が持つ。
	Tabs []molecule.TabView
	// OrphanUnits と Warnings は状態行の左側に出す件数。
	OrphanUnits int
	Warnings    int
	// Err は状態行の左側に出す直近のエラー。error は標準ライブラリの型であり
	// ドメインの型ではないので、文言を組み立て直さずそのまま受け取る。
	Err error
	// Notice は親が出す一時的な案内（無効なタブの理由）。次の打鍵で消える。
	Notice string
	// Status は page が ChromeMsg で報告した状態行の右側。
	Status string
	// Hints は page が ChromeMsg で報告したフッタのキーヒント（可否と理由込み）。
	Hints []atom.Hint
	// Width は端末の幅、Styles は解決済みの配色。
	Width  int
	Styles token.Styles
}

// Header は能力判定の結果を出すヘッダ行を返す。
//
// gh の認証ユーザー名は載せない。Caps はトークンそのものを持たない設計であり、
// ユーザー名の取得は GitHub API を使う機能の担当である（molecule 側は認証済みで
// ユーザー名が無い状態を「認証済み」と描く）。
func Header(v View) string {
	return molecule.CapsBar(molecule.CapsView{
		Host:       v.Host,
		Root:       v.Root,
		Systemd:    v.Systemd,
		GitHubUser: "",
		HasToken:   v.HasToken,
	}, v.Width, v.Styles)
}

// TabBar はタブ行を返す。
func TabBar(v View) string {
	return molecule.TabBar(v.Tabs, v.Width, v.Styles)
}

// Status は状態行を返す。
//
// 左側は親が持つ検出結果から作る（孤児ユニット件数・警告件数・直近のエラー）。
// 右側は page が報告した文（選択件数・入力中）を置く。同じ検出結果からの件数を
// タブごとに作らせないため、この分担にしている。
func Status(v View) string {
	return atom.Justify(counts(v), right(v), v.Width)
}

// right は状態行の右側を返す。
//
// 無効なタブの理由は page の報告より優先する。番号キーを押した打鍵では page へ
// キーが渡らず ChromeMsg も更新されないため、理由を出せるのはここだけである。
func right(v View) string {
	if v.Notice != "" {
		return v.Styles.Muted.Render(v.Notice)
	}
	return v.Status
}

// counts は検出結果から状態行の左側を組み立てる。
func counts(v View) string {
	var parts []string
	if v.OrphanUnits > 0 {
		parts = append(parts, v.Styles.Warn.Render(
			token.IconWarn+" 孤児ユニット "+strconv.Itoa(v.OrphanUnits)+" 件"))
	}
	if v.Warnings > 0 {
		parts = append(parts, v.Styles.Warn.Render("警告 "+strconv.Itoa(v.Warnings)+" 件"))
	}
	if v.Err != nil {
		// エラーで画面遷移を巻き戻さず、状態行に出すだけにする
		// （architecture/overview.md のエラーハンドリング）。
		parts = append(parts, v.Styles.Fail.Render(token.IconFailed+" "+v.Err.Error()))
	}
	return strings.Join(parts, " / ")
}

// Footer はフッタ 2 行を返す。
//
// キーヒントは page が可否と理由込みで報告したものを使う。?:ヘルプ は
// molecule.KeyBar が必ず付けるため、ここでは足さない。
func Footer(v View) string {
	return molecule.KeyBar(v.Hints, v.Width, v.Styles)
}
