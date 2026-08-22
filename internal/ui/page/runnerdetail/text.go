package runnerdetail

import (
	"strconv"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// 詳細画面の情報部に出す文字列の組み立てを集める。表示のための言い換え（一覧では
// 記号で足りる値を文で出す）はこの階層の関心事であり、ドメイン層の String は
// 一覧向けの短い表記のままにする。

// infoLines は情報部の行を返す。項目は screens.md の詳細画面のモックに従う。
func (d Model) infoLines() []string {
	r := d.target
	svc, svcRole := serviceText(r)
	job, jobRole := jobText(r)
	version, versionRole := versionText(r)
	items := []struct {
		label string
		value string
		role  token.RoleToken
	}{
		{"スコープ", r.Scope.String(), token.RolePlain},
		{"起動方式", managedText(r), token.RolePlain},
		{"サービス", svc, svcRole},
		{"ジョブ", job, jobRole},
		{"バージョン", version, versionRole},
		{"ディレクトリ", r.Dir, token.RolePlain},
		{"work", r.WorkDir, token.RolePlain},
	}

	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, d.infoLine(it.label, it.value, it.role))
	}
	return out
}

// infoLine はラベルと値を 1 行に組む。
//
// 値は装飾する前に幅へ収める。装飾してから切り詰めると ANSI 列が壊れる
// （molecule.styledCell と同じ順序に揃えてある）。
func (d Model) infoLine(label, value string, role token.RoleToken) string {
	rest := max(d.width-labelWidth, 1)
	return d.styles.Muted.Render(atom.Cell(label, labelWidth, atom.Left)) +
		d.styles.Style(role).Render(atom.Truncate(value, rest))
}

// managedText は起動方式の行の値を返す。systemd 管理ならユニット名も添える。
//
// 判定できていない 2 つの状態は一覧と同じ記号（- / ?）にせず文で出す。列幅の制約が
// ある一覧では 1 セルに詰めるしかないが、余裕のある詳細画面では何が分かっていないのかを
// 読み取れるようにする（ManagedBy.String はテーブル表示用の短い表記であり、詳細用の
// 表記は表示側の関心事なのでここに置く）。
//
//   - ManagedUnknown: ユニットもプロセスも無い（未稼働だと分かっている）
//   - ManagedUnavailable: ユニットが有るか分からない（systemctl list-units が失敗した）
func managedText(r runner.Runner) string {
	switch r.Managed {
	case runner.ManagedUnknown:
		return managedUnknownText
	case runner.ManagedUnavailable:
		return managedUnavailableText
	case runner.ManagedSystemd, runner.ManagedStandalone:
	}
	if r.UnitName == "" {
		return r.Managed.String()
	}
	return r.Managed.String() + "（" + r.UnitName + "）"
}

// serviceText はサービスの行の値と表示上の役割を返す。
//
// enabled / disabled（UnitFileState）を併記するのは、停止中の runner が次の起動で
// 上がるかどうかがここでしか分からないためである。
func serviceText(r runner.Runner) (string, token.RoleToken) {
	if r.Svc == nil {
		return atom.StatusText("", "")
	}
	if r.Svc.Active == "" {
		// systemctl show が失敗したユニット。ユニットが無いのとは書き分ける。
		return atom.StatusUnknown()
	}
	text, role := atom.StatusText(r.Svc.Active, r.Svc.Sub)
	if r.Svc.FileState != "" {
		text += " / " + r.Svc.FileState
	}
	return text, role
}

// jobText はジョブの行の値と表示上の役割を返す。
//
// リポジトリ名は出さない。Runner.Worker（/proc 由来）にはジョブのリポジトリ情報が
// 無く、ログから取るのはログのパッケージの担当である。
func jobText(r runner.Runner) (string, token.RoleToken) {
	if !r.Busy() {
		// 表記を一覧（JOB 列）と揃えるため atom に任せる。
		return atom.JobText(false, 0)
	}
	pids := make([]string, 0, len(r.Workers))
	for _, w := range r.Workers {
		pids = append(pids, strconv.Itoa(w.PID))
	}
	return token.IconJob + " 実行中 " + atom.Duration(r.JobElapsed()) +
		"（Worker PID " + strings.Join(pids, ", ") + "）", token.RoleAccent
}

// versionText はバージョンの行の値と表示上の役割を返す。
//
// 最新版は渡さない。最新版の取得は GitHub API を使う機能の担当であり、未取得の
// 状態を「古い」と示さないためである（atom.VersionText の契約）。disableUpdate は
// 自動更新が止まっていることを示すため、設定されていれば添える。
func versionText(r runner.Runner) (string, token.RoleToken) {
	text, role := atom.VersionText(r.Version, "")
	if r.Config.DisableUpdate {
		text += "（disableUpdate=true）"
	}
	return text, role
}
