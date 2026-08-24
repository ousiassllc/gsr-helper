package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/configmodal"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 自身の設定（FR-41 / FR-42）の回帰テスト。
//
// runner の設定編集（regress_test.go）と分けているのは、**編集対象が違う**ためで
// ある。こちらが書くのは gsr-helper 自身の設定ファイル 1 つで、差分の基準も
// 「保存済みの値か、ファイルが無いか」で決まる。1 ファイル 300 行の上限
// （docs/ui/atomic-design.md）に収める際の切れ目もここになる（Issue #108）。

// 自身の設定を保存したあと開き直すと、保存した値が初期値になること（FR-42）。
func TestSelfConfigReopensWithSavedValues(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	st := pagetest.State(80, 24)
	st.Config = page.ConfigDeps{Conf: st.Config.Conf, Path: dir + "/config.yaml", FirstRun: false}
	m := New(tabIndex, st)
	next, _ := m.Update(st)
	m = next.(Model)

	m, _ = send(t, m, organism.ChosenMsg{ID: selfID, Key: ""})
	m.vals.Self.Refresh = "42"
	m, _ = send(t, m, page.ResultMsg{Kind: configmodal.FormKind, Msg: dialog.FormDoneMsg{Form: nil}})

	m, cmd := send(t, m, page.ResultMsg{Kind: configmodal.DiffKind, Msg: dialog.DecidedMsg{Confirmed: true}})
	done, err := doneOf(cmd)
	if err != nil {
		t.Fatalf("書き込みの結果を取れない: %v", err)
	}
	if done.err != nil {
		t.Fatalf("設定の書き込みに失敗: %v", done.err)
	}
	m, _ = send(t, m, done)

	m, _ = send(t, m, organism.ChosenMsg{ID: selfID, Key: ""})
	if m.vals.Self.Refresh != "42" {
		t.Errorf("開き直した初期値 = %q, want 42（保存した値）", m.vals.Self.Refresh)
	}
}

// 書き込めた設定は親へ返り、書き込めなければ返らないこと（Issue #128）。
//
// 返らないと親の設定は起動時のままで、Disk タブの警告閾値超過と doctor の
// リソース診断が再起動するまで古い閾値で判定し続ける。失敗したときに返すのは
// もっと悪く、ファイルの中身と画面の判定が食い違う。
func TestSelfConfigSaveReturnsNewConfigToParent(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		path string
		want bool
	}{
		{name: "書き込めたら返す", path: t.TempDir() + "/config.yaml", want: true},
		// 書き込み先を既存ファイルのある位置の下へ置き、Save を失敗させる。
		{name: "書き込めなければ返さない", path: fileAsDir(t) + "/config.yaml", want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			st := pagetest.State(80, 24)
			st.Config = page.ConfigDeps{Conf: appconfig.Default(), Path: tt.path, FirstRun: false}
			m := New(tabIndex, st)
			next, _ := m.Update(st)
			m = next.(Model)

			m, _ = send(t, m, organism.ChosenMsg{ID: selfID, Key: ""})
			m.vals.Self.Warn, m.vals.Self.Critical = "55", "77"
			m, _ = send(t, m, page.ResultMsg{Kind: configmodal.FormKind, Msg: dialog.FormDoneMsg{Form: nil}})

			m, cmd := send(t, m, page.ResultMsg{Kind: configmodal.DiffKind, Msg: dialog.DecidedMsg{Confirmed: true}})
			done, err := doneOf(cmd)
			if err != nil {
				t.Fatalf("書き込みの結果を取れない: %v", err)
			}
			if got := done.err != nil; got == tt.want {
				t.Fatalf("書き込みの成否 = %v（err = %v）, この筋の前提と食い違う", !got, done.err)
			}

			_, cmd = send(t, m, done)
			saved, err := savedOf(cmd)
			if !tt.want {
				// 返らないことを見る筋なので、待ち時間切れでは代用できない（Issue #140）。
				if !errors.Is(err, pagetest.ErrNotFound) {
					t.Fatalf("親へ返さない筋の結果 = %v, want %v", err, pagetest.ErrNotFound)
				}

				return
			}
			if err != nil {
				t.Fatalf("親へ返していない: %v", err)
			}
			if w := (appconfig.DiskThresholds{Warn: 55, Critical: 77}); saved.Conf.DiskThresholds != w {
				t.Errorf("親へ返した閾値 = %+v, want %+v", saved.Conf.DiskThresholds, w)
			}
		})
	}
}

