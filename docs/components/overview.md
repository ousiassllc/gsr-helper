# コンポーネント設計

パッケージの分割・責務・依存関係を定める。全体構成は [アーキテクチャ設計](../architecture/overview.md)、内部モデルは [データモデル](../architecture/data-model.md) を参照。

## 依存関係

```mermaid
graph TD
    Main[cmd/gsr-helper]

    subgraph UI[UI 層]
        UIApp[ui<br/>親 Model / page]
        UIParts[ui/template・organism<br/>ui/molecule・atom・token]
    end

    subgraph Domain[ドメイン層]
        Runner[runner]
        RScope[runner/scope]
        Svc[svc]
        Setup[setup]
        SetupJob[setup/job]
        Disk[disk]
        Logs[logs]
        Doctor[doctor]
        Config[config]
    end

    subgraph Infra[インフラ層]
        Exec[exec]
        GH[gh]
        Audit[audit]
        Appconf[appconfig]
    end

    Main --> UIApp
    Main --> Appconf
    Main --> Audit
    Main --> GH

    UIApp --> UIParts

    UIApp --> Runner
    UIApp --> Svc
    UIApp --> Setup
    UIApp --> SetupJob
    UIApp --> GH
    UIApp --> Disk
    UIApp --> Logs
    UIApp --> Doctor
    UIApp --> Config
    UIApp --> Appconf

    Svc --> Runner
    Svc --> Appconf
    SetupJob --> Runner
    SetupJob --> Setup
    SetupJob --> GH
    Setup --> Runner
    Setup --> Svc
    Disk --> Runner
    Logs --> Runner
    Doctor --> Runner
    Doctor --> GH
    Config --> Runner
    Config --> GH

    Runner --> Exec
    Runner --> RScope
    Svc --> Exec
    SetupJob --> Exec
    SetupJob --> RScope
    Setup --> Exec
    Setup --> RScope
    Disk --> Exec
    Disk --> Audit
    Logs --> Exec
    Doctor --> Exec
    GH --> Exec
    GH --> RScope
    GH --> Appconf
    Appconf --> Exec
    Exec --> Audit
```

**UI 層が `setup` と `gh` を直に参照するのは値の型のためである。** 実行前プレビュー（FR-16）は `setup.Plan` をそのまま描くので `ui/page` 以下が `setup` を import し（本番ファイル 7 本）、短命トークンの預け先 `gh.Secrets` は起動時に `cmd` が 1 つ作って UI へ配るため `cmd` と `ui` の双方が `gh` を import する。**実行そのものを呼ぶのは `setup/job` だけである**——UI は `setup.Apply` を直接叩かない。

### 依存の規則

