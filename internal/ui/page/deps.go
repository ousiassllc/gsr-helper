package page

import (
	"github.com/ousiassllc/gsr-helper/internal/audit"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// Deps は page がドメイン層を呼ぶための道具。StateMsg に載せて全 page へ配る。
//
// StateMsg の描画に使う値と分けてあるのは変わる理由が違うためで、置くのは複数の
// タブが使うものに限る（タブ 1 枚ぶんは SetupDeps / ConfigDeps）。理由は
// docs/ui/atomic-design.md の「共有状態は page.StateMsg で受け取る」にある。
type Deps struct {
	// Exec は外部プロセス実行の唯一の経路。page がドメイン層を tea.Cmd で呼ぶときに使う。
	//
	// 共有状態に載せるのは、ドメイン層を呼べるのが page 階層だけ（atomic-design.md の
	// 依存の規則）であり、その page に Executor を渡す道が他に無いためである。載せないと
	// タブを足す Issue ごとにこの構造体と親 Model の 2 箇所を直すことになる。
	//
	// **systemctl が無い環境でも nil にはしない。** 検出（discover.go）は Executor を
	// nil にして systemd の参照を落とす縮退を持つが、それは runner.Discover の契約で
	// あってこの層の約束ではない。page は systemctl を使えるかを Caps.Systemd で判断し、
	// nil 判定を各タブに書かせない。
	//
	// 監査記録の失敗は Executor 自身が通知先（command.WithAuditErrorFunc）へ渡し、
	// cmd 側が TUI の終了後にまとめて出す。**UI から stderr へ書かない**（描画が壊れる）ため、
	// 監査エラーの受け皿を page へ配る必要はない。
	Exec exec.Executor
	// ScanProcs は稼働プロセスの走査を差し替える口（svc.Drainer.Scan と同じ形。nil なら
	// svc が procs.Scan で /proc を読む）。**テストをホストのプロセス表から切り離すための
	// 継ぎ目である**（SetupDeps.NewClient / Fetch と同じ役目。既定は pagetest.ScanOf が
	// 入れる。理由は Issue #155 とそちらの doc）。runner.Process は procs.Process の別名。
	ScanProcs func() ([]runner.Process, error)
	// Audit は破壊的操作の記録先。外部コマンドを伴わない削除（internal/disk の
	// ファイル削除など）を記録するために page 階層まで配る（Issue #71）。
	//
	// nil / audit.Discard() は no-op で、監査ログを開けない場合の縮退はそのまま
	// 働く。UI から直接書き込む経路は無く、渡すだけで済む点は Exec と同じである。
	Audit *audit.Logger
}