// 初回ウィザードを値を変えずに確定しても設定ファイルが作られること（FR-41）。
//
// ウィザードの初期値は既定値そのものなので、素通しの確定は保存済みの内容と
// 一致する。差分なしで書き込みを飛ばすとファイルが作られず、次回起動も
// FirstRun のままでウィザードが毎回出続ける。
func TestFirstRunWizardWritesWithoutEdits(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/config.yaml"
	st := pagetest.State(80, 24)
	m := New(tabIndex, st)

	// 実環境の初回起動と同じ基準にする。appconfig.Load はファイルが無ければ
	// Default() を返し、ウィザードの初期値もそこから作られるため、素通しの
	// 確定は往復して同じ値になる（pagetest の Conf はゼロ値で往復しない）。
	st.Config = page.ConfigDeps{Conf: appconfig.Default(), Path: path, FirstRun: true}
	next, cmd := m.Update(st)
	m = pagetest.Advance(next, cmd, 5).(Model)
	if !m.self {
		t.Fatal("初回設定ウィザードが開いていない")
	}

	// 値を触らずに確定する。
	m, _ = send(t, m, page.ResultMsg{Kind: configmodal.FormKind, Msg: dialog.FormDoneMsg{Form: nil}})

	m, cmd = send(t, m, page.ResultMsg{Kind: configmodal.DiffKind, Msg: dialog.DecidedMsg{Confirmed: true}})
	done, err := doneOf(cmd)
	if err != nil {
		t.Fatalf("書き込みの結果を取れない（%v）: %s", err, view(m))
	}
	if done.err != nil {
		t.Fatalf("設定の書き込みに失敗: %v", done.err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("設定ファイルが作られていない: %v", err)
	}
}

// 再起動が要ることを画面で伝えるのは、走っているログの出力先とずれたときだけ（Issue #132）。
//
// 監査ログの出力先は起動時に開いたファイルハンドル（ui.Options.Audit）として
// 配られており、走行中に開き直すと差し替えの最中に走っている書き込みを取りこぼす。
// 反映しないと決めた以上、黙って効かないままにはしない——利用者から見れば
// 「設定に書いたパスが効かない」という Issue #128 と同じ体験になる。
//
// **比べる相手は起動時の値である。** 前回の保存値と比べると、一度変えてから元へ
// 戻した保存で「再起動後に切り替わります」と出る——再起動しても何も変わらないのに
// 再起動を促す、事実として誤った案内になる。
func TestSelfConfigTellsRestartOnlyWhenAuditLogDiffersFromStartup(t *testing.T) {
	t.Parallel()

	const startup = "/var/log/gsr-helper/audit.jsonl"
	const moved = "/tmp/gsr-helper-audit.jsonl"

	for _, tt := range []struct {
		name string
		// audits は保存を繰り返す順に並べた audit_log の値（空なら触らない）。
		audits []string
		want   bool
	}{
		{name: "出力先を変えたら伝える", audits: []string{moved}, want: true},
		{name: "変えていなければ伝えない", audits: []string{""}, want: false},
		{name: "起動時の値へ戻したら伝えない", audits: []string{moved, startup}, want: false},
		{name: "変えた後で別の項目だけ保存しても伝える", audits: []string{moved, ""}, want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := newSelfConfigTab(t, startup)
			status := ""
			for i, audit := range tt.audits {
				// 監査ログ以外にも必ず差分を作る。差分が無いと書き込み自体が飛ぶ。
				m, status = saveSelf(t, m, strconv.Itoa(40+i), audit)
			}

			if got := strings.Contains(status, auditRestartNote); got != tt.want {
				t.Errorf("状態行 = %q, 再起動の案内を含むか = %v, want %v", status, got, tt.want)
			}
			if !tt.want && status != "設定を書き込みました" {
				t.Errorf("状態行 = %q, want %q（書き込みの報告まで消えている）", status, "設定を書き込みました")
			}
		})
	}
}

// 書き込みに失敗したときは再起動の案内を出さないこと（Issue #132）。
//
// 出力先が切り替わるのは設定ファイルに書けた場合だけである。書けていないのに
// 「再起動後に切り替わります」と出すと、再起動して元のままなのを見た利用者が
// 二度目の失敗まで原因に気付けない。
func TestSelfConfigDoesNotTellRestartWhenSaveFailed(t *testing.T) {
	t.Parallel()

	m := newSelfConfigTab(t, "/var/log/gsr-helper/audit.jsonl")
	m.st.Config.Path = fileAsDir(t) + "/config.yaml"

	m, status := saveSelf(t, m, "42", "/tmp/gsr-helper-audit.jsonl")
	if strings.Contains(status, auditRestartNote) {
		t.Errorf("書き込みに失敗したのに再起動の案内が出ている: %q", status)
	}
	if !strings.HasPrefix(status, "失敗: ") {
		t.Errorf("状態行 = %q, want 失敗の報告", status)
	}
	_ = m
}

// newSelfConfigTab は起動時の audit_log を控えた Config タブを返す。
//
// **New に Config 入りの StateMsg を渡さない。** 本番でタブを組み立てる
// tabset.New が渡す StateMsg は Config を持たず、設定は最初の共有状態で届く。
// 同じ順序を踏まないと setState の「最初の StateMsg」の判定が働かない。
func newSelfConfigTab(t *testing.T, audit string) Model {
	t.Helper()

	conf := appconfig.Default()
	conf.AuditLog = audit

	st := pagetest.State(80, 24)
	m := New(tabIndex, st)
	st.Config = page.ConfigDeps{Conf: conf, Path: t.TempDir() + "/config.yaml", FirstRun: false}
	next, _ := m.Update(st)

	return next.(Model)
}

// saveSelf は自身の設定を 1 度保存し、保存後の状態行を返す。
//
// refresh は必ず変える（差分が無いと書き込み自体が飛ぶ）。audit が空なら
// 監査ログの欄は触らない。
func saveSelf(t *testing.T, m Model, refresh, audit string) (Model, string) {
	t.Helper()

	m, _ = send(t, m, organism.ChosenMsg{ID: selfID, Key: ""})
	m.vals.Self.Refresh = refresh
	if audit != "" {
		m.vals.Self.AuditLog = audit
	}
	m, _ = send(t, m, page.ResultMsg{Kind: configmodal.FormKind, Msg: dialog.FormDoneMsg{Form: nil}})

	m, cmd := send(t, m, page.ResultMsg{Kind: configmodal.DiffKind, Msg: dialog.DecidedMsg{Confirmed: true}})
	done, err := doneOf(cmd)
	if errors.Is(err, pagetest.ErrNotFound) {
		t.Fatal("書き込みの Cmd が出ていない（差分なしで飛ばされた）")
	}
	if err != nil {
		t.Fatalf("書き込みの結果を取れない: %v", err)
	}
	m, _ = send(t, m, done)

	return m, m.status()
}
