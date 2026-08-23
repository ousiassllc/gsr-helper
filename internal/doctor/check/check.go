package check

import (
	"context"
	"net"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// Check は 1 つの診断項目（components/overview.md の internal/doctor）。
//
// 項目を足すことが既存コードに影響しないよう、各項目を独立した実装にして
// レジストリ（internal/doctor.Default）へ並べるだけの構造にしてある。
//
// **Run の戻りが複数なのは components/overview.md の草案（CheckResult 単数）
// からの意図的な変更である。** パーミッションや docker グループ所属のように
// runner ごとに判定する項目は runner の数だけ行が並ぶ（screens.md の Doctor
// タブは TARGET 列に runner 名を出す）。単数に固定すると、レジストリが runner
// 一覧を知る前に項目を組み立てられないか、複数の runner の結果を 1 行へ畳んで
// TARGET 列を捨てるかの二択になる。前者はレジストリを検出結果に依存させ、
// 後者は画面仕様を満たせない。
type Check interface {
	// ID はチェックの識別子。レジストリ内で一意にする。
	ID() string
	// Category は分類。上記の Cat* のいずれかを返す。
	Category() string
	// Startup は起動時の自動実行（FR-44）の対象かを返す。
	//
	// 真にしてよいのはホスト内の読み取りと軽量なコマンドで完結する項目だけ
	// である（ネットワーク到達性やディスク集計は含めない）。
	Startup() bool
	// Run は診断を実行する。対象が無ければ空を返す（nil でよい）。
	//
	// 並列に呼ばれるため、実装は Input を書き換えてはならない。
	Run(ctx context.Context, in Input) []Result
}

// ProbeTimeout は 1 コマンドあたりの既定の待ち時間。
//
// hostcaps の能力判定（500ms）より長くしてあるのは、doctor が叩くのが
// `docker info` や `sudo -l -U` のように応答が遅いことのあるコマンドだから
// である。診断は 3 秒周期の再検出と違って利用者が明示的に起こすものなので、
// 取りこぼすより待つ方を採る。
const ProbeTimeout = 3 * time.Second

// Input は全チェックに配る入力。
//
// テストのための差し替え口（Now / Dial / Getenv / LookPath / FSRoot /
// NewClient）を持つ。ゼロ値のままでも実環境を見る既定へ落ちるので、本番の
// 組み立て側はドメインの値（Runners / Caps / Exec）だけを詰めればよい。
type Input struct {
	// Runners は検出済みの runner 一覧。親 Model が検出したものを配る
	// （doctor は自分で検出しない）。
	Runners []runner.Runner
	// Caps は起動時の能力判定。SKIP の判断に使う。
	Caps appconfig.Caps
	// Exec は外部プロセス実行の唯一の経路。nil のときコマンドを使う
	// チェックは SKIP を返す。
	Exec exec.Executor

	// Now は現在時刻。nil なら time.Now。
	Now func() time.Time
	// Dial はネットワーク到達性の確認に使う。nil なら TCP のダイヤラ。
	Dial func(ctx context.Context, network, addr string) (net.Conn, error)
	// Getenv は環境変数の参照。nil なら os.Getenv。
	Getenv func(string) string
	// LookPath はコマンドの存在確認。nil なら exec.LookPath。
	LookPath func(name string) (string, error)
	// FSRoot は /proc や /etc を読むときの起点。空なら実ファイルシステム。
	FSRoot string
	// NewClient は GitHub API クライアントを組み立てる。nil ならトークンから
	// 組み立てる既定（gh.Token → gh.New）を使う。
	NewClient func(ctx context.Context) (*gh.Client, error)
}
