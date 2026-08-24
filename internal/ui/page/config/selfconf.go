package config

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/config"
	"github.com/ousiassllc/gsr-helper/internal/config/edit"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/configmodal"
)

// 自身の設定（FR-41〜FR-42）のフォーム。初回起動時（設定ファイルが無い）は
// 自動で開き、以後も対象の一覧から選び直せる。編集できるのは要件が挙げる
// 4 つ——走査ルート・ディスク閾値・ポーリング間隔・監査ログ出力先——である。
//
// 値の変換・正規化・検証は edit.SelfValues が持つ（tea にも huh にも依らない）。
// 入力欄の組み立ては page/configmodal が持つ（Issue #104）。ここに残るのは、
// 現在値の決め方と、承認から書き込みまでの画面の流れだけである。

// selfTitle はフォームの見出し。初回かどうかで変える。
const (
	selfTitleFirst = "初回設定（gsr-helper 自身の設定）"
	selfTitleEdit  = "gsr-helper 自身の設定"
)

// selfConf は自身の設定の現在値を返す。
//
// **保存済みの値があればそちらを優先する。** page.ConfigDeps.Conf は ui.New が
// 起動時に決めた写しで、書き込んでも次のフレームまでは更新されない（親が配り直す
// StateMsg が届くのはそのときであり、書き込んだ直後に開き直すフォームには
// 間に合わない）。優先しないと、保存した直後に開き直したフォームが古い値を出し、
// その古い値を基準に差分を組んでしまう（FR-42 の黙ったデータ喪失）。
//
// なお**書き込めた設定は親へも返す**（Issue #128。発行は rememberSelf）。返さないと
// 設定ファイルだけが新しくなり、ディスク閾値の判定は再起動まで古い値のままになる。
// **一度書き込んだ後は、以後このセッションでは自分の写しを真とする**——confSet は
// 解除されないので、親が配り直す st.Config.Conf はもう読まない。写しは書き込めた
// 値そのもので、親が採用する値と同一なので食い違いは起きない。
func (m Model) selfConf() appconfig.Config {
	if m.confSet {
		return m.conf
	}
	return m.st.Config.Conf
}

// firstRunUnsaved は設定ファイルがまだ無い状態か（初回起動で未保存）を返す。
//
// FirstRun は起動時の判定なので、このセッションで一度書き込んだ後（confSet）は
// もうファイルがある。差分の基準をどこに置くかは saveSelf が使う。
func (m Model) firstRunUnsaved() bool {
	return m.st.Config.FirstRun && !m.confSet
}

// openSelfForm は自身の設定のフォームを開く（FR-41 / FR-42）。
func (m *Model) openSelfForm() tea.Cmd {
	m.self = true
	m.formShown = true
	m.vals.Kind = edit.KindSelf
	m.vals.Self = edit.NewSelfValues(m.selfConf())

	title := selfTitleEdit
	if m.st.Config.FirstRun {
		title = selfTitleFirst
	}

	return configmodal.OpenForm(&m.overlay, title, m.vals, m.st)
}

// saveSelf は自身の設定の差分を出して承認を求める（FR-41 / FR-42）。
//
// 差分の承認はここでも経る。破壊的な書き込みであることは runner 側の設定と
// 変わらないためである（FR-37）。バックアップは appconfig.Save が一時ファイル +
// rename で置き換えるため、書き損じで元の設定が壊れることはない。
//
// Apply は appconfig の正規化まで通す。**欄をまたぐ検証（警告 < 危険）が効くのは
// ここだけである**——huh の Validate は 1 欄しか見えないので、警告 90 / 危険 80 の
// ような組み合わせはフォームでは弾けない。通してしまうと差分の承認まで進み、
// 書き込みの直前で初めて失敗する。
func (m *Model) saveSelf() tea.Cmd {
	base := m.selfConf()

	next, err := m.vals.Self.Apply(base)
	if err != nil {
		m.notice = err.Error()
		m.overlay.Close()

		return nil
	}

	// **初回起動では差分の基準を「ファイルが無い」に置く。** ウィザードの初期値は
	// 既定値そのものなので、base と比べると値を触らない確定が常に
	// 「変更はありません」になり、設定ファイルが作られない。ファイルが無ければ
	// 次回起動も FirstRun のままで、ウィザードが毎回出続ける（FR-41 が求める
	// 「初回だけ」が成立しない）。空を基準にすれば差分は全行追加になり、
	// 承認の画面もファイルの新規作成として正しく読める。
	before, after := edit.RenderConfig(base), edit.RenderConfig(next)
	if m.firstRunUnsaved() {
		before = ""
	}
	if before == after {
		m.notice = "変更はありません"
		m.overlay.Close()

		return nil
	}

	m.pendingSelf, m.pendingSet = next, true
	m.overlay.Close()

	return configmodal.OpenDiff(&m.overlay, dialog.DiffApprovalInput{
		Path:   m.st.Config.Path,
		Diff:   edit.SplitDiff(config.Diff(before, after)),
		Backup: "",
	})
}

// commitSelf は承認された自身の設定を書き込む。
//
// 書き込んだ設定は savingSelf / savedSelf に控え、成功した doneMsg を受けた
// 時点で selfConf の答えに昇格させる（onDone）。失敗した場合に昇格させないのは、
// 書けなかった値を次の編集の基準にすると差分が現実と食い違うためである。
func (m *Model) commitSelf() tea.Cmd {
	cfg, path := m.pendingSelf, m.st.Config.Path
	m.pendingSelf = appconfig.Config{}
	m.savingSelf, m.savedSelf = true, cfg
	m.busy = true

	return page.Do(m.tab, func() tea.Msg {
		if err := appconfig.Save(cfg, path); err != nil {
			return doneMsg{text: "", err: err}
		}
		return doneMsg{text: "設定を書き込みました", err: nil}
	})
}
