package disk

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/disk/cleanview"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// **削除に至る道が「選択 → ドライラン → 確認 → 実行」の 1 本しか無いことを固定する。**
// このファイルが緑である限り、確認を経ない破壊的経路は存在しない
// （security.md「確認を経ない破壊的経路を作らない」）。
//
// 対象に docker の内訳を使うのは、削除が実際に起きたかを exec.Fake の記録
// （docker system prune -f が発行されたか）で確実に判定できるためである。ファイルの
// 削除は痕跡が消えたファイルにしか残らず、「走らなかった」ことの証明に向かない。

// selected は docker の行を 1 件選んだ状態のタブと Executor を返す。
func selected(t *testing.T) (Model, *exec.Fake) {
	t.Helper()

	st, fake := dockerState()
	m := activate(t, newModel(t, st))
	m, _ = send(t, m, press("space"))
	if len(m.tbl.Checked()) != 1 {
		t.Fatal("対象を選択できていない（前提が崩れている）")
	}
	return m, fake
}

// c は確認モーダルを開くだけで、削除は走らない（設計原則 3）。
func TestCleanKeyOnlyOpensConfirm(t *testing.T) {
	m, fake := selected(t)

	m, c := send(t, m, press("c"))
	if !c.Modal {
		t.Fatal("c で確認モーダルが開かない")
	}
	if _, ran := pruneCall(fake); ran {
		t.Error("確認する前に削除が走っている")
	}
	if m.clean != nil {
		t.Error("確認する前に実行が始まっている")
	}

	// 何が起きるかは確認画面が示す（実行するコマンドと解放見込み）。
	body := m.View().Content
	for _, want := range []string{"prune", "解放見込み", "prune -f では削除されません", "復元できません"} {
		if !strings.Contains(body, want) {
			t.Errorf("確認画面に %q が出ていない:\n%s", want, body)
		}
	}
}

// キャンセル（n / esc / enter）では削除が走らない。
//
// **enter もキャンセルである**（設計原則 5「Enter の連打では進まない」）。一覧から
// 続けて enter を打っている流れのまま破壊的操作へ到達しないようにするためで、
// esc は「1 枚閉じる」経路を通るが結果は同じでなければならない。
func TestConfirmCancelKeysDoNotDelete(t *testing.T) {
	for _, k := range []string{"n", "esc", "enter"} {
		t.Run(k, func(t *testing.T) {
			m, fake := selected(t)
			m, _ = send(t, m, press("c"))

			m, _ = send(t, m, press(k))
			if m.overlay.Active() {
				t.Errorf("%s で確認モーダルが閉じない", k)
			}
			if _, ran := pruneCall(fake); ran {
				t.Errorf("%s を押したのに削除が走った", k)
			}
			if m.clean != nil {
				t.Errorf("%s を押したのに実行が始まっている", k)
			}
			// 取り消した計画を抱えたままにしない（esc は決定を発行しない経路を通る）。
			if m.plan.Docker || len(m.plan.Paths) > 0 {
				t.Errorf("%s で閉じたのに計画が残っている（%+v）", k, m.plan)
			}
		})
	}
}

// y を押して初めて削除が走り、監査の action が付いた 1 本だけが発行される。
func TestConfirmYesRunsCleanup(t *testing.T) {
	m, fake := selected(t)
	m, _ = send(t, m, press("c"))

	m, _ = send(t, m, press("y"))

	call, ran := pruneCall(fake)
	if !ran {
		t.Fatalf("y を押しても docker system prune が発行されない（%v）", fake.Calls())
	}
	if got := call.String(); got != "docker system prune -f" {
		t.Errorf("発行されたコマンド = %q, want %q", got, "docker system prune -f")
	}
	// 記録の起点は Executor の手前 1 か所（internal/disk）であり、page は監査を扱わない。
	if call.Options.Action != "disk.clean" {
		t.Errorf("監査の action = %q, want %q", call.Options.Action, "disk.clean")
	}

	// 確認は閉じ、代わりに進捗表示が前面に出る（Issue #75）。
	if !m.overlay.Active() {
		t.Error("実行後に進捗表示が出ていない")
	}
	if body := m.View().Content; !strings.Contains(body, "クリーンアップ中") {
		t.Errorf("進捗表示の見出しが出ていない:\n%s", body)
	}
	if !strings.Contains(m.status(), "クリーンアップ完了") {
		t.Errorf("結果が報告されていない（status = %q）", m.status())
	}
	// 使用量が変わったので選択を解いて集計をやり直す。
	if n := len(m.tbl.Checked()); n != 0 {
		t.Errorf("実行後も選択が残っている（%d 件）", n)
	}
}

// 確認モーダルを開いている間、背後へキーが流れない。
//
// 差し戻すと**削除の確認中に打った q でアプリが終わる**。閉じ込められるのは
// モーダルを持っている page だけである（page.GlobalKeyMsg の doc）。
func TestKeysAreTrappedWhileConfirmIsOpen(t *testing.T) {
	m, fake := selected(t)
	m, _ = send(t, m, press("c"))

	next, cmd := m.Update(press("q"))
	if _, _, bubbled := pagetest.ScanKey(cmd); bubbled {
		t.Error("確認中に打った q が親へ差し戻されている")
	}
	if !as(t, next).overlay.Active() {
		t.Error("関係の無いキーで確認モーダルが閉じている")
	}
	if _, ran := pruneCall(fake); ran {
		t.Error("確認中の打鍵で削除が走った")
	}
}

