package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/config/edit"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/configmodal"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 対象を決めるまでは runner の一覧を出し、自身の設定も選べること（FR-41〜FR-42）。
func TestPickerListsRunnersAndSelf(t *testing.T) {
	t.Parallel()

	m := newPage(t, pagetest.SampleRunner())

	if !contains(m, "build01-1") {
		t.Errorf("runner が一覧に無い:\n%s", view(m))
	}
	if !contains(m, "gsr-helper 自身の設定") {
		t.Errorf("自身の設定が一覧に無い:\n%s", view(m))
	}
}

// 対象を選ぶと設定項目の一覧に変わること。画面仕様の Config タブのモックに
// 並ぶ項目がすべて出る。
func TestChoosingRunnerShowsItems(t *testing.T) {
	t.Parallel()

	r := pagetest.SampleRunner()
	m := newPage(t, r)
	m, _ = send(t, m, organism.ChosenMsg{ID: r.Name(), Key: ""})

	if !contains(m, r.Name()+" の設定") {
		t.Fatalf("見出しが出ていない:\n%s", view(m))
	}
	for _, want := range []string{
		".env", ".path", "systemd drop-in", "ラベル", "runner group",
		"名前・work dir・ephemeral", "複製",
	} {
		if !contains(m, want) {
			t.Errorf("項目 %q が一覧に無い:\n%s", want, view(m))
		}
	}
}

// 再登録が要る項目は変更できず、理由が備考に出ること（screens.md のモック）。
func TestReregisterRowShowsWarning(t *testing.T) {
	t.Parallel()

	r := pagetest.SampleRunner()
	m := newPage(t, r)
	m, _ = send(t, m, organism.ChosenMsg{ID: r.Name(), Key: ""})

	if !contains(m, "再登録が必要") {
		t.Errorf("再登録の注意書きが出ていない:\n%s", view(m))
	}
}

// Runners タブからの依頼で対象が決まること（e で設定を編集）。
func TestEditConfigMsgSetsTarget(t *testing.T) {
	t.Parallel()

	r := pagetest.SampleRunner()
	m := newPage(t)
	m, _ = send(t, m, page.EditConfigMsg{Runner: r})

	if !contains(m, r.Name()+" の設定") {
		t.Errorf("対象が設定されていない:\n%s", view(m))
	}
}

// esc は対象の選択へ戻り、対象が無ければ Runners タブへ戻ること。
func TestBackReturnsToPickerThenRunners(t *testing.T) {
	t.Parallel()

	r := pagetest.SampleRunner()
	m := newPage(t, r)
	m, _ = send(t, m, organism.ChosenMsg{ID: r.Name(), Key: ""})

	m, _ = send(t, m, pagetest.Press("esc"))
	if !contains(m, "gsr-helper 自身の設定") {
		t.Fatalf("対象の選択へ戻っていない:\n%s", view(m))
	}

	_, cmd := send(t, m, pagetest.Press("esc"))
	var opened bool
	for _, msg := range pagetest.Msgs(cmd) {
		if open, ok := msg.(page.OpenTabMsg); ok && open.Title == page.TabRunners {
			opened = true
		}
	}
	if !opened {
		t.Error("Runners タブへ戻る要求が出ていない")
	}
}

// .env を選ぶとフォームが開き、入力中であることが状態行に出ること。
func TestEnterOpensForm(t *testing.T) {
	t.Parallel()

	r := withTempDir(t, "build01-1", "PATH=/usr/bin\n")
	m := newPage(t, r)
	m, _ = send(t, m, page.EditConfigMsg{Runner: r})

	m, cmd := send(t, m, pagetest.Press("enter"))
	m = pagetest.Advance(m, cmd, 4).(Model)

	if !m.formShown {
		t.Fatal("フォームが開いていない")
	}
	if got := chromeOf(t, m.chrome()).Input; got != "フォーム" {
		t.Errorf("状態行の入力中 = %q, want フォーム", got)
	}
}

// 承認するまで書き込まないこと（FR-37）。取り消したらファイルは元のまま。
func TestCancelledApprovalDoesNotWrite(t *testing.T) {
	t.Parallel()

	const before = "PATH=/usr/bin\n"
	r := withTempDir(t, "build01-1", before)
	m := newPage(t, r)
	m, _ = send(t, m, page.EditConfigMsg{Runner: r})

	m = openEnvForm(t, m)
	m, _ = send(t, m, page.ResultMsg{Kind: configmodal.DiffKind, Msg: dialog.DecidedMsg{Confirmed: false}})

	got, err := readFile(filepath.Join(r.Dir, ".env"))
	if err != nil {
		t.Fatalf(".env の読み込みに失敗: %v", err)
	}
	if got != before {
		t.Errorf("取り消したのに書き換えられた: %q", got)
	}
	if !strings.Contains(m.status(), "取りやめ") {
		t.Errorf("状態行 = %q, want 取りやめた旨", m.status())
	}
}