| 規則 | 理由 |
|------|------|
| ドメイン層は `ui` に依存しない | TUI なしでテストできるようにする |
| ドメイン層は bubbletea に依存しない | 同上。UI 層が `tea.Cmd` でラップする |
| ドメイン層は `os/exec` を直接使わない | 監査ログとタイムアウトの適用漏れを防ぐ（`exec` 経由に限定） |
| `runner` は他のドメインパッケージに依存しない | 最下層のモデルとして全体から参照されるため |
| `exec` はドメインを知らない | 汎用のプロセス実行に留める |
| ドメイン層を呼ぶのは `ui/page` のみ | 副作用の発生点を UI の 1 階層に閉じる。詳細は [TUI コンポーネント設計](../ui/atomic-design.md#依存の規則) |
| 循環依存を作らない | — |

## パッケージ責務

### `cmd/gsr-helper`

エントリポイント。フラグ解析、設定の読み込み、監査ログのオープン、能力判定、色を使うかの判定、`tea.Program` の起動を行う。ロジックを持たない。

**秘密情報の提供元（`gh.Secrets`）と、トークン有無の判定手段（`gh.HasToken`）を組み立てるのもここである。** どちらも `internal/gh` の実装を、それを知らない側（`internal/exec/command` と `internal/appconfig/hostcaps`）へ渡す配線であり、依存の向きを増やさずに実装を 1 つに保つための一手である。`gh.Secrets` は `command.New` の値一致マスク（段 2）の提供元と、Setup タブが取得した短命トークンの預け先を兼ねる**同じ 1 つの実体**で、預けた値がそのまま監査ログとエラー文言のマスクに効く（[セキュリティ設計](../architecture/security.md#監査ログでのマスク)）。

代替スクリーンへの切り替えは親 Model が宣言し、panic からの端末復元は bubbletea が行う。`cmd` は `tea.NewProgram(app).Run()` を呼ぶだけで、どちらも自分では扱わない（扱う箇所を 2 つ持つと、片方だけが効いた状態を追えなくなる）。

色を使うかは `NO_COLOR` / `--no-color` / 非 TTY から**ここで 1 つの値に決めて**親 Model に渡す。判定箇所を分散させない。背景の明暗は起動後に端末へ問い合わせるため、親 Model が受け持つ（[アーキテクチャ設計](../architecture/overview.md#起動シーケンスと能力判定)）。

| フラグ | 意味 |
|-------|------|
| `--root <path>` | 追加の走査ルート（複数指定可）。`scan_roots` と同じ検査を通す（絶対パスで `..` を含まない） |
| `--config <path>` | 設定ファイルのパスを指定 |
| `--refresh <秒>` | 自動更新間隔の上書き。`refresh_interval` と同じ有効範囲（1〜3600 秒） |
| `--no-color` | 色を使わない（`NO_COLOR` も尊重） |
| `--version` | バージョン表示 |
| `-h` / `--help` | 使い方を標準出力へ出して終了（終了コード 0） |

#### 終了コード

| コード | 意味 |
|-------|------|
| 0 | 正常終了。`--version` / `--help` もこれ |
| 1 | 起動または実行の失敗（設定の読み込み失敗、`tea.Program` の失敗など） |
| 2 | 引数の誤り。メッセージと使い方を**標準エラー出力**へ出す |
| 3 | panic からの復帰 |

`--version` / `--help` は標準出力、引数の誤りは標準エラー出力に出す。パイプで受けたときに正常な出力とエラーが混ざらないようにするためである。

#### 監査ログを開けない場合の縮退

監査ログを開けなくても**起動は続ける**。ホスト内の状態を読むだけの一覧表示が、ログの出力先の権限で使えなくなることを避けるためである。

| 状況 | 挙動 |
|------|------|
| `audit_log` を開けない・検証に落ちた | 記録しない書き込み先（`audit.Discard()`）に差し替え、`警告: 監査ログを開けませんでした（記録せずに続行します）: <原因>` を標準エラー出力へ 1 行出す。代替スクリーンへ入る前に出すため、終了後の画面に残る |
| 実行中の記録の書き込みに失敗した | 実行そのものは成功として扱い、失敗を集計する。TUI の終了後に `警告: 監査ログの記録に N 件失敗しました（最初の失敗: <原因>）` を標準エラー出力へ出す |
| 終了時に監査ログを閉じられなかった | TUI の終了後に `警告: 監査ログのクローズに失敗しました: <原因>` を標準エラー出力へ出す。**捨てない。** 開けなかった場合と記録に失敗した場合は警告が出るのに閉じ損ないだけが見えないと、書けなかったレコードの存在に気付けない |

**縮退した場合は「全外部コマンドの監査ログ記録」（[セキュリティ設計](../architecture/security.md)）が効いていない。** 記録が要件である運用では、この警告を起動の失敗として扱う運用手順を用意すること。TUI の実行中に標準エラー出力へ書かないのは、描画が壊れて画面が読めなくなるためである。

### `internal/runner`

runner の検出とモデル定義。**最下層**であり、他のドメインパッケージに依存しない。

| 要素 | 責務 |
|------|------|
| `Runner` / `Config` / `Process` / `SvcState` / `Result` | モデル定義 |
| `Discover(ctx, Options) Result` | 3 経路から収集してマージする**唯一の入口**。`Options.Exec` が `nil` のとき systemd を参照せず、**警告も返さない**（systemctl 不在時の縮退。3 秒ごとのポーリングで同じ警告が積み上がらないようにするため。可否は起動時の `Caps` としてヘッダに出る） |
| `IsRunnerDir` / `LoadConfig` | runner ディレクトリの判定と `.runner` の読み取り |
| `DefaultRoots()` | 既定の走査ルート（10 個の glob パターン）を展開して返す。全量は [FR-01](../requirements/functional.md#既定の走査ルートfr-01) |
| `scanRoots`（非公開） | 実際に掘るルートを決める。`Options.SkipDefaultRoots` が偽なら `DefaultRoots()` に `Options.Roots` を足し、真なら `Options.Roots` だけを返す |
| `findRunnerDirs`（非公開） | ルート配下の探索。`.runner` を見つけた時点で**その配下は掘らず**そのディレクトリを返し、降りる途中で名前が `_work` / `_diag` のディレクトリは辿らない |
| `attach`（非公開） | 正規化済み入力を受け取る**純粋関数**。プロセス・ユニットの紐付け、孤児ユニットの抽出、`Runner.Managed` の決定、警告の生成 |
| `resolveRunAsUser`（非公開） | runner の実行ユーザーの決定。`SvcState.User` を第一、`Listener` の UID を第二の情報源とする（UID → 名前の解決関数を引数で受ける純粋関数） |

`attach` は紐付け以外に次も決める。**表示の決定性を保つために必要な処理をここに集めている。**

- `Runner.Managed` の 4 値。判定に「ユニット一覧が取れたか」を引数で受け取る（[FR-03](../requirements/functional.md#起動方式の-4-状態fr-03)）。取れていない場合に「ユニットが無い」と読み替えない
- `Workers` を PID 順に並べる。`/proc` の走査順に依存すると、同じ状態でも表示が入れ替わる
- 同じ runner に複数の `Listener` が見つかった場合は `Started` の新しいものを採る（古い残骸プロセスを稼働中として出さない）
- ユニット照合は `UnitName` を第一パス、`WorkingDirectory` を第二パスの 2 段で行う。1 パスで回すと、あるユニットの `WorkingDirectory` 一致が別のユニットの `UnitName` 一致を上書きしうる

#### 走査ルートの合成（`--root` / `scan_roots` / `SkipDefaultRoots`）

走査ルートは 3 つの入口から決まる。合成の順序と既定値を次に定める。

| 入口 | 与えるもの | 既定 |
|------|-----------|------|
| `DefaultRoots()` | [FR-01](../requirements/functional.md#既定の走査ルートfr-01) の 10 個の glob を展開したもの | **使う**（`Options.SkipDefaultRoots` が偽） |
| 設定ファイルの `scan_roots` | 追加の走査ルート | 空 |
| `--root <path>`（複数指定可） | 追加の走査ルート | 空 |

`cmd/gsr-helper` は `scan_roots` の後ろに `--root` を並べて `Options.Roots` に渡し、`internal/runner` はその前に `DefaultRoots()` を置く。したがって最終的な走査順は **既定ルート → `scan_roots` → `--root`** である。

**`--root` は `scan_roots` と同じ検査を通し（`appconfig.CleanScanRoot`）、重複除去も入口をまたいで行う（`appconfig.MergeScanRoots`）。** 検査を入口ごとに分けると、安全上の根拠がある絶対パス・`..` の検査が `--root` だけ効かない状態になる。重複除去を入口ごとに分けると、`scan_roots` と `--root` に同じルートを書いたときに同じディレクトリを 2 度走査する。同じ runner が 2 度一覧に出ることは `collectDirs` が実パスで畳むため起きないが、走査そのものは 2 度走る。

**`--root` は「既定ルートの置き換え」ではなく「追加」である。** 既定を置き換える指定にすると、`--root` を 1 つ足しただけで既定の設置場所にある runner が一覧から消える。runner を見落とす側に倒れる既定は取らない。

`Options.SkipDefaultRoots` は**既定ルートを使わない**ことを呼び出し側が明示するための指定で、既定は偽（使う）である。テストのように走査対象を完全に固定したい呼び出しのために用意してあり、`cmd/gsr-helper` からは設定できない（利用者向けのフラグ・設定項目は持たない）。真にしても [FR-02](../requirements/functional.md#検出一覧fr-01fr-05) の補完（稼働プロセスと systemd ユニット由来のディレクトリ回収）は止まらない。走査ルートは「どこを掘るか」の指定であって、検出全体の範囲ではないためである。

`resolveRunAsUser` が `SvcState.User` を採るのは**ユニットの実体がある場合に限る**（`Load` が空でも `not-found` でもない）。`systemctl show` に失敗したプレースホルダと `svc.sh uninstall` 後の残骸ユニットは `User=` が空であり、そのまま採ると全 runner が root と表示される。名前が解決できないときは UID の 10 進表記になる（[データモデル](../architecture/data-model.md#runasuser-の決定と-uid-フォールバック)）。

パースと判定は I/O から分離し、`parseConfig` / `parseListUnits` / `parseShow` / `attach` を個別にテストする。

`Discover` の systemd 参照の所要時間は「1 コマンドのタイムアウト（既定 30 秒）× ceil(ユニット数 / 8)」まで伸びるため、呼び出し側は deadline 付きの `ctx` を渡す。`systemctl show` の同時実行数は 8 である。`ctx` のキャンセル後は残りの `systemctl show` を発行しない。

**`systemctl list-units` 自体が失敗した場合は状態を 1 件も返さない。** ユニットの一覧が無ければどのユニットを `show` すべきかも分からないためである。この場合の警告は `systemd.ErrListUnits` を包んだ 1 件だけで、呼び出し側は `errors.Is` で「ユニットが 0 件」と「一覧が取れていない」を区別する。区別しないと起動方式を誤判定する（[FR-03](../requirements/functional.md#起動方式の-4-状態fr-03)）。**これが検出処理で最も影響範囲の広い縮退である。**

**次の 2 種のユニットは孤児として扱わない。** どちらも「対応する runner ディレクトリが無い」わけではないため、孤児区画（[FR-05](../requirements/functional.md#孤児ユニットに含めないものfr-05)）に出さず `Result.Warnings` に集約する。

- `systemctl show` に失敗したユニット。状態が取れないと `WorkingDirectory` が空になるため、`.service` ファイルを持たない runner のユニットが「対応ディレクトリなし」と誤判定される。警告は systemd の参照が既に返しているので二重に報告しない
- `LoadState=not-found` のユニット。`svc.sh uninstall` 後に参照だけが残っている状態で、runner ディレクトリは存在する。**runner への紐付けも行わない**（存在しないユニットに対してサービス制御を提示しないため。`resolveRunAsUser` も同じ理由で `not-found` を除いている）

紐付けを行わないユニットと、既に別のユニットが紐付いた runner ディレクトリを指す 2 本目のユニットは、黙って落とさず警告として出す。文言は [データモデルの `Result` の警告](../architecture/data-model.md#result-の警告) に定める。

### `internal/runner/procs`・`internal/runner/systemd`

`internal/runner` の収集経路のうち、`/proc` の走査（`procs`）と systemd の参照（`systemd`）を行数上限のために切り出したもの。**`internal/runner` から一方向に import し、逆向きの参照は作らない。** `Process` / `ProcKind` / `SvcState` は `internal/runner` の別名として再公開してあるため、呼び出し側は分割を意識しなくてよい。**下位パッケージの `Scan` は再公開しない。** 個別の経路を外から呼ぶと 3 経路の突き合わせ（`Discover`）を通らない結果が生まれ、`systemd.ErrListUnits` の解釈のような `Discover` 内の判断を呼び出し側が再実装することになるためである（同じ理由で `ErrListUnits` も再公開しない）。

| パッケージ | 主な要素 |
|-----------|---------|
| `runner/procs` | `Process` / `Kind` / `Scan`。`/proc/<pid>/exe` の末尾に付く ` (deleted)` を照合の前に落とす（runner の自動更新でバイナリが差し替わっても稼働中の runner を見落とさない）。`Process.Exe` には印を残した生の値を保つ |
| `runner/systemd` | `State` / `Scan` / `ErrListUnits`。`list-units` と並列の `show`、`WorkingDirectory` の先頭 `-` の除去 |

### `internal/runner/scope`

`.runner` の `gitHubUrl` からスコープ（repo / org / enterprise）を判定する。**純粋な文字列処理のみ**で、ホスト走査・`/proc`・systemd に依存しない。

| 要素 | 責務 |
|------|------|
| `Scope` / `Kind` | スコープのモデル定義（`Runner.Scope` の型） |
| `Parse(gitHubUrl) (Scope, error)` | スコープ判定。ホストを含まない URL・`orgs` / `enterprises` の単独指定は誤りとして拒否する |

`internal/runner` の下位に置くのは 2 つの理由による。`Scope` は GitHub API のパス生成にも使うため（[データモデル](../architecture/data-model.md#scope)）、`internal/gh` が `internal/runner` 全体を import せずにスコープだけを参照できる。また `internal/runner` の行数上限（1 ディレクトリ 2000 行）に対する余裕を確保する。

行数チェック（`linterly`）の集計は直下のファイルのみを対象とする。現在の使用量は `internal/runner` 1656 / `runner/systemd` 552 / `runner/procs` 426 / `runner/scope` 165 行である。**`internal/runner` は残り 344 行しか無い。** サービス制御や追加・削除の Issue が `runner` へ機能を足す場合は、先に切り出し先を決めること。

`Parse` は判定できない入力を必ず error にする。**Unknown を error 無しで返すことはない**（[データモデル](../architecture/data-model.md#scope)）。

### `internal/svc`

サービス制御とドレイン停止。外部プロセスは `exec.Executor` 経由でのみ起動する。

| 要素 | 責務 |
|------|------|
| `Op` | サービス制御操作の識別子（`OpStart` / `OpStop` / `OpKill` / `OpDrain` / `OpRestart` / `OpEnable`） |
| `Start` / `Stop` / `Restart` / `Enable` / `Disable` | `systemctl <verb> <unit>` の呼び出し。ユニット名が空なら実行せず `ErrNoUnit` を返す |
| `Kill(ctx, Executor, Runner)` | **強制停止**。`Runner.Listener` と `Runner.Worker` の PID へ `kill -KILL` を 1 回発行し、ユニット名があれば続けて `systemctl stop` を発行する。systemd 管理でなくても動く。**PID もユニット名も無ければ 1 本も発行せず `ErrNoKillTarget` を返す** |
| `DaemonReload` | drop-in 変更の反映 |
| `Drain(ctx, Executor, Runner, progress)` / `Drainer` | `Runner.Worker` の消滅を待ってから停止。無制限に待ち、`ctx` のキャンセルで中断（このとき停止処理は行わない）。**ユニット名が無ければ待機に入らず `ErrNoUnit` を返す**（待ち切った先の `systemctl stop` が必ず失敗すると分かっているため）。`Drainer` は走査手段・間隔・時刻を差し替えられる |
| `CommandLine(Op, Runner) []string` | 操作が発行するコマンドを実行順に返す。確認ダイアログの「実行するコマンド全文」がこれを読む |
| `Enabled(Runner) bool` | ユニットが enable 済みかを返す。`OpEnable` が enable / disable のどちらへ倒れるかの判定を 1 箇所に置く |
| `CanControl(Op, Runner, Caps) (bool, string)` | 操作可否と不可の理由を返す。非 root・systemd 不在・`run.sh` 直起動・管理状態の判定不能を、この順に判定する。後ろ 2 段は同じ 5 操作（強制停止以外）を塞ぎ、理由の文言だけが違う |
| `ReasonRoot` / `ReasonSystemd` / `ReasonStandalone` / `ReasonManagedUnknown` / `ReasonNoCommand` | 不可の理由の文言。表示側が同じ文言を持たないよう公開する |

`CanControl` が `Op` を取るのは、[無効な操作の表示](../ui/screens.md#無効な操作の表示) の判定表が操作ごとに塞ぐ範囲を変えるためである（root が要るのは開始・停止・強制停止・再起動の 4 つで、ドレイン停止と enable の切替は要らない）。`CanControl` を 1 箇所に集約し、UI 側で操作可否の判断を再実装しない。

**`run.sh` 直起動（3 段目）と管理状態の判定不能（4 段目）は同じ 5 つ（開始・停止・ドレイン停止・再起動・enable の切替）を塞ぎ、違うのは理由の文言だけである。** どちらも「`systemctl` を安全に駆動できない」点で同じ状況で、理由が「systemd 管理外だと判明している」のか「そもそも判定できない」のかだけが違う。どちらの段でも通すのは強制停止だけで、worker のプロセスへ直接シグナルを送る操作は `systemctl` の可否に依らないためである。**ドレイン停止を塞ぐ**のは、待機そのものは `/proc` の走査だけでも Worker が消えたあとに発行するのは停止とまったく同じ `systemctl stop` だからで、停止が塞がれた runner に対しドレイン停止だけが確認も猶予も無く同じコマンドを発行することになる（Worker が 0 件なら `Drainer.Drain` は初回走査で即停止へ抜ける。`run.sh` 直起動ではそもそも待機に入る前に `ErrNoUnit` を返す）。**enable の切替を塞ぐ**のは、`systemctl enable` / `disable` が [FR-09](../requirements/functional.md) の言う systemd 操作そのもので、管理状態が判定できないときに塞ぐ以上、管理外と判明しているときに通す理由が無いためである。

**`ReasonNoCommand` は `CanControl` が返す理由ではない。** `CanControl` の 4 段が見るのは起動方式と能力だけで、対象そのものの有無は見ない。可否を通っても `CommandLine` が空になる runner（未稼働かつサービス未インストールで、PID もユニット名も無い）は残るため、UI 側（`ui/page/runnerop`）が**確認ダイアログを開く前に**この理由で対象から外す。文言だけを `svc` に置くのは、理由の出どころを 1 箇所に保つためである。

**確認ダイアログに出すコマンド全文は `CommandLine` から引く。** 表示側で `"systemctl " + verb + " " + unit` を組み直すと、承認した文面と実際に発行される内容が食い違いうる。「実行コマンド全文の提示と `y/N`」（[画面仕様の操作フロー](../requirements/functional.md)）は、提示と実行が同じ 1 箇所から出ていて初めて意味を持つ。

**`Kill` は `systemctl kill` を使わない。** 対象を main プロセス以外へ広げるフラグの綴りが systemd のバージョンで変わり（`--kill-who` / `--kill-whom`）、既定のままでは main プロセスしか落とせずに worker が生き残るためである。プロセスを落とした後に `systemctl stop` まで打つのは、シグナルだけでは systemd 側が「停止した」と記録せず `Restart=` 付きのユニットが戻ってくるためである。`kill` が失敗しても `stop` は試み、両方の結果を `errors.Join` でまとめる。**どちらの段も対象を持たない場合は 1 本も発行せず `ErrNoKillTarget` を返す。** 発行が 0 本のまま `errors.Join(nil...)` を返すと nil になり、何もしていない実行が「成功」として報告されて、止まっていない runner を止まったものとして扱わせてしまう。

### `internal/setup`

runner の追加・削除・バージョン更新。最も破壊的な操作を担う。

| 要素 | 責務 |
|------|------|
| `Kind` | 計画の種類（`KindAdd` / `KindRemove` / `KindUpdate`）。`String()` が確認ダイアログの見出しに出る表示名（`追加` / `削除` / `バージョン更新`）を返す |
| `Plan` / `Unit` / `Step` | 確定した計画。`Plan` は台ごとの `Unit`、`Unit` は実行順の `Step` を持つ。`Step.Kind` は `StepCommand` / `StepMkdir` / `StepExtract` / `StepDrain` の 4 種 |
| `PlanAdd(AddSpec) (Plan, error)` | 追加の計画を立てる。作成するディレクトリと実行コマンドを**確定させてから**返す。検証に落ちた時点でエラーにし、途中まで作った計画は返さない |
| `PlanRemove(RemoveSpec)` / `PlanUpdate(UpdateSpec)` | 削除・更新の計画。削除は `svc.sh stop` → `svc.sh uninstall` → `config.sh remove --token` の順（FR-17。ユニットが無い runner には `svc.sh` の 2 手順を入れない）、更新は「ドレイン停止 → 展開 → 起動」の順（FR-22） |
| `Apply(ctx, ApplyInput) (Result, error)` | 計画を順に実行。失敗した時点で中止し、成功分と失敗理由を `Result` に載せて返す（FR-15）。`error` は `Result.Err` と同じもの |
| `Progress` / `Result` / `StepError` | 1 手順ごとの進捗、実行結果（成功した台 / 失敗した台とフェーズ / 着手しなかった台）、どの台のどのフェーズで失敗したかを保つエラー |
| `NextIndex(existing []string, prefix string) int` / `RunnerName` / `Names` | 命名の連番決定（**純粋関数**。ホストの状態を一切読まない） |

**計画（Plan）と実行（Apply）を分離する。** これにより実行前プレビュー（FR-16）が計画をそのまま表示するだけで実現でき、表示と実行の食い違いが起きない。

**短命トークンは計画に載せない。** `Step.TokenIndex` が「`Args` のどこにトークンが入るか」だけを持ち、`Args` にはマスクのプレースホルダ（`mask.Placeholder`）が入ったままである。実際の値は `Apply` が `Args` の複製に対して実行の直前に差し込む。プレビューが参照するのは常にプレースホルダのままの `Args` なので、**承認画面にトークンが載る経路が構造として存在しない**（[セキュリティ設計](../architecture/security.md#保持と出力)）。

**短命トークンの渡し方は 2 系統ある。** `ApplyInput.Token` は全台で共通の 1 本（追加はスコープが 1 つなので使い回せる。FR-14）、`ApplyInput.TokenFor` は台ごとに引く関数で、設定されていればこちらが優先する。削除の対象は複数のスコープにまたがりうるうえ remove token はスコープごとに発行されるため、1 本を使い回すと別スコープの台で必ず失敗する。

**トークンの取得と tarball の手配はこのパッケージに入れない。** `internal/setup` が知るのは計画とその実行だけで、GitHub API もネットワークも触らない（`internal/gh` を import しない）。外部資源を揃える側は `internal/setup/job` である。

#### 分割したパッケージ

1 ディレクトリ 2000 行（テスト込み）の上限に対する分散と、責務の切り分けを兼ねる。依存は **`setup/job` → `setup` → `setup/tarball` / `setup/valid`** の一方向で、逆向きは無い。**`setup/job` は `setup/tarball` も直に import する**（取得情報を `tarball.Info` に詰めて `tarball.Fetch` を呼ぶのが `setup/job` の役目のため）。`setup` を経由しなければならない決まりではなく、下位のパッケージを飛び越して参照してよい。

| パッケージ | 置くもの |
|-----------|---------|
| `setup` | `Plan` / `Unit` / `Step` / `PlanAdd` / `PlanRemove` / `PlanUpdate` / `Apply` / `NextIndex` |
| `setup/valid` | 入力検証（`Name` / `Labels` / `Dir` / `URL` / `Count`）とその理由の文言。**外部コマンドを一切起動しない純粋な判定**であり、[セキュリティ設計の「入力を検証してから渡す」](../architecture/security.md#外部コマンド実行の安全性) の表を実装する |
| `setup/tarball` | tarball の取得（`Fetch`）・検証（`Verify`）・展開（`Extract`）・上書きしない名前（`PreservedNames`。FR-21）。**内部パッケージを 1 つも import しない**（`net/http` と `os` だけで完結する） |
| `setup/job` | 短命トークンの取得と tarball の手配を済ませて `setup.Apply` を呼ぶ（`Deps` / `Input` / `Run` / `LatestVersion`）。`internal/gh` を import する唯一のドメイン側 |

**`setup/job` を UI 層から分けたのは、この一連が bubbletea を知らない普通の関数として書けるためである。** TUI なしでテストでき、UI 層に残るのは「進捗を画面へ流す」ことだけになる。`Run` を呼ぶのは承認のあとだけである——承認前に呼ぶと、キャンセルした場合にも有効な短命トークンを発行してしまう。

`setup/job` は短命トークンをスコープと操作の組でキャッシュし、期限内なら使い回す（FR-14）。取得したトークンは `gh.Secrets` へ預け、実行を終えたら忘れる。預けている間だけ監査ログとエラー文言の値一致マスクが効く（[セキュリティ設計](../architecture/security.md#監査ログでのマスク)）。tarball も 1 回だけ取得して各ディレクトリへ展開に使い、終わったら消す（FR-13）。

**`setup/tarball` は展開の前に SHA-256 を必ず検証する。** 検証に失敗した場合は展開せず、取得したファイルを消してエラーを返す。root 権限で動くため展開は tar の中身を信用せず、`os.OpenRoot` で展開先を根に固定したうえで絶対パス・`..` を含むエントリ・展開先の外を指すリンク・**保持対象（`PreservedNames`）へリンクで潜り込むエントリ**を拒否し、1 エントリ 1 GiB / 合計 2 GiB の上限も置く（展開量で埋め尽くされないため）。最後の 1 つ（`ErrPreservedLink`、`keeplink.go`）が FR-21 の保持を守る検査である。`os.OpenRoot` は展開先の**外**への逸脱しか防がないため、`link -> .runner` の直後に `link/pwned` を並べる tar は、名前だけを見る保持判定をすり抜けて `.runner` の中へ書き込める。この展開で作ったリンクの向き先を覚え、**見かけの名前ではなく実際に書き込まれる場所**で保持判定を行う。 展開そのものは展開先の中の一時ディレクトリ（`.gsr-stage-<乱数>`）で行い、済んでから `rename` で最終位置へ移す（`stage.go`）——更新中に強制終了されても中途半端なファイルを残さないためである。移動は 1 件ずつなので新旧の混在までは防げない。詳細は[セキュリティ設計](../architecture/security.md#ネットワーク)。

### `internal/disk`

使用量の集計とクリーンアップ。

| 要素 | 責務 |
|------|------|
| `Scan(ctx, Runner, out chan<- Usage)` | 対象ごとに非同期集計し、判明順に送出 |
| `FSStats(path)` | 容量と inode の残量 |
| `DockerUsage(ctx, ex)` | `docker system df --format {{json .}}` の解析 |
| `PlanClean(targets) (CleanPlan, error)` | 削除計画。対象パスと解放見込み容量を確定させる（ドライラン）。保護された対象（`Target.Protected` が空でない）を 1 件でも含めば計画を作らない |
| `PruneReclaimable(items) int64` | `docker system prune -f` が実際に回収する見込みの容量（Containers / Build Cache のみ） |
| `ValidatePath(base, target) error` | **削除パスの検証**。基準ディレクトリ配下であること、`..` を含まないこと、許可サブツリー内であることを判定 |
| `Apply(ctx, ex, lg, CleanPlan, progress)` | 削除の実行。シンボリックリンクは辿らず、リンク自体のみを削除 |

`DockerUsage` と `Apply` が `exec.Executor` を取るのは、外部プロセス実行の唯一の経路が `internal/exec` だからである（上記「依存の規則」）。`docker system prune -f` は `exec.Options.Action` に `disk.clean` を設定して発行し、破壊的操作として監査ログに全件記録される（[セキュリティ設計](../architecture/security.md#監査ログ)）。

`Apply` が取る `lg *audit.Logger` は、ファイル削除（`removeTree`）を監査ログへ記録するための入口である（下記「ファイル削除の監査ログ」、[`internal/audit`](#internalaudit)）。`internal/disk` がこれを持つのはドメイン層で唯一の例外で、他のドメイン実装は `audit.Logger` を直接呼ばない。

**`Scan` の `out` は閉じない。** 呼び出し側が runner ごとの `Scan` を 1 本のチャネルへ集約するため、閉じる責務は集約する側にある。`Scan` は全対象を送り終えてから返るので、呼び出し側は `WaitGroup` で待ってから閉じられる。

集計と削除の対象は runner ディレクトリ直下の `_work` / `_diag` に固定する。`ValidatePath` が許可するサブツリーと同じものだけを見ることで、集計に出た対象が計画の段階で弾かれる食い違いを防ぐ。

`ValidatePath` は `PlanClean` と `Apply` の**両方**から呼ばれる構造にし、検証を通らないパスを削除できないようにする。`Apply` が削除直前にもう一度呼ぶのは、計画を組み立てずに `Apply` を呼ぶ経路が将来できても検証を迂回できないようにするためである。異常系のテストを必須とする（[セキュリティ設計](../architecture/security.md#1-削除パスの検証を必須にする)）。

**ジョブ実行中の保護（[FR-31](../requirements/functional.md)）も同じ形で二重にする。** `Scan` が判定した理由は `Usage.Reason` から `Target.Protected` へ引き継ぎ、`PlanClean` と `Apply` の両方が空でない `Protected` を拒否する。可否を UI（選択できない行）にだけ持たせると、`Target` を直接組む呼び出しが 1 つ増えた時点で保護が外れる（[セキュリティ設計](../architecture/security.md#3-ジョブ実行中の操作をガードする)）。

**`docker system prune -f` の解放見込みは内訳の合計ではない。** 発行するのはこの 1 本だけで、`--volumes` が無いためボリュームは消えず、`-a` が無いため dangling 以外の未使用イメージも残る。したがって解放見込みには `PruneReclaimable` が返す種別（Containers / Build Cache）だけを載せ、イメージとボリュームの `Reclaimable` は内訳の表示（[FR-27](../requirements/functional.md)）に留める。

**ファイル削除も監査ログに残る（Issue #71）。** ファイルの再帰削除（`removeTree`）は外部コマンドを起動しないため `Executor` を通らないが、`Apply` が呼ぶ `removeTarget`（1 対象の削除ごとに必ず通る 1 箇所）が `lg.Report` で記録する。`action` は docker と同じ `disk.clean`、`command` には実行したコマンドが無いため `["(削除)", <削除したパス>]` を載せる（`rm` のような実在するコマンド名にしないのは、実行していないコマンドを起動したと誤読させないため。[セキュリティ設計](../architecture/security.md#監査ログ)）。保護・検証で中止した対象も `exit_code: 1` と `error` 付きで記録し、「削除しなかった」事実を後から追えるようにする。この階層が発行する docker の 2 コマンドも**どちらも記録される**。`docker system df`（`Action: disk.df`）と `docker system prune -f`（`Action: disk.clean`）のいずれも `SkipAudit` を付けない。記録対象外にするのは再検出の `systemctl list-units` / `show` だけである（[外部インターフェース](../api/external-interfaces.md#systemd)）。

### `internal/logs`

ログの一覧と追従。

| 要素 | 責務 |
|------|------|
| `List(Runner) ([]File, error)` | `_diag` 配下の `Runner_*.log` / `Worker_*.log` を更新時刻の降順で列挙（サイズ・更新時刻付き） |
| `LatestWorker(Runner) (File, bool)` | 直近ジョブの Worker ログを特定（`l` の宛先） |
| `Tail(ctx, path, out chan<- Line) error` | `fsnotify` による追記の検知と送出 |
| `Journal(ctx, Executor, unit, out chan<- Line) error` | systemd ユニットのログを一定間隔で取得し、増えた分を送出 |
| `Classify(text) Level` | 行の重大度（`ERROR` / `WARN`）の判定。強調表示（FR-25）の入力 |

型名にパッケージ名を重ねない規約に従い、ログファイル 1 件は `File` と呼ぶ（`logs.LogFile` とはしない）。

**`Tail` / `Journal` は戻るときに `out` を閉じる。** 受け手（`ui/page/logs`）は閉じたことで購読の終わりを知り、読み直しの `tea.Cmd` を発行し続けずに済む。したがって `out` は 1 本の購読専用に作る。

**色は決めない。** 強調表示に使うのは `Level`（値）であり、色を割り当てるのは表示層である（[依存の規則](#依存の規則)）。

#### `journalctl -f` を使わない理由

`Journal` は `journalctl -u <unit> -n <N> --no-pager` を 2 秒ごとに発行し、前回の出力との重なりを除いた差分を送る。`-f`（follow）は使わない。

`Executor` は 1 回の実行の出力をまとめて返す契約であり（`Run(ctx, name, args...) (Result, error)`）、`-f` を渡すと**タイムアウトまで 1 行も届かない**。追従のためだけに標準出力をストリームで受け取る経路を開けると、外部プロセスの実行が `Executor` 1 本ではなくなり、タイムアウト・監査記録・マスクの適用漏れを構造的に防ぐという `internal/exec` の目的が崩れる。取得のたびにプロセスを起こす費用より、実行経路を 1 本に保つほうを採る。

重なりの判定は行の内容だけで行うため、**同じ文言が連続して出力された場合は重なりを長く取りすぎて数行を出し損ねることがある**。`journalctl` の行は時刻を含むため実際にはまれで、取りこぼしても次の取得で末尾側は必ず届く。

取得は監査ログに記録しない（`exec.Options.SkipAudit`）。理由は [セキュリティ設計](../architecture/security.md#記録対象外とする読み取りコマンド) を参照。

### `internal/doctor`

環境診断。チェックを追加しやすい構造にする。

```go
// Check は 1 つの診断項目。型は internal/doctor/check にある。
type Check interface {
    ID() string
    Category() string
    Startup() bool // 起動時の自動実行（FR-44）の対象か
    Run(ctx context.Context, in Input) []Result
}
```

- 各チェックを独立した `Check` の実装とし、レジストリ（`doctor.Default()`）に並べる。項目の追加が既存コードに影響しない。
- `Run(ctx, in)` は並列に実行される。`Input` に runner 一覧・`Caps`・`Executor` と、テストのための差し替え口（`Now` / `Dial` / `Getenv` / `LookPath` / `FSRoot` / `NewClient`）を渡す。差し替え口はゼロ値のままなら実環境を見る既定へ落ちるので、本番の組み立て側はドメインの値だけを詰めればよい。
- 能力不足で実行できないチェックは `SKIP` を返し、失敗と区別する。`SKIP` は影響も対処も持たない（対処すべき不備が見つかっていない）。
- `Startup()` が真のチェックは起動時にも実行する（[FR-44](../requirements/functional.md)）。判定はレジストリの絞り込み（`doctor.Startup`）だけで済み、doctor タブと起動時で実装が分かれない。対象はホスト内の読み取りと軽量なコマンドで完結するものに限る。

**`Run` の戻りは複数である。** パーミッションや docker グループ所属のように runner ごとに判定する項目は runner の数だけ行が並ぶ（[画面仕様](../ui/screens.md#doctor-タブ)の TARGET 列）。単数に固定すると、レジストリが runner 一覧を知る前に項目を組み立てられないか、複数 runner の結果を 1 行へ畳んで TARGET 列を捨てるかの二択になる。前者はレジストリを検出結果に依存させ、後者は画面仕様を満たせない。

#### パッケージの分割

1 ディレクトリ 2000 行（テストを含む）の上限に対し、10 分類ぶんの項目とその検査を 1 つのディレクトリへ置くと収まらない。分類ごとに下位パッケージへ分ける。

| パッケージ | 持つもの |
|-----------|---------|
| `doctor/check` | `Check` / `Input` / `Result` / `Status` と、`Input` の補助（`Probe` / `ReadFile` / `DialAddr` / `Client`）。**葉のパッケージ**であり、項目もレジストリも import しない |
| `doctor` | `check` の型の別名、レジストリ（`Default`）、並列実行と整列（`Run` / `Sort` / `Replace`）、件数の集計（`Count`） |
| `doctor/authz` | 認証・権限（パーミッション・`hidepid`・トークンスコープ） |
| `doctor/netcheck` | ネットワーク（到達性・プロキシ設定の整合） |
| `doctor/hostres` | 時刻・リソース・障害履歴（NTP・ディスク / inode・メモリ / swap・OOM 履歴） |
| `doctor/jobreq` | docker daemon とジョブ実行の前提（[FR-43](../requirements/functional.md) の 4 点） |
| `doctor/hostcfg` | 依存コマンド・systemd の整合・構成整合（孤児ユニット・ユニット名の不一致・重複ユニット） |

**型を葉に置くのは import の循環を避けるためである。** レジストリは項目を import し、項目は型を import する。型をレジストリと同じパッケージに置くと、この 2 本が逆向きに交わる。呼び出し側（UI）が名指しするのは `doctor` の別名（`doctor.Check` / `doctor.CheckResult`）であり、`doctor/check` を直に import するのは項目の実装だけである。

**項目の型はどのパッケージでも公開しない。** 入口は分類ごとの `Checks() []check.Check` 1 つに絞る。顔ぶれと並びの判断を分類の中だけで完結させるためであり、レジストリ側で個々の型を名指しできる形にすると、並びの規定が 2 箇所に散る。

**`internal/runner` は変更しない。** `systemd.State` は `Restart` も `Environment` も持たないが、それを足すのは runner の検出（3 経路の突き合わせ）の責務であって診断の都合ではない。検出は 3 秒ごとに全 runner ぶん走るので、診断のためだけに `systemctl show` の項目を増やすとポーリングが重くなる。要る値は `doctor/hostcfg` から自分で `systemctl show` を発行して読む。

### `internal/config`

runner 側の設定ファイルの読み書き。

| 要素 | 責務 |
|------|------|
| `EnvFile` | `.env` の表現。**コメントと空行を含む全行を順序どおり保持**し、変更行のみ置換する |
| `LoadEnv` / `SaveEnv` | 読み書き |
| `PathFile` | `.path` の読み書き |
| `DropIn` | systemd drop-in の生成・読み書き |
| `Diff(before, after) string` | 差分の生成（書き込み前のプレビュー用） |
| `Backup(path) error` | 書き込み前のバックアップ作成。所有者とパーミッションを維持 |
| `Validate*` | ラベル・runner 名・パスの検証（**純粋関数**） |

ラベルと runner group の変更は GitHub API 側なので `gh` パッケージに委譲する。

実体は下位パッケージに分かれており、上の名前は `internal/config` が別名として見せる。

| サブパッケージ | 責務 |
|---------------|------|
| `config/fileio` | どう読み書きするか。シンボリックリンクの拒否（`O_NOFOLLOW`）・一時ファイル + rename の原子的な置き換え・**既存ファイルの所有者とパーミッションの引き継ぎ**・`Backup` |
| `config/envfile` | `.env` と `.path` の表現 |
| `config/dropin` | drop-in の表現と配置（`<root>/<unit>.d/override.conf`）。ユニット名は `.service` 由来で runner 実行ユーザーが書き換えられるため、パス区切りを含む名前を拒む |
| `config/apply` | 反映方法（[FR-39](../requirements/functional.md)）。ドレイン再起動は `svc` に無いので `svc.Drain` と `svc.Start` を組み合わせる |
| `config/edit` | 設定項目ごとの変更の組み立て・差分・書き込み・ラベルの API 呼び出し。Config タブから tea に依らない部分を切り出したもの |

**分ける理由は 1 ディレクトリ 2000 行の上限だけではない。** 書き込みの安全策（リンクの拒否・原子的な置き換え・所有者の引き継ぎ）を `fileio` に集めておけば、対象が `.env` / `.path` / drop-in と増えても守り方が分岐しない。

### `internal/gh`

GitHub API とトークンの取得。**GitHub と通信するのはこのパッケージだけである。** ドメイン層は `Client` のメソッド越しにしか API を触らず、`go-github` の型は外へ出さない。

| 要素 | 責務 | 状況 |
|------|------|------|
| `Source` / `Token(ctx, Executor)` | 優先順に従ってトークンを取得（環境変数 `GH_TOKEN` → `SUDO_USER` の `gh` → `gh`）。`Source` は環境変数・実効 UID・`LookPath` を差し替えられる | 実装済み |
| `HasToken(ctx, Executor, timeout) bool` | 取得できるかだけを返す。**値そのものは返さない**（`hostcaps.TokenFunc` として渡す。下記「能力判定」） | 実装済み |
| `Client` / `New` / `WithHTTPClient` / `WithBaseURL` | `go-github` のラッパー。スコープ（repo / org / enterprise）に応じたパスの切り替えを内部で処理する。`WithHTTPClient` / `WithBaseURL` はテスト（`httptest`）とプロキシ環境のための差し替え | 実装済み |
| `RegistrationToken` / `RemoveToken` | 短命トークンの取得。戻りの `ShortToken` は `String()` がマスク済みで、書式指定子で誤って出力しても平文にならない。`Valid(now)` で使い回しの可否を判定する（FR-14） | 実装済み |
| `ListRunners` / `DeleteRunner` | runner 情報の取得と削除。一覧はページングを最後まで辿る | 実装済み |
| `RunnerDownloads` / `PickDownload` / `HostArch` | tarball の URL と SHA-256 の取得と、OS / アーキテクチャに合う 1 件の選択。GitHub の表記（amd64 は `x64`）への読み替えを 1 箇所に置く | 実装済み |
| `LatestRunnerVersion` | runner 本体の最新版。タグの先頭の `v` を落として `bin/runnerversion` と同じ表記に揃える | 実装済み |
| `APIError` | 失敗を「次に何をすればよいか」まで含めて表す（不足スコープ・待機時間・確認コマンドを `Hint` に載せる）。**自動リトライはしない**（レート制限を再消費しないため） | 実装済み |
| `Secrets` | マスク対象の秘密文字列をメモリ上だけで保持する。`command.New` の秘密情報の提供元として渡す（下記） | 実装済み |
| `Labels` 系 | ラベルの取得・置換 | 実装済み（`RunnerLabels` / `ReplaceRunnerLabels`。FR-35 の設定編集で使う）。**追加（POST）と個別削除（DELETE）は未実装**——全量の置き換えで足り、呼び出し元の無い公開 API は置かないため |
| `RunnerGroups` 系 | runner group の一覧と付け替え | 実装済み（`ListRunnerGroups` / `AddRunnerToGroup`。**org / enterprise のみ**で、repo スコープは `ErrNoRunnerGroups`） |
| `TokenScopes` / `Scopes` / `RequiredScope` | 保有スコープの取得と、必要なスコープを満たすかの判定。`X-OAuth-Scopes` を返さないトークン（fine-grained PAT / GitHub App）を「スコープを持たない」と区別する（`Scopes.Classic`）。判定は `admin:x ⊃ write:x ⊃ read:x` の包含を辿る | 実装済み（doctor の「認証・権限」が使う） |

**`Secrets` は `command.New` の契約を満たすために、保持済みの値を複製して返すだけの実装にしてある。** 提供元は `Run` のたびに呼ばれるので並行安全であることと、**外部コマンドを起動しないこと**が要る（起動すると `gh auth token` が無限に再帰する）。`cmd/gsr-helper` が 1 つ作って `command.New` と UI（`page.StateMsg.Setup.Secrets`）の両方へ渡し、`setup/job` が取得した短命トークンをここへ預ける。

### `internal/exec`

外部プロセス実行の唯一の経路。

```go
type Executor interface {
    Run(ctx context.Context, name string, args ...string) (Result, error)
}
```

| 実装 | 用途 |
|------|------|
| 実行実装 | プロセスを起動する。タイムアウトの適用、監査ログの記録、トークンのマスクを行う |
| テスト実装 | 発行されたコマンド列を記録し、あらかじめ設定した結果を返す |

- シェルを経由しない。
- マスク処理はここに実装し、呼び出し側が忘れられない構造にする。

`Executor` は `Run` 1 メソッドに固定する（interface を増やさない規則）。そのため 1 回の実行ごとに変わるパラメータは `ctx` に載せて渡す。

#### 実行ごとのオプション（`Options`）

| フィールド | 用途 |
|-----------|------|
| `Action` | 監査ログの `action`（`svc.stop` / `runner.add` など） |
| `Runner` | 監査ログの `runner`。ホスト全体の操作では空 |
| `Dir` | 作業ディレクトリ。監査ログの `dir` にもこの値を記録する（runner ディレクトリでの `config.sh` 実行に必要） |
| `Env` | 追加の環境変数（`KEY=VALUE`） |
| `SkipAudit` | この実行を監査ログに記録しない指定。**既定は偽（記録する）**。使ってよいのは再検出（`internal/runner/systemd` の `Scan`）とログ追従（`internal/logs` の `Journal`）が発行する読み取り専用コマンドだけ（[セキュリティ設計の監査ログ](../architecture/security.md#記録対象外とする読み取りコマンド)） |

`exec.WithOptions(ctx, o)` で載せ、実行側が `exec.OptionsFrom(ctx)` で取り出す。**監査レコードの `action` と `runner` を埋める経路はこれだけである。** 未設定でもエラーにはせず、`action` が空のレコードとして残る（記録漏れにはしない）。

**子プロセスは親の環境変数をすべて継承する**（`Env` は追加であって置き換えではない）。したがって `GH_TOKEN` を持って起動した場合、`config.sh` を含む全ての子プロセスがそれを受け取る。この点は [セキュリティ設計のトークン露出](../architecture/security.md) の前提である。

#### タイムアウトとプロセスグループ

| 項目 | 値・挙動 |
|------|---------|
| 既定のタイムアウト | 30 秒 |
| `WithTimeout(d)` | `d <= 0` は**黙って無視**して既定値を保つ（0 を「無制限」と読み替えて UI を固めないため） |
| 呼び出し側の deadline | タイムアウトと min を取る。**短くする方向にしか働かない**（呼び出し側の期限をこの層が延ばさない） |
| 期限切れ / キャンセル | 子プロセスグループへ SIGKILL を送る（`Setpgid` で起動しているため `-pid` を指定する）。`./config.sh` や `svc.sh` の孫プロセスが孤児として生き残らない。既に終了していた場合はエラーにしない |
| `WaitDelay` | SIGKILL の後、出力の読み取りを打ち切るまで 5 秒 |

#### 出力の上限

| 対象 | 上限 |
|------|------|
| `Result.Stderr` | 1 MiB まで取り込む。あふれたら**古い先頭を捨てて末尾を残す**（プロセス自体は中断しない） |
| `Result.Stdout` | **上限なし。** `systemctl show` / `list-units` の出力を呼び出し側が解析するため、切ると解析が壊れる |
| 監査レコードの `error` | 4096 バイト（[データモデル](../architecture/data-model.md#error-フィールドに入れるもの)） |

#### 分割したパッケージ

行数上限のため 3 つに分ける。依存は **`exec/command` → `exec`・`exec/mask` の一方向**である。契約（`Executor` / `Result` / `Options`）を最下層に置き、それを実装する側が上に乗る形なので、`exec` はどちらのサブパッケージも import しない。

| パッケージ | 置くもの |
|-----------|---------|
| `exec` | `Executor` / `Result` / `Options` / `WithOptions` / `OptionsFrom` / `LookPath` / テスト実装 |
| `exec/mask` | `Args` / `String` / `Placeholder`。マスクの規則（[セキュリティ設計](../architecture/security.md)） |
| `exec/command` | 実行実装。`Command` / `New` / `NoSecrets` / `WithTimeout` / `WithAudit` / `WithAuditErrorFunc` / `ExitError` / `AuditError` |

実行前のコマンド表示（確認ダイアログのプレビュー）は `mask.Args(args, secrets...)` を直接呼ぶ。`Executor` は `Run` 1 メソッドに固定されており、プレビュー用のメソッドを生やせないためである。**マスクする秘密情報を明示的に渡すこと。**

### `internal/audit`

監査ログの記録。JSON Lines で追記する。呼ぶのは `internal/exec/command`（外部コマンドの実行実装）と `internal/disk`（外部コマンドを伴わないファイル削除。Issue #71）の 2 層に限る。形式とフィールドは [データモデル](../architecture/data-model.md#監査ログjson-lines)。

| 要素 | 責務 |
|------|------|
| `Open(path)` | 出力先を開く。新規作成は `O_EXCL｜O_NOFOLLOW` の後に 0600、**既存ファイルはモードを変えず**、開いた fd 上で検証する（通常ファイル・`Nlink == 1`・所有者が実効 UID・内容が空か先頭が `{`）。検証に落ちたらエラーを返す |
| `Discard()` | 記録しない書き込み先。開けなかった場合の縮退（[`cmd/gsr-helper`](#cmdgsr-helper)） |
| `Logger` | レコードの追記。タイムスタンプは排他区間の内側で採るため、`ts` の順序と行の順序が一致する |
| `Logger.Write(rec) error` | 1 レコードを書く。`internal/exec/command` はこれを使い、失敗を `*command.AuditError` として `WithAuditErrorFunc` へ渡す |
| `Logger.Report(rec)` | `Write` の破壊的操作向けの入口。**戻り値を持たない**。記録の書き込み失敗を呼び出し側の操作の失敗に混ぜないためで、失敗は `WithErrorFunc` の通知先へ渡す（未設定なら標準エラー出力へ 1 行）。`internal/disk` の `removeTarget` が使う |
| `WithErrorFunc(fn)` | `Report` の書き込み失敗の通知先。`command.WithAuditErrorFunc` と同じ役割で、TUI から呼ぶ場合は必ず設定する |

**契約を「`exec` の実行実装だけ」から「外部コマンドと、それに準ずる破壊的操作」へ広げてある。** ファイルの再帰削除は `os.Remove` を直接呼ぶだけで外部コマンドを起動しないため、記録の起点を `Executor` 側の 1 箇所に寄せる形では拾えなかった。契約を広げても記録漏れが構造的に増えないのは、層ごとに記録の起点を 1 関数へ固定しているためである（`internal/exec/command` は `Run`、`internal/disk` は `removeTarget`）。[セキュリティ設計](../architecture/security.md#監査ログ)も参照。

- **親ディレクトリはこのツールが作る場合のみ 0700 にする。** 既存のディレクトリのモードは変えない。`/var/log` や他の所有者のディレクトリのパーミッションを書き換えないためである。したがって「監査ログのディレクトリは 700」は**このツールが作ったディレクトリに限る**保証である（ファイル自体は常に 0600）。
- `O_NOFOLLOW` を使うのは、出力先がシンボリックリンクに差し替えられている場合に辿らないためである。FIFO を指定されても開いたまま止まらない。
- ローテーションは書き込みごとに dev+ino を確認して追従する（[データモデル](../architecture/data-model.md#ローテーション)）。`copytruncate` は不要。
- `WithClock` を渡す場合、その関数は `Logger` をロックした状態で呼ばれる。**`Logger` を再入してはならない。**

#### 記録の失敗の扱い

`Executor.Run` が返すエラーは「コマンド自体が失敗した」ことのみを表す。**監査の書き込み失敗は別経路で通知する。**

| 経路 | 挙動 |
|------|------|
| `command.WithAuditErrorFunc` を設定した場合 | `*command.AuditError`（`Unwrap` で書き込み側のエラー）を渡す |
| 設定しない場合 | 標準エラー出力へ 1 行だけ出す |

**bubbletea の画面を持つ場合は必ずハンドラを設定すること。** 標準エラー出力への書き込みは描画を壊す。`cmd/gsr-helper` はハンドラで件数を集計し、TUI の終了後にまとめて出す。

### `internal/appconfig`

本ツール自身の設定（YAML）の読み書き。

| 要素 | 責務 |
|------|------|
| `Load(path) (Config, error)` | 読み込み。すべての項目に既定値を持たせ、ファイルが無くても動作する。値の検証と起動を止めるエラーは [データモデル](../architecture/data-model.md#検証と既定値) |
| `Save(Config, path) error` | 書き込み。一時ファイル + rename で常に 0600・**`SUDO_USER` の所有権**にする |
| `DefaultPath() (string, error)` | 配置先の決定（`SUDO_USER` を考慮） |
| `Detect(ctx, Executor, Options) Caps` | root / systemd / docker / journalctl / トークンの能力判定（下記） |

`Exists(path) (bool, error)` は設定ファイルの有無を返す。`Load` はファイルが無くても既定値を返すため、戻り値からは初回起動を判別できない。初回設定ウィザード（[FR-41](../requirements/functional.md)）の判定に使う。

**権限などで確認できなかった場合を「無い」に丸めない。** 丸めると、ウィザードが毎回立ち上がったうえで書き込みも失敗し続ける状態を黙って作ることになる。呼び出し側（`cmd`）はエラーを報告したうえでウィザードを出さずに起動する。

なお `Exists` は一度削除されている。**呼び出し元の無い公開 API は置かない**という規則に従い、FR-41 が未実装の間は形が正しいかを確かめる手段が無かったためである。Issue #12 で呼び出し元ができたので、その必要に合わせて戻した。

#### 能力判定（`appconfig/hostcaps`）

判定方法は項目ごとに違う。**すべてを `Executor` に通すわけではない。**

| 項目 | 判定方法 |
|------|---------|
| Root | `os.Geteuid() == 0`。プロセスを起動しない |
| Systemd / Journal | `LookPath` でコマンドの存在を見る。プロセスを起動しない |
| Docker | `Executor.Run` で `docker info --format …` を実行し、値が取れたことをもって daemon 応答とみなす |
| GitHubToken | `Executor.Run` 経由でトークンを取得できたか。判定手段は注入する（下記 `Options`）。`cmd/gsr-helper` が `gh.HasToken` を渡す |

外部プロセスを起動するのは docker とトークンの 2 つだけであり、この 2 つは互いに独立なので並行実行する。

| 上限 | 値 |
|------|-----|
| 1 プローブあたり | 500 ms |
| `Detect` 全体 | 800 ms |

**`Detect` は同期的に呼ぶ。** 全体を 800 ms で打ち切ることで「起動から一覧表示まで 1 秒以内」（[非機能要件](../requirements/non-functional.md#応答性性能)）の内側に収める。構造的には「先に描画してから能力を `Msg` で折り込む」方が望ましいが、それは UI 層の変更を伴うため現時点では採っていない。**時間の保証はこの 800 ms の上限のみである。**

`Options` で呼び出し側が判定を差し替えられる。

**`HasToken` に既定の実装を持たせない**のは、取得の優先順（[セキュリティ設計](../architecture/security.md#取得の優先順)）の実装を `internal/gh` の 1 箇所に閉じるためである。`appconfig` から `gh` への依存を作らずに済み、規定 1 つに対して実装も 1 つに保てる。渡し忘れた起動は「認証されていない」として縮退し、追加・削除・バージョン更新がグレーアウトする。**判定関数は取得した値そのものを返さない。** 判定のためだけに取り出したトークンが `Caps` や画面に載る経路を作らないためである（[セキュリティ設計の「保持と出力」](../architecture/security.md#保持と出力)）。

`TokenFunc` が 1 コマンドあたりの上限（`timeout`）を受け取るのは、判定が取得元を優先順に試すため複数のコマンドを逐次で発行しうるからである。全体の上限（800 ms）だけでは、1 本目が応答しないときに 2 本目を試す余地が無くなる。

| フィールド | 用途 |
|-----------|------|
| `HasToken TokenFunc` | トークン判定。`TokenFunc` は `func(ctx, exec.Executor, timeout time.Duration) bool` で、**`nil` のときはトークン無しとして扱う**（既定の判定を持たない） |
| `Timeout time.Duration` | 1 プローブあたりの上限の上書き。0 以下は既定値（500 ms） |

#### 分割したパッケージ

| パッケージ | 置くもの |
|-----------|---------|
| `appconfig` | `Config` / `Default` / `Load` / `Save` / 検証 |
| `appconfig/confpath` | 配置先の決定、所有者の決定、`SUDO_USER` の検証、所有権付きの原子的な書き込み |
| `appconfig/hostcaps` | 能力判定 |

`Caps` / `Options` / `TokenFunc` / `Detect` / `DefaultPath` / `SudoUser` は `appconfig` の別名として再公開してあるため、呼び出し側は分割を意識しなくてよい。

#### 設定ファイルの配置とパーミッション（`appconfig/confpath`）

| 対象 | 挙動 |
|------|------|
| 設定ファイル | 常に 0600・所有者に合わせる。一時ファイル + rename で置き換える |
| 末端のディレクトリ（名前が `gsr-helper` で、**かつ**所有者のホーム配下） | 作成時 0700。既存でも 0700 に締め直し、所有者に合わせる。root が 0755 で作ったまま残っていると、次に非 root で起動したときに一時ファイルを作れない |
| 作成した中間ディレクトリ（ホーム配下） | `MkdirAll` が 0700 で作り、所有者に合わせる。**既存のディレクトリには触らない**（`~/.config` のような共有の祖先を chown しないため） |
| ホームの外のディレクトリ（`--config /etc/gsr-helper/…` など） | `MkdirAll` 0700 で作るが、**chmod も chown もしない**。既存ディレクトリのモードも変えない |
| 所有者のホームが無い / ディレクトリでない | 所有者を実行ユーザーへ落とす（ホームを持たないサービスアカウントでも書ける）。その実行ユーザーのホームも無い場合はエラーになる |

線引きを「名前」と「ホーム配下であること」の**両方**で行うのは、名前だけで判断すると `--config /etc/gsr-helper/config.yaml` の指定で root が `/etc/gsr-helper` を 0700 にして非特権ユーザー所有へ chown してしまうためである。本ツールへの `sudo` だけを許されたユーザーにとっては権限昇格になる。

chmod は `O_NOFOLLOW｜O_DIRECTORY` で開いた fd に対して行い、chown も `lchown` を使う。`MkdirAll` から所有者変更までの隙にリンクを差し込まれても、root が任意のパスのモードや所有者を書き換えないためである。

### `internal/ui`

bubbletea の Model 群。**内部を Atomic Design で階層化する。** 部品一覧・デザイントークン・画面との対応は [TUI コンポーネント設計](../ui/atomic-design.md) に定める。ここでは階層と責務の対応のみを示す。

| サブパッケージ | 階層 | 責務 |
|--------------|------|------|
| `ui`（`app.go`） | 親 Model | 検出結果・`Caps`・端末サイズ・背景の明暗を保持し、page を切り替える。自動更新（既定 3 秒）の再検出を駆動する。キーの配送を担う（`ctrl+c` のみ親が直接解釈し、他は有効タブへ渡す）。**page の寿命を管理する**（下記） |
| `ui/page` | page | タブ共通の `Msg`（`StateMsg` / `ChromeMsg` / `TabMsg` / `GlobalKeyMsg` / `AttachMsg` / `ModalMsg` / `ResultMsg` / `ActivateMsg` / `DeactivateMsg` / `ShutdownMsg`）、**タブをまたぐ移動の `Msg`**（`OpenTabMsg` と移動先の名前 `TabLogs` / `TabSetup`、用件の `ShowLogMsg` / `SetupRequestMsg`）、モーダルの中身が発行した `Cmd` を中身へ戻す包み（`WrapModal`）、モーダルの重なり（`Overlay`） |
| `ui/page/action` | page | 操作の識別子（`action.ID`）と、可否・理由の判定（`Allow` / `Set`）。依存は `page/action` → `page` の一方向で、`page` からは参照しない |
| `ui/page/<tab>` | page | タブ 1 枚（`tea.Model`）。organism を構成し、キー入力をドメイン層の `tea.Cmd` に変換する。Setup タブ（`ui/page/setup`）が呼ぶのは `internal/setup` と `internal/setup/job` で、GitHub API と tarball はその内側にある |
| `ui/page/runnerdetail` | page | runner の詳細画面。Runners / Jobs が共用するモーダルで、タブではない。依存は `page/runnerdetail` → `page` の一方向 |
| `ui/page/runnerop` | page | runner に対するサービス制御の起点（対象の決定・確認ダイアログ・実行・結果の報告）。Runners / Jobs / 詳細画面が共用し、タブではない。依存は `page/runnerop` → `page` / `page/action` / `page/runnerdetail` / `organism/dialog` / `svc` の一方向 |
| `ui/page/pagetest` | page | `page/<tab>` **と親 Model** が共用するテスト用の道具（共有状態・`Spy`・打鍵の組み立て・`Cmd` の展開と走査（`Msgs` / `ScanKey`）・長寿命の購読を模した `StreamPage`）。**テスト専用で本番からは import しない**（`TestNoProductionCodeImportsPagetest` が本番ファイルの import を読んで検査する） |
| `ui/template` | template | 画面共通の枠（ヘッダ / タブ / 本体 / 状態行 / フッタ、モーダル、2 ペイン）。中身を知らない |
| `ui/organism` | organism | カーソルと選択を持つ対話的な部品（`ChoiceList`）。`tea.Model` は実装せず `bubbles` 流の署名に揃える |
| `ui/organism/table` | organism | 区画に分かれた一覧の共通実装（`bubbles/table` のラッパー） |
| `ui/organism/pane` | organism | スクロールする領域（`Detail` / `Help` / `Log` / `ProgressList`）。`Detail` / `Help` は表示専用、`Log` は追従の ON/OFF とフィルタの入力欄を持つ（ただし一致の判定は持たず、装飾済みの行を受け取るだけである）。`ProgressList` は一括処理の逐次表示と結果報告で、行の状態を決めるのは page 側である |
| `ui/organism/dialog` | organism | 承認・待機・入力のダイアログ（`Confirm` / `DrainWaiter` / `Form`）。`Form` は `huh.Form` のラッパーで、ドメイン層は呼ばず完了・中断を `tea.Msg` で page へ返すだけである。`DiffApproval` は未実装 |
| `ui/molecule` | molecule | 1 区画の描画（ヘッダ・タブ行・フッタ・操作リスト・列の選択）。純粋関数 |
| `ui/molecule/listrow` | molecule | 一覧の 1 行。セル列（`[]string`）を返す。純粋関数。一覧を持つタブが 1 つずつ足す |
| `ui/chrome` | molecule | 本体以外の領域（ヘッダ・タブ行・状態行・フッタ）の中身の組み立て。親 Model の型も bubbletea も知らない純粋関数。import するのは `ui/molecule` / `ui/atom` / `ui/token` だけで、**ドメインの型は受け取らない**（`chrome.View` はバッジの真偽値・件数・`[]molecule.TabView` といった表示用の値のみ）。`Caps` / `Result` / `[]tabset.Tab` からの写し替えは親 Model が行う |
| `ui/tabset` | page | タブのメタ情報と並び。`ui/page/<tab>` を import する唯一の場所 |
| `ui/atom` | atom | 最小の表示単位。純粋関数 |
| `ui/keymap` | keymap | キー定義とヘルプ文言（`bubbles/key.Binding`）。読み手の範囲は [TUI コンポーネント設計の依存の規則](../ui/atomic-design.md#依存の規則) |
| `ui/token` | token | 色・記号・幅。色は背景の明暗で解決し、色を使わない場合の縮退をここに閉じる。`huh.Theme` もここで組み立てる |

- タブ間で共有する状態は親のみが持つ。これを実際に守らせているのは `page/pagetest/import_test.go` の `TestOnlyTabsetImportsTabs` で、`ui/page/<tab>` を import してよいのは `ui/tabset` だけであることを本番ファイルの import から検査する（Go が禁じるのは `page` → `page/<tab>` の循環だけで、タブ同士の参照は止まらない）。**検出（`runner.Discover`）を呼ぶのは親 Model だけで、page は呼ばない。** page は親から配られたスナップショット（`page.StateMsg`）を描画に使う。端末サイズも親が持ち、`template.BodySize` で算出した領域を配る。
- 一覧と確認ダイアログはそれぞれ `organism/table.Model` / `organism/dialog.Confirm` の 1 実装に統一する。個別のダイアログを追加しないことで「確認を経ない破壊的操作の経路を作らない」を構造として守る（`organism/dialog` で未実装なのは `DiffApproval` だけである。[TUI コンポーネント設計の実装状況](../ui/atomic-design.md#実装状況)）。
- 操作の起点は複数あるが（一覧の直接キー / 詳細画面の操作リスト / Jobs タブ、[FR-45〜FR-47](../requirements/functional.md)）、いずれも同じ確認ダイアログを経る。選択肢を並べる UI は `organism.ChoiceList` の 1 実装に統一する。
- **タブをまたぐ移動も親が担う。** Runners / Jobs の `l`（選択中 runner の直近ジョブの Worker ログを開く）は Logs タブへ移って対象を渡すが、タブ同士は互いを import しないため（上記の `TestOnlyTabsetImportsTabs`）、移動元は移動先の型もタブ番号も持てない。そこで移動元は `page.OpenTabMsg{Title: page.TabLogs, Msg: page.ShowLogMsg{...}}` を親へ投げ、親が `[]tabset.Tab` を**名前で**走査して移り、移動先へ用件を配る。この 3 つを `ui/page` に置くのは、**移動元と移動先の双方から見える場所がここしか無い**ためである（`ShowLogMsg` は Logs タブ固有の用件だが、同じ理由でここに置く）。名前は文字列で突き合わせるので、タブ名を変えると移動だけが静かに効かなくなる。`tabset` の `TestOpenTabTitlesMatchTabs` が `page.TabLogs` に対応する有効なタブの実在を検査してこれを防ぐ。一致するタブが無い・無効な場合、親は移動せず理由を状態行に出す（押しても何も起きないキーを作らないため）。
- **page の寿命は親が知らせる。** タブを切り替えるときは離れるタブへ `page.DeactivateMsg`、移動先へ `page.ActivateMsg` を配る（長寿命の購読を張り直させるため）。終了時は有効な全タブへ `page.ShutdownMsg` を配り、各 page が返した後始末の `tea.Cmd` を `tea.Sequence` で `tea.Quit` より**前**に流す（`tea.Batch` では並走して後始末の前に止まりうる）。この契約は `q` / `ctrl+c` の終了でのみ働き、シグナル終了では `Update` を通らないため走らない。
- キーの定義は `ui/keymap` に集約する。可否の判断は `ui/page/action`（`action.Allow` / `action.Set`）が持ち、`atom.KeyHint` は受け取った可否と理由を描くだけとする。**サービス制御の判定は `svc.CanControl` へ委譲済みである。** `action.Allow` が表示層に持つのは「どの操作をドメイン層のどの操作として問うか」の対応（`action.ID` → `svc.Op`）だけで、判定表と理由の文言は `internal/svc` にある。表示層に残る判定は `svc` の関心事ではないもの（GitHub の認証・ジョブ実行中・この版での実装状況）に限る。`?` の全キー一覧は `bubbles/help` に描かせるが、フッタは無効キーをグレーアウトする必要があるため自前で描く。
- キーは最上位のモーダルにのみ配り、入力中（絞り込み・フィルタ・フォーム）はグローバルキーを解釈しない。**この閉じ込めを担うのは page 自身である**（グローバルキーを親へ差し戻さないことで実現する。[TUI コンポーネント設計](../ui/atomic-design.md#キー入力の配送)）。
- `atom` / `molecule` / `template` は bubbletea / bubbles を import しない。

## 主要な interface 一覧

| interface | 定義場所 | 差し替えの目的 |
|-----------|---------|--------------|
| `Executor` | `exec` | テストで発行コマンドを検証する |
| `Check` | `doctor` | 診断項目を独立して追加する |
| `tea.Model` | bubbletea | タブ・モーダルの共通化。独自の interface を作らずに「キーを `Update` で受け、`View` で描く」約束を表す |

interface はこの 3 つに留める。ドメインごとの interface は、差し替えの必要が生じた時点で追加する。

## テストの配置

| 対象 | テストの置き場所 |
|------|---------------|
| `parseConfig` / `parseListUnits` / `parseShow` / `attach` | `internal/runner` の内部テスト。testdata にフィクスチャを置く |
| `scope.Parse` | `internal/runner/scope` のテーブルテスト |
| `normalize`（自前設定の検証） | `internal/appconfig` のテーブルテスト。範囲外・相対パス・`..`・`-` 始まりを網羅 |
| `NextIndex` | `internal/setup` のテーブルテスト |
| `Name` / `Labels` / `Dir` / `URL` / `Count`（入力検証） | `internal/setup/valid` のテーブルテスト。先頭が `-`・`..` を含むパス・予約ラベル・GitHub 以外のホストを網羅。**認証情報を埋め込んだ URL は、解析できるものと解析に失敗するもの（`%` の直後が 16 進でないなど）の両方**を含め、どちらもエラー文言に入力が載らないことを検証する——解析失敗の分岐だけが入力を echo する形の欠陥を再発させないためである |
| tarball の検証と展開 | `internal/setup/tarball`。SHA-256 の不一致、絶対パス・`..`・展開先の外を指すリンクを含む tar、サイズ上限の超過を必ず含める。**一時ディレクトリが成功時・失敗時のどちらでも残らないこと**と、**保持対象へリンクで潜り込む tar**（`ErrPreservedLink`）は、リンクを 1 段辿るだけの tar と、リンクを鎖状に重ねた tar（`d -> .` の下に `d/link -> .runner` を置き `d/link` へ書き込む）の両方を含める——1 段しか見ない実装は前者だけでは落ちない |
| 計画の組み立てと実行 | `internal/setup`。発行コマンド列（トークンの位置がプレースホルダのままであること）、失敗した台で中止して成功分を残すこと、上書きしない名前を網羅 |
| `ValidatePath` | `internal/disk`。異常系（`..`、基準外、リンクによる逸脱、基準自身）を網羅 |
| `Validate*`（ラベル・名前・パス） | `internal/config` のテーブルテスト |
| コマンド発行を伴う処理 | 各ドメインで `Executor` のテスト実装に差し替え、発行コマンド列を検証 |
| マスク処理 | `internal/exec/mask`。キー名ベースと値一致ベースの両方 |
| UI の表示部品（`atom` / `molecule` / `template`） | 各パッケージ。純粋関数として期待文字列と比較する（[TUI コンポーネント設計](../ui/atomic-design.md#テストの配置)） |
| UI の状態を持つ部品（`organism` / `page`） | 各パッケージ。キー入力列に対する状態遷移と発行される `tea.Msg` / `tea.Cmd` を検証。入力モード中にグローバルキーを解釈しないこと、モーダル表示中に背後へキーが流れないことを含める |
| キー定義（`keymap`） | `internal/ui/keymap`。すべてのキーに説明文があること、同一画面でキーが重複していないこと |

## 改訂履歴

| 版 | 日付 | 変更内容 | 変更理由 |
|----|------|---------|---------|
| 1.0 | 2026-08-21 | 新規作成 | 初版 |
| 1.1 | 2026-08-21 | `internal/ui` を Atomic Design の階層構成に置き換え、UI 内部の依存規則とテスト配置を追加 | UI 層の部品分割を [TUI コンポーネント設計](../ui/atomic-design.md) として定義したため |
| 1.2 | 2026-08-21 | `Check` interface に `Startup()` を追加 | 起動時の前提チェック（FR-44）を doctor のレジストリと共通の実装で扱うため |
| 1.3 | 2026-08-21 | 操作の起点が複数でも `Confirm` / `ChoiceList` は 1 実装に統一することを明記 | FR-45〜FR-47 で操作の入口を増やしたため。入口ごとに確認の実装が分かれることを防ぐ |
| 1.4 | 2026-08-21 | `ui/keymap` を追加。organism と `bubbles` 部品の対応、キーの配送、端末サイズと色の所有者、`cmd` での色判定を明記 | キー定義の置き場所と `bubbles` の使い方が仕様として未定義だったため。キーの二重解釈とサイズの渡し忘れを構造で防ぐ |
| 1.5 | 2026-08-22 | 依存関係に `runner --> exec` を追加。systemd ユニットの走査が `Executor` を受けることと `resolveRunAsUser` を責務表に追記 | `runner` が `os/exec` を直接使っていたのを `Executor` 経由に変えたため。ドメイン層が `os/exec` を直接使わない規則に合わせた |
| 1.6 | 2026-08-22 | 依存関係に `appconfig --> exec` を追加。`appconfig` の責務表に `Exists` と能力判定の並行実行を追記 | `appconfig` の能力判定が `Executor` 経由で外部コマンドを発行しており、グラフに依存が無かったため |
| 1.7 | 2026-08-22 | スコープ判定を `internal/runner/scope` として分離。systemd ユニット走査の所要時間とキャンセルの契約、`systemctl show` 失敗ユニットを孤児にしない規則を追記 | `Scope` は GitHub API のパス生成にも使うため、`internal/gh` が `internal/runner` 全体に依存せず参照できる形にした。`show` 失敗ユニットは `WorkingDirectory` が空になるため孤児と誤判定される欠陥があった |
| 1.8 | 2026-08-22 | `exec` の実行オプション・タイムアウト・プロセスグループ・出力上限、`audit` の縮退と記録失敗の通知、`appconfig` の能力判定の上限（800 ms）と設定ファイルの配置規則、`cmd` の終了コードと監査ログの縮退を追記。`exec` / `appconfig` / `runner` のサブパッケージ分割を記載。`attach` が決める値と `list-units` 失敗時の縮退を明記。`cmd` から代替スクリーンと panic 復元の記述を削除 | これらはいずれも実装のみに存在する契約で、仕様からは値も縮退の範囲も読み取れなかった。代替スクリーンは親 Model が宣言し panic 復元は bubbletea が行うため、`cmd` の責務としていた記述が実装と食い違っていた |
| 1.9 | 2026-08-22 | 走査ルートの合成規約（`--root` / `scan_roots` / `SkipDefaultRoots`）と `scanRoots` を追加。`exec.Options` に `SkipAudit` を追記。呼び出し元の無い `appconfig.Exists` と `State.Label()` を削除 | 既定の走査ルートが実ホストのパスを glob するため検証がホストに依存していた。読み取り専用の定期実行が監査ログを埋めていた。呼び出し元の無い公開 API は実際の必要に対して形が正しいかを確かめられない |
| 1.10 | 2026-08-22 | `--refresh` / `--root` が設定ファイルと同じ有効範囲・検査を通すことと、走査ルートの重複除去が入口をまたぐことを明記 | `--refresh` に上限が無く、`--root` が `scan_roots` の絶対パス・`..` 検査を迂回していた |
| 1.11 | 2026-08-22 | 監査ログのクローズ失敗を利用者に報告することを縮退の表に追加 | クローズのエラーを捨てており、監査ログのエラーのうちこれだけが利用者に見えなかった |
| 1.12 | 2026-08-22 | `SkipAudit` を使ってよい範囲を「読み取り専用の定期実行」から「再検出（`Scan`）が発行する読み取り専用コマンド」に改め、参照先の見出しに追随 | 記録対象外の判定基準を発行契機から発行元へ統一したため（[セキュリティ設計](../architecture/security.md#記録対象外とする読み取りコマンド) 1.5） |
| 1.13 | 2026-08-22 | `internal/runner` の責務表から `ScanProcesses` / `ScanUnits` の行を削除し、`Executor` が `nil` のときの縮退を `Discover` の行に統合。所要時間・キャンセルの契約と孤児にしないユニットの記述、下位パッケージの再公開範囲を実装に合わせた | 再公開面を絞って `Discover` を唯一の入口にしたため（Issue #42）。下位パッケージの `Scan` は 3 経路の突き合わせを迂回するため意図的に再公開していないが、仕様書には別名として再公開してあると書かれていた |
| 1.14 | 2026-08-22 | `Result.Stderr` の取り込み上限を「先頭 1 MiB」から「1 MiB まで取り込み、あふれたら古い先頭を捨てて末尾を残す」に訂正 | 実装（`limitedBuffer`）は末尾を残しており、[セキュリティ設計](../architecture/security.md#標準エラー出力の取り込みと抜粋)の表とも記述が食い違っていた |
| 1.15 | 2026-08-22 | `ui/page/action` を階層表に追加し、`ui/page` の責務から可否の判定を外す。タブ共通の `Msg` の列挙に `AttachMsg` / `ModalMsg` / `ResultMsg` / `ActivateMsg` / `DeactivateMsg` / `ShutdownMsg` を追加。親 Model の責務に page の寿命管理（切替時の `Deactivate` / `Activate`、終了時の `Shutdown` と `tea.Sequence` での後始末）を追加。可否の判断が `action.Allow` にある暫定である旨と、`svc` を持ち込む Issue が置き換える範囲を明記 | 可否の判定は Issue #34 で `ui/page/action` へ分離済みだったが、表は `ui/page` の責務のままで新しいパッケージの行も無かった。Issue #26 / #41 が足した 6 つの `Msg` と、親が担うようになった page の寿命管理が本書に反映されていなかった。「page がドメイン層（`svc.CanControl` など）に問い合わせ」は `svc` が存在しない以上そのまま読むと実装できず、暫定であることが読み取れなかった |
| 1.16 | 2026-08-23 | `internal/ui` のサブパッケージ表に `ui/page/runnerdetail`（Runners / Jobs が共用する詳細モーダル）と `ui/page/pagetest`（テスト専用のフィクスチャ）の行を追加。「タブ間で共有する状態は親のみが持つ」の箇条書きに、それを守らせている検査（`TestOnlyTabsetImportsTabs`）を明記 | 表が `ui/page` → `ui/page/action` → `ui/page/<tab>` の 3 行だけで、`page/` 階層が「page + 共通部品 + タブ 1 枚ずつ」だと読めた。[TUI コンポーネント設計](../ui/atomic-design.md)（1.20）が明記した「`page/` は 1 ディレクトリ 1 タブではない」と食い違い、実在する 2 パッケージが本書からは辿れなかった。共有状態の規則も規約としてしか書かれておらず、それを機械的に守らせている検査が本書からは読み取れなかった（PR #67 のレビュー指摘） |
| 1.17 | 2026-08-23 | `ui/page/pagetest` の行を「`page/<tab>` と親 Model が共用するテスト用の道具」に改め、`Msgs` / `ScanKey` / `StreamPage` を挙げた | 表は同パッケージを `page/<tab>` 用のフィクスチャに限定して書いていたが、親 Model 専用の道具（寿命テストの `StreamPage`、Issue #31 で移した打鍵の走査 `ScanKey`）も置かれており、[TUI コンポーネント設計](../ui/atomic-design.md) 側は「タブと親で共用する検証の道具の置き場」と記して親側からの利用を推奨している。2 文書が同じパッケージの守備範囲について別のことを述べていた（Issue #31 の最終ゲート指摘） |
| 1.18 | 2026-08-23 | `internal/disk` の節から「この版で監査ログに残るのは `docker system prune -f` だけである」を削除し、`docker system df`（`disk.df`）も記録されること・記録対象外は再検出の `list-units` / `show` だけであることに訂正。`PruneReclaimable` を関数表に追加し、`prune -f` の解放見込みが内訳の合計ではない理由を追記。`PlanClean` の行と本文に、ジョブ実行中の保護を `Target.Protected` で運び `PlanClean` と `Apply` の両方で弾く構造を追記 | 監査ログの記述が誤っており、[外部インターフェース](../api/external-interfaces.md)の「例外は再検出の `list-units` / `show` のみ」とも正面から矛盾していた。実装は `DockerUsage` が `exec.Options{Action: "disk.df", SkipAudit: false}` で発行しており、`disk.df` も全件記録される。解放見込みは `docker system df` の `Reclaimable` をそのまま使っており、`prune -f` では 1 バイトも消えないボリュームを含んでいた。ジョブ実行中の保護は `Usage` の段階にしか無く、境界の型に可否が無かった |
| 1.19 | 2026-08-23 | `internal/runner` 系の行数を実測へ更新した（1656 / 552 / 426 / 165、残り 344） | 記載値（1558 / 544 / 317、残り 400 行強）は測り直す前のもので、`runner/procs` が 109 行、残余が約 60 行ぶん**多く（危険側に甘く）**表示されていた。この段落は読者に「先に切り出し先を決める」判断を求める箇所であり、余裕の過大表示は分割の判断を誤らせる |
| 1.20 | 2026-08-23 | `internal/logs` の要素表を実装に合わせて更新（`LogFile` → `File`、`List` / `Tail` / `Journal` の戻り値、`Journal` が `Executor` を取ること、`Classify` の追加）。`journalctl -f` を使わず一定間隔の再発行と差分の送出で追従する理由を新設。依存グラフに `Logs --> Exec` を追加。`SkipAudit` を使ってよい範囲にログ追従の `journalctl` を追加 | ログ閲覧を実装した（Issue #9）。`Executor` は 1 回の実行の出力をまとめて返す契約で、`-f` を渡すとタイムアウトまで 1 行も届かない。追従のためだけにストリームの経路を開けると外部プロセスの実行が `Executor` 1 本でなくなり、タイムアウト・監査記録・マスクの適用漏れを構造的に防ぐという `internal/exec` の目的が崩れる。表が `Journal(ctx, unit, out)` としていたのは `Executor` を渡す道が無く実装できない署名だった |
| 1.21 | 2026-08-23 | `internal/ui` のサブパッケージ表を Logs タブ（Issue #9）の実装に合わせた。`ui/organism/pane` の責務に `Log` を追加し、`Detail` / `Help` が表示専用なのに対し `Log` は追従の ON/OFF とフィルタの入力欄を持つ（ただし一致の判定は持たない）ことを明記。`ui/page` の責務のタブ共通 `Msg` の列挙に `OpenTabMsg` / `TabLogs` / `ShowLogMsg` を追加し、タブをまたぐ移動を親が担う仕組み（移動先を名前で指し、親が `[]tabset.Tab` を走査して用件を配る）と、その 3 つを `ui/page` に置く理由・タブ名の不一致を防ぐ検査（`TestOpenTabTitlesMatchTabs`）を箇条書きで新設 | 表は `pane` を「（`Detail` / `Help`）」、`ui/page` の `Msg` を `ShutdownMsg` までと書いており、実装済みの `pane.Log` と `page.OpenTabMsg` / `page.ShowLogMsg` が両方とも漏れていた。[TUI コンポーネント設計](../ui/atomic-design.md)は同じ内容を更新済みで、**同じ事実について 2 文書が食い違う**状態だった。本書はパッケージの責務境界の一覧であり、ここに無い型は「その層に置くと決まっていないもの」と読まれる。とくにタブをまたぐ移動は「タブ同士は互いを import しない」という規約の唯一の抜け道になりうる箇所で、**なぜ `page` に置くのか**が本書に無いと、次のタブが移動を実装するときに移動元へ移動先を直接 import する形を選びかねない |
| 1.22 | 2026-08-23 | 依存関係に `Svc --> Appconf` を追加。`internal/svc` の責務表に `Op` / `Kill` / `Drainer` / 理由の文言を足し、`CanControl` のシグネチャを `CanControl(Op, Runner, Caps)` に訂正。可否の判断が `action.Allow` にある暫定である旨を、`svc.CanControl` へ委譲済みの記述に置き換えた | サービス制御（Issue #5）で `internal/svc` を実装したため。`CanControl` は操作ごとに塞ぐ範囲が違う（[無効な操作の表示](../ui/screens.md#無効な操作の表示)）ので `Runner` と `Caps` だけでは判定できず、仕様書のシグネチャのままでは実装できなかった。`Caps` を引数に取る以上 `appconfig` への依存もグラフに必要で、強制停止（`Kill`）は責務表に行が無かった |
| 1.23 | 2026-08-23 | `internal/ui` のサブパッケージ表に `ui/page/runnerop` と `ui/organism/dialog` の行を追加。`internal/svc` の責務表に `CommandLine` / `Enabled` を追加し、確認ダイアログのコマンド全文が `CommandLine` を出どころとする規約を明記 | サービス制御（Issue #5）で実装したパッケージが本書の階層表から辿れなかった。実行コマンドの提示と実行を別々に組み立てると承認の意味が失われるため、出どころを 1 箇所に定める規約を仕様の側にも残す必要があった |
| 1.24 | 2026-08-23 | `internal/svc` の責務表で `Kill` に「PID もユニット名も無ければ 1 本も発行せず `ErrNoKillTarget` を返す」、`Drain` に「ユニット名が無ければ待機に入らず `ErrNoUnit` を返す」を追記し、理由の文言の行に `ReasonNoCommand` を追加。`CanControl` の行を判定の順（非 root → systemd 不在 → `run.sh` 直起動 → 判定不能）に書き改め、3 段目がドレイン停止も塞ぐ理由と 4 段目が塞がない理由、`ReasonNoCommand` が `CanControl` の返す理由ではなく UI 側が確認ダイアログの手前で使う文言であることを段落で追記。`organism/dialog` を「未実装」と書いていた箇条書き（`Confirm` を 1 実装に統一する規則）を、`Confirm` / `DrainWaiter` は実装済みで未実装は `DiffApproval` / `Form` だけである記述に訂正 | 同じ文書の `internal/ui` のサブパッケージ表（1.19 で更新）が `organism/dialog` を実装済みと書く一方、箇条書きは「未実装」のままで**文書が自分自身と矛盾**しており、リンク先の [TUI コンポーネント設計の実装状況](../ui/atomic-design.md#実装状況) とも食い違っていた。`Kill` / `Drain` の「対象が無ければ発行しない」は本 PR で入れた振る舞いで、書かないと 0 本の実行を成功として報告する実装へ戻りうる。`CanControl` は 3 段目でドレイン停止も塞ぐようになったのに責務表は塞ぐ範囲を挙げておらず、`ReasonNoCommand` に至っては公開定数が本書のどこからも辿れなかった（PR #70 のレビュー指摘） |
| 1.25 | 2026-08-23 | `internal/svc` の `CanControl` の段落を、`run.sh` 直起動（3 段目）と判定不能（4 段目）が**同じ 5 操作**（強制停止以外）を塞ぎ理由の文言だけが違う、という記述に書き改め。責務表の `CanControl` の行にも同じ旨を追記 | 4 段目がドレイン停止を通す仕様は、停止（`x`）が塞がれた runner に対し確認ダイアログ無しで同じ `systemctl stop` を発行させていた（ジョブを持たない runner では `Drainer.Drain` が初回走査で即停止へ抜ける）。3 段目が enable の切替を通す仕様は、`run.sh` 直起動の runner に `systemctl enable` を発行させ [FR-09](../requirements/functional.md) に反していた。「ユニット名は `<dir>/.service` から読めるため停止は成立しうる」という 4 段目の理由付けは、同じ理屈が `x` にも当てはまるのに `x` を塞いでいる事実と矛盾するため撤回した（PR #70 のレビュー指摘） |
| 1.26 | 2026-08-23 | `ui/organism/pane` の行に Logs タブの `Log` を、`ui/organism/dialog` の行に `Confirm` / `DrainWaiter` を併記する形へ統合し、`organism/dialog` を「未実装」と書いていた箇条書きを削除 | Logs / Disk タブとサービス制御が同じ階層へ同時に部品を足したため、両方の記述が揃っていないと`organism/dialog` に何があるのかが本書から辿れなかった（PR #70 のベース追従） |
| 1.27 | 2026-08-23 | runner の追加・削除・バージョン更新（Issue #8）の実装を反映。`internal/setup` の責務表を実際の API（`Plan` / `Unit` / `Step` / `Apply` / `Progress` / `Result`）へ書き直し、短命トークンを計画に載せない構造と `Token` / `TokenFor` の 2 系統を明記。`setup/valid` / `setup/tarball` / `setup/job` を「分割したパッケージ」として追加し、依存グラフの `Setup --> GH` を `SetupJob --> Setup` / `SetupJob --> GH` に訂正、`GH --> Appconf` を追加。UI 層が値の型として `setup.Plan` と `gh.Secrets` を参照するため `UIApp --> Setup` を残し、`Main --> GH` / `UIApp --> GH` を追加。`setup/job` が `setup/tarball` を直に import することを「分割したパッケージ」の依存の向きに追記。`internal/gh` の責務表に状況の列を足し、`Labels` 系と `TokenScopes` が未実装であることと `HasToken` / `APIError` / `Secrets` / `PickDownload` を追記。`hostcaps` の `HasToken` を新しい署名（1 コマンドあたりの上限を取る）と「nil はトークン無し」の規則へ更新。`cmd/gsr-helper` に `gh.Secrets` / `gh.HasToken` の配線を追記。`internal/ui` の表に `ProgressList` / `Form` / `WrapModal` / `TabSetup` / `SetupRequestMsg` を追加。テストの配置に `setup/valid` / `setup/tarball` / `setup` の行を追加 | `internal/setup` の表は `FetchTarball` のように実在しない API を挙げ、`internal/gh` は実装済みと未実装が混在したまま全件が「有る」ように読めた。**依存グラフの `Setup --> GH` は実装と逆で**、`internal/setup` は `gh` を import しない（外部資源を揃えるのは `setup/job` である）。`hostcaps.Options.HasToken` は既定の実装が消えて必須になっており、nil で渡す呼び出しが「既定の判定に落ちる」と読める記述のままだと、認証済みでも追加・削除がグレーアウトする起動を書いてしまう |
| 1.28 | 2026-08-23 | `setup/tarball` の段落に、拒否対象として**保持対象へリンクで潜り込むエントリ**（`ErrPreservedLink`、`keeplink.go`）と、一時ディレクトリ（`.gsr-stage-<乱数>`）へ展開してから `rename` で移す段（`stage.go`）を追記。テストの観点表の「tarball の検証と展開」に、リンクを 1 段辿る tar と**鎖状に重ねた tar** の両方・一時ディレクトリが残らないことを追加し、「入力検証」の行に**認証情報つき URL は解析できるものと解析に失敗するものの両方**を含めることを追加 | [セキュリティ設計](../architecture/security.md) 1.11 と同じ穴が本書にもあった。本書の観点表は各パッケージの**テストが何を必ず含むか**の一次情報であり、`ErrPreservedLink` の検査を挙げないまま「1 段辿る tar」だけを求めると、`resolve()` の要素ごとの走査（鎖状のリンクを潰す部分）を単段の参照へ退化させても検証が緑のままになる。実際そのミューテーションはこの周まで検知されていなかった。同様に URL の行も、解析に失敗する経路だけが入力を echo する形の欠陥を捕まえられなかった（PR #78 の 2 周目レビュー指摘 B2 / C2） |
| 1.29 | 2026-08-23 | 依存グラフに実装にあって描かれていなかった 5 本（`SetupJob --> Exec` / `SetupJob --> Runner` / `SetupJob --> RScope` / `Setup --> RScope` / `GH --> RScope`）を追加した | 1.27 で「依存グラフを実際の import と照合した」と記しながら、`setup/job` が `internal/exec` / `internal/runner` / `internal/runner/scope` を、`internal/setup` と `internal/gh` が `internal/runner/scope` を直に import している事実が落ちていた。**このグラフは §依存の規則 を突き合わせる先の一次情報である**ため、辺の欠落は「その依存は存在しない」と読まれる。とくに `setup/job → exec` は、外部コマンドを `exec` 経由に限定するという規則に**従った**正しい import であるにもかかわらず、グラフに無いことを根拠に規則違反（あるいは循環依存の持ち込み）と判定され、差し戻される側に倒れる。`GH --> RScope` も、`internal/gh` が `internal/runner` 全体ではなくスコープだけを参照するという [`internal/runner/scope`](#internalrunnerscope) の分離理由そのものが、グラフからは裏取りできない状態だった（PR #78 の 3 周目レビュー指摘） |
| 1.30 | 2026-08-23 | doctor（Issue #11）の実装を反映。`Check` の `Run` の戻りを `[]Result` に改め、runner ごとに判定する項目が行を分ける必要があることを理由として明記。`Input` の差し替え口（`Now` / `Dial` / `Getenv` / `LookPath` / `FSRoot` / `NewClient`）を追記。分類ごとの下位パッケージへの分割（`doctor/check` を葉に置く理由・入口を `Checks()` に絞る理由・`internal/runner` を変更せず `systemctl show` を自前で発行する理由）を「パッケージの分割」として新設。`internal/gh` の `TokenScopes` を実装済みへ改め、`Scopes.Classic` による fine-grained PAT の区別と包含関係の判定を追記 | 草案の `Run(ctx, in) CheckResult`（単数）は、runner ごとに 1 行を並べる[画面仕様](../ui/screens.md#doctor-タブ)の TARGET 列と両立しない。単数のまま実装すると、レジストリが検出結果に依存するか TARGET 列を捨てるかのどちらかになる。分割の記述が無いと、次に項目を足す Issue が 1 ディレクトリ 2000 行の上限に当たってから置き場所を考えることになる |
| 1.31 | 2026-08-23 | Issue #71 の実装を反映。依存グラフに `Disk --> Audit` を追加。`internal/audit` の節を「`exec` から呼ばれる」から「`internal/exec/command` と `internal/disk` の 2 層から呼ばれる」へ改め、`Logger.Report` / `WithErrorFunc` を責務表に追加し、契約を広げても記録漏れが増えない理由（層ごとに記録の起点を 1 関数へ固定）を追記。`internal/disk` の節の `Apply` の署名に `lg *audit.Logger` を追加し、「ファイル削除は監査ログに残らない」という記述を「ファイル削除も監査ログに残る」に書き換えて `removeTarget` が記録すること・`command` に `["(削除)", <パス>]` を載せることを明記 | ファイルの再帰削除の監査ログ記録を実装したため。1.18 以前から本節が明記していた「ファイル削除は外部コマンドではないため監査ログに残らない」という欠落が解消されたので、実装と一致するよう更新する必要があった。`internal/disk` が `internal/audit` を新たに import するため、依存グラフの辺も追加しないと § 依存の規則 と食い違う |