// 選択 0 件で c を押しても確認モーダルは開かない。理由は状態行に出す。
func TestCleanKeyWithoutSelectionDoesNothing(t *testing.T) {
	st, fake := dockerState()
	m := activate(t, newModel(t, st))

	_, c := send(t, m, press("c"))
	if c.Modal {
		t.Error("対象が無いのに確認モーダルが開いた")
	}
	if c.Status != noticeNoTarget {
		t.Errorf("状態行 = %q, want %q", c.Status, noticeNoTarget)
	}
	if _, ran := pruneCall(fake); ran {
		t.Error("対象が無いのに削除が走った")
	}
	// フッタでも押せないことと理由を示す（screens.md の無効な操作の表示）。
	if !hintDisabled(c.Footer, "c", noticeNoTarget) {
		t.Errorf("フッタの c が無効になっていない（%v）", c.Footer)
	}
}

// 検証を通らない対象では確認へ進まない（ドライランで落ちる）。
//
// PlanClean は 1 件でも検証を通らなければ計画そのものを作らない。一部だけ通った
// 計画で確認を出すと、y を押した後に「画面に出ていない対象が残った」ことに気付けない。
func TestInvalidTargetDoesNotReachConfirm(t *testing.T) {
	st, fake := baseState()
	m := update(t, newModel(t, st), page.ActivateMsg{})

	// 集計対象の体裁は持つが、実在しない（＝ ValidatePath を通らない）パスの行。
	bad := fakeUsage("build01-1 / _work/gone", 100)
	m = update(t, m, usageMsg{gen: m.gen, usage: bad, ok: true})
	m, _ = send(t, m, press("j"))
	m, _ = send(t, m, press("space"))
	if len(m.tbl.Checked()) != 1 {
		t.Fatal("対象を選択できていない（前提が崩れている）")
	}

	_, c := send(t, m, press("c"))
	if c.Modal {
		t.Error("検証を通らない対象で確認モーダルが開いた")
	}
	if !strings.Contains(c.Status, "削除できません") {
		t.Errorf("状態行に理由が出ていない（status = %q）", c.Status)
	}
	if len(fake.Calls()) != 0 {
		t.Errorf("外部コマンドが発行されている（%v）", fake.Calls())
	}
}

// hintDisabled はフッタに、指定したキーが無効かつ理由付きで出ているかを返す。
func hintDisabled(footer []atom.Hint, key, reason string) bool {
	for _, h := range footer {
		if h.Key == key {
			return !h.Enabled && h.Reason == reason
		}
	}
	return false
}

// 選べない行の理由は disk.Target まで運ぶ（FR-31）。
//
// 表が選択を阻むだけにすると保護が表示層の約束で終わり、Target を直接組む呼び出しが
// 1 つ増えた時点で黙って外れる（disk.Target.Protected の doc）。
func TestCleanviewTargetsCarriesProtectedReason(t *testing.T) {
	u := fakeUsage("build01-1 / _work/bar", 100)
	u.Removable, u.Reason = false, busyReasonPrefix
	if got := cleanview.Targets(checkedUsage([]row{{usage: u}}))[0].Protected; got != busyReasonPrefix {
		t.Errorf("Protected = %q, want %q", got, busyReasonPrefix)
	}
}

// 承認を待つ間にジョブが始まった runner の _work は削除しない（FR-31）。
//
// Target.Protected は disk.Scan がジョブの有無を見た「その時点」の値なので、確認
// ダイアログを開いている間にジョブが始まると、保護されていない計画のまま y に到達する。
// 確認ダイアログに時間制限は無いため窓は任意に広がる。**この経路が塞がっていないと、
// 実行中ジョブの _work が root 権限で消える。**
func TestJobStartedWhileConfirmingAbortsCleanup(t *testing.T) {
	busy := busyRunner(t)
	idle := busy
	idle.Workers = nil

	st, fake := baseState()
	st.Result.Runners = []runner.Runner{idle}
	m := activate(t, newModel(t, st))

	// カーソルを _work の行（docker の SKIP 行の次）へ移してから選ぶ。
	m, _ = send(t, m, press("j"))
	m, _ = send(t, m, press("space"))
	if len(m.tbl.Checked()) == 0 {
		t.Fatal("集計した _work を選択できていない（前提が崩れている）")
	}
	m, _ = send(t, m, press("c"))
	if !m.overlay.Active() {
		t.Fatal("確認ダイアログが開いていない（前提が崩れている）")
	}

	// 承認を待つ間にジョブが始まる。共有状態は 3 秒ごとに配られる。
	busyState := st
	busyState.Result.Runners = []runner.Runner{busy}
	m, _ = send(t, m, busyState)

	m, _ = send(t, m, press("y"))

	kept := filepath.Join(busy.Dir, "_work", "bar", "a.txt")
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("ジョブ実行中になった _work が削除された（%s: %v）", kept, err)
	}
	if len(fake.Calls()) != 0 {
		t.Errorf("中止したのに外部コマンドが発行された（%v）", fake.Calls())
	}
	if !strings.Contains(m.status(), "ジョブが開始したため中止") {
		t.Errorf("中止の理由が報告されていない（status = %q）", m.status())
	}
}
