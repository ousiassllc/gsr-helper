package chrome

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule/chromebar"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// specTabs は screens.md の共通レイアウトが定める 7 タブを表示用の値で返す。
//
// 親 Model や page を組み立てないのは、chrome が受け取るのが表示用の値だけだから
// である（ドメインの型を扱わない molecule 階層）。未実装のタブ 3〜7 を Enabled:
// false にして、選択可・選択不可の 2 状態を再現する。
func specTabs() []chromebar.TabView {
	titles := []string{"Runners", "Jobs", "Disk", "Logs", "Doctor", "Config", "Setup"}
	tabs := make([]chromebar.TabView, 0, len(titles))
	for i, title := range titles {
		tabs = append(tabs, chromebar.TabView{
			Key:     strconv.Itoa(i + 1),
			Title:   title,
			Active:  i == 0,
			Enabled: i < 2,
		})
	}
	return tabs
}

// testView は幅 80・色無しの 1 フレーム分の入力を返す。
//
// 期待値を素の文字列で書けるよう色は無効にする（atom / molecule のテストと同じ）。
func testView() View {
	styles := token.NewStyles(true, false)
	return View{
		Host:        "build01",
		Root:        true,
		Systemd:     true,
		HasToken:    true,
		Tabs:        specTabs(),
		OrphanUnits: 0,
		Warnings:    0,
		Err:         nil,
		Notice:      "",
		Status:      "",
		Hints:       nil,
		Width:       80,
		Styles:      styles,
	}
}

// ヘッダはホスト名を出す。
func TestHeaderShowsHost(t *testing.T) {
	if got := Header(testView()); !strings.Contains(got, "build01") {
		t.Errorf("ヘッダにホスト名が無い: %q", got)
	}
}

// タブ行には screens.md の共通レイアウトの 7 タブすべてが幅 80 で収まる。
func TestTabBarShowsEverySpecTabWithin80(t *testing.T) {
	got := TabBar(testView())

	for i, want := range []string{
		"Runners", "Jobs", "Disk", "Logs", "Doctor", "Config", "Setup",
	} {
		key := strconv.Itoa(i + 1)
		if !strings.Contains(got, "["+key+"]"+want) && !strings.Contains(got, "("+key+")"+want) {
			t.Errorf("タブ %q がタブ行に無い: %q", want, got)
		}
	}
	if w := lipgloss.Width(got); w > 80 {
		t.Errorf("タブ行の幅 = %d, want 80 以下（%q）", w, got)
	}
}

// 選択中のタブにはカーソル記号が付き、無効なタブはキーが丸括弧になる。
//
// 3 状態（選択中・選択可・選択不可）を色だけで区別しない（screens.md の設計原則 4）。
// 親が渡した Active を正しく反映していることを、記号の位置で確かめる。
func TestTabBarMarksActiveAndDisabledTabs(t *testing.T) {
	v := testView()
	v.Tabs[0].Active = false
	v.Tabs[1].Active = true

	got := TabBar(v)
	if !strings.Contains(got, token.IconCursor+"[2]Jobs") {
		t.Errorf("選択中のタブにカーソル記号が無い: %q", got)
	}
	if !strings.Contains(got, " [1]Runners") {
		t.Errorf("選択中でない有効タブにカーソル記号が付いている: %q", got)
	}
	if !strings.Contains(got, "(3)Disk") {
		t.Errorf("無効なタブのキーが丸括弧になっていない: %q", got)
	}
}

// 状態行の左側には孤児ユニット件数・警告件数・直近のエラーが並ぶ。
func TestStatusShowsCounts(t *testing.T) {
	v := testView()
	v.OrphanUnits = 1
	v.Warnings = 2
	v.Err = errors.New("検出に失敗しました")

	got := Status(v)
	for _, want := range []string{"孤児ユニット 1 件", "警告 2 件", "検出に失敗しました"} {
		if !strings.Contains(got, want) {
			t.Errorf("状態行に %q が無い: %q", want, got)
		}
	}
}

// 検出結果が空でエラーも無いとき、状態行の左側には何も出ない。
func TestStatusIsEmptyWithoutFindings(t *testing.T) {
	if got := Status(testView()); got != "" {
		t.Errorf("空の状態行 = %q, want 空文字", got)
	}
}

// 無効なタブの案内は page が報告した状態行より優先する。
//
// 番号キーを押した打鍵では page へキーが渡らず ChromeMsg も更新されないため、
// 理由を出せるのは親だけである。
func TestStatusPrefersNoticeOverPageStatus(t *testing.T) {
	v := testView()
	v.Status = "2 件選択中"
	v.Notice = "[3]Disk この版では未対応です"

	got := Status(v)
	if !strings.Contains(got, v.Notice) {
		t.Errorf("状態行に案内が無い: %q", got)
	}
	if strings.Contains(got, v.Status) {
		t.Errorf("案内があるのに page の報告が残っている: %q", got)
	}

	// 案内が消えたら page の報告に戻る。
	v.Notice = ""
	if got := Status(v); !strings.Contains(got, v.Status) {
		t.Errorf("案内が無いときの状態行に page の報告が無い: %q", got)
	}
}

// フッタは page が報告したキーヒントを出し、?:ヘルプ を必ず添える。
func TestFooterShowsHintsAndHelp(t *testing.T) {
	v := testView()
	v.Hints = []atom.Hint{
		{Key: "s", Desc: "開始", Enabled: true, Reason: ""},
		{Key: "x", Desc: "停止", Enabled: false, Reason: "この版では未対応です"},
	}

	got := Footer(v)
	if lines := strings.Split(got, "\n"); len(lines) != 2 {
		t.Fatalf("フッタの行数 = %d, want 2（%q）", len(lines), got)
	}
	for _, want := range []string{"s:開始", "?:ヘルプ", "この版では未対応です"} {
		if !strings.Contains(got, want) {
			t.Errorf("フッタに %q が無い: %q", want, got)
		}
	}
}

// 起動時のジョブ実行の前提チェック（FR-44）の不備はヘッダと状態行の両方に出し、
// Doctor タブへ誘導する。
//
// 件数を出すだけで行き先を示さないと、内訳と対処を出せるのが Doctor タブだけで
// あることが分からず、利用者は 7 枚のタブを順に開くことになる。
//
// **数え方はここでは決めない。** 何を不備と数えるか（SKIP を含めないなど）は
// internal/ui/hostreq が持ち、chrome は受け取った件数を描くだけである。
func TestHostRequirementWarningReachesHeaderAndStatus(t *testing.T) {
	tests := map[string]struct {
		bad  int
		key  string
		warn bool
		hint bool
	}{
		"不備があれば警告して誘導する":      {bad: 2, key: "5", warn: true, hint: true},
		"不備が無ければ警告しない":        {bad: 0, key: "5", warn: false, hint: false},
		"Doctor タブが無ければ誘導しない": {bad: 2, key: "", warn: true, hint: false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			v := testView()
			v.HostReq, v.DoctorKey = tt.bad, tt.key
			status, header := Status(v), Header(v)

			want := "ホスト前提 " + strconv.Itoa(tt.bad) + " 件"
			for label, line := range map[string]string{"状態行": status, "ヘッダ": header} {
				if got := strings.Contains(line, want); got != tt.warn {
					t.Errorf("%sの警告 = %v, want %v（%q）", label, got, tt.warn, line)
				}
			}
			if got := strings.Contains(status, "（"+tt.key+" で詳細）"); got != tt.hint {
				t.Errorf("状態行の誘導 = %v, want %v（%q）", got, tt.hint, status)
			}
		})
	}
}
