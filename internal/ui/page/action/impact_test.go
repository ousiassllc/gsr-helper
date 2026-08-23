package action

import (
	"testing"
)

// 確認ダイアログを経る操作が影響の文言を持つことを固定する。
//
// 停止（x）と再起動（R）の Impact が空だと、dialog.Confirm の「空のブロックは
// 見出しごと落とす」規則で確認ダイアログの影響ブロックがまるごと消え、対象が
// ジョブ実行中でない限り影響行が 1 行も出なくなる。functional.md の確認フロー図が
// 必須段としている「対象と影響の提示」を満たさない状態である（Issue #5）。

// 停止・強制停止・再起動・削除は影響の文言を持ち、安全な操作は持たない。
//
// 文言は screens.md の**詳細画面（`enter`）のモック**の括弧内に合わせる。同じ節の
// 「Runners タブの操作」の表にも影響の列があるが、そちらは「安全」「—」まで含む
// 記述的な要約で表示文字列ではない（`X` は表では `⚠ 実行中ジョブを中断`）。停止と
// 再起動に ⚠ を付けないのは、断定（中断されます）と可能性の差を記号で示すためで、
// この差もここで固定する。
func TestMetaImpact(t *testing.T) {
	want := map[ID]string{
		Start:   "",
		Stop:    impactAffectsJob,
		Kill:    impactKill,
		Drain:   "",
		Restart: impactAffectsJob,
		Enable:  "",
		Delete:  impactDelete,
	}
	for id, w := range want {
		if got, _, _ := meta(id); got != w {
			t.Errorf("meta(%s) の影響 = %q, want %q", id, got, w)
		}
	}

	// モックの文言そのもの。screens.md を書き換えずに実装だけが動くのを防ぐ。
	if impactAffectsJob != "実行中ジョブに影響する可能性" {
		t.Errorf("停止・再起動の影響 = %q, screens.md の詳細画面のモックと食い違っている", impactAffectsJob)
	}
}

// 組み上がった操作の定義にも影響が載る（詳細画面の操作リストと確認ダイアログの出どころ）。
//
// meta ではなく Set.List() を引くのは、newDef が meta の戻りを Def へ写し損ねても
// 検出できるようにするためである。
func TestConfirmedActionsHaveImpact(t *testing.T) {
	for _, d := range testActions().List() {
		switch d.ID {
		case Stop, Kill, Restart:
			if d.Impact == "" {
				t.Errorf("%s（%s）の影響が空である（確認ダイアログに影響行が出ない）", d.ID, d.Key)
			}
		case Unknown, Start, Drain, Enable, Add, Delete, Update, Edit, Logs:
		}
	}
}

// 破壊性（区切り線の下に置くか）は影響の有無とは別に決まる。
//
// 再起動に影響の文言を足したときに Destructive まで真にすると、詳細画面の操作
// リストで再起動が区切り線の下へ移り、screens.md の詳細画面のモックと食い違う。
func TestMetaDestructive(t *testing.T) {
	want := map[ID]bool{
		Start: false, Stop: true, Kill: true,
		Drain: false, Restart: false, Enable: false, Delete: true,
	}
	for id, w := range want {
		if _, got, _ := meta(id); got != w {
			t.Errorf("meta(%s) の破壊性 = %v, want %v", id, got, w)
		}
	}
}