// 承認すると退避してから書き込み、そのあと反映方法を選ばせること（FR-38 / FR-39）。
func TestApprovedWriteBacksUpAndAsksApplyMethod(t *testing.T) {
	t.Parallel()

	r := withTempDir(t, "build01-1", "PATH=/usr/bin\n")
	m := newPage(t, r)
	m, _ = send(t, m, page.EditConfigMsg{Runner: r})
	m = openEnvForm(t, m)

	// 差分の承認。書き込みは Cmd として返る。
	m, cmd := send(t, m, page.ResultMsg{Kind: configmodal.DiffKind, Msg: dialog.DecidedMsg{Confirmed: true}})
	done, err := doneOf(cmd)
	if err != nil {
		t.Fatalf("書き込みの結果を取れない: %v", err)
	}
	if done.err != nil {
		t.Fatalf("書き込みでエラー: %v", done.err)
	}

	got, err := readFile(filepath.Join(r.Dir, ".env"))
	if err != nil {
		t.Fatalf(".env の読み込みに失敗: %v", err)
	}
	if !strings.Contains(got, "/opt/bin") {
		t.Errorf("書き込み後 = %q, want 新しい値", got)
	}
	if bak, berr := readFile(filepath.Join(r.Dir, ".env.bak")); berr != nil || bak != "PATH=/usr/bin\n" {
		t.Errorf("退避 = %q, %v, want 元の内容", bak, berr)
	}

	// 反映方法の選択が開き、既定がドレイン再起動である（FR-39）。
	m, cmd = send(t, m, done)
	m = pagetest.Advance(m, cmd, 4).(Model)
	if !m.overlay.Active() {
		t.Fatal("反映方法の選択が開いていない")
	}
	if !contains(m, "ドレイン再起動") {
		t.Errorf("反映方法の選択肢が出ていない:\n%s", view(m))
	}
	if !contains(m, "強制再起動") || !contains(m, "反映しない") {
		t.Errorf("3 つの選択肢が揃っていない:\n%s", view(m))
	}
}

// ラベルの変更は GitHub 側で即時に反映されるので、反映方法を尋ねないこと。
func TestLabelChangeSkipsApplyMethod(t *testing.T) {
	t.Parallel()

	r := withTempDir(t, "build01-1", "")
	m := newPage(t, r)
	m, _ = send(t, m, page.EditConfigMsg{Runner: r})

	m.pending = edit.BuildLabels(r.Name(), []string{"old"}, []string{"gpu"})
	m, cmd := send(t, m, doneMsg{text: "書き込みました", err: nil})
	m = pagetest.Advance(m, cmd, 3).(Model)

	if m.overlay.Active() {
		t.Errorf("再起動が要らないのに反映方法を尋ねている:\n%s", view(m))
	}
}

// 設定ファイルが無い状態の起動では初回ウィザードが開くこと（FR-41）。
func TestFirstRunOpensWizard(t *testing.T) {
	t.Parallel()

	st := pagetest.State(80, 24)
	m := New(tabIndex, st)

	st.Config = page.ConfigDeps{Conf: st.Config.Conf, Path: "/tmp/config.yaml", FirstRun: true}
	next, cmd := m.Update(st)
	m = pagetest.Advance(next, cmd, 5).(Model)

	if !m.self {
		t.Fatal("初回設定ウィザードが開いていない")
	}
	if !contains(m, "初回設定") {
		t.Errorf("ウィザードの見出しが出ていない:\n%s", view(m))
	}
}

// 設定ファイルがあれば初回ウィザードは開かないこと。
func TestExistingConfigDoesNotOpenWizard(t *testing.T) {
	t.Parallel()

	st := pagetest.State(80, 24)
	m := New(tabIndex, st)

	st.Config = page.ConfigDeps{Conf: st.Config.Conf, Path: "/tmp/config.yaml", FirstRun: false}
	next, cmd := m.Update(st)
	m = pagetest.Advance(next, cmd, 5).(Model)

	if m.self {
		t.Error("設定ファイルがあるのにウィザードが開いた")
	}
}
