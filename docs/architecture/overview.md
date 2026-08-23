# アーキテクチャ設計

機能一覧は [機能要件](../requirements/functional.md)、性能・テスト方針は [非機能要件](../requirements/non-functional.md)、権限とトークンの扱いは [セキュリティ設計](security.md) を参照。

## 全体構成

```mermaid
graph TD
    subgraph cmd[cmd/gsr-helper]
        Main[main<br/>フラグ解析・能力判定・tea.Program 起動]
    end

    subgraph ui[internal/ui - bubbletea 層]
        App[親 Model<br/>検出結果・Caps・タブ管理]
        Tabs[page（各タブ Model）<br/>Runners / Jobs / Disk / Logs / Doctor / Config / Setup]
        Forms[template / organism / molecule / atom / token / keymap<br/>共通レイアウト・部品・スタイル・キー定義]
    end

    subgraph domain[ドメイン層 - bubbletea に依存しない]
        Runner[runner<br/>検出・マージ]
        Svc[svc<br/>サービス制御・drain]
        Disk[disk<br/>集計・クリーンアップ]
        Logs[logs<br/>テール]
        Doctor[doctor<br/>診断チェック]
        Conf[config<br/>設定の読み書き・差分]
        Setup[setup<br/>追加・削除・更新の計画と実行]
        SetupJob[setup/job<br/>外部資源を揃えて実行まで運ぶ]
    end

    subgraph infra[インフラ層]
        Exec[exec<br/>汎用 Executor]
        GH[gh<br/>GitHub API・トークン借用]
        Audit[audit<br/>監査ログ]
    end

    Main --> App
    App --> Tabs
    Tabs --> Forms
    Tabs -.->|tea.Cmd 内で呼ぶ| domain
    App -.->|tea.Cmd 内で呼ぶ| Runner

    Svc --> Exec
    Disk --> Exec
    Doctor --> Exec
    Setup --> Exec
    Setup --> Svc
    SetupJob --> Setup
    SetupJob --> GH
    Conf --> GH
    Exec --> Audit

    Runner -->|ファイル・/proc の直接読み取り| OS[(OS)]
    Exec -->|外部プロセス| OS
```

### 依存の方向

- **ドメイン層は bubbletea を知らない。** ドメインの関数は `context.Context` を取り値を返す普通の関数であり、UI 層がそれを `tea.Cmd` でラップして非同期に実行する。これによりドメインロジックを TUI なしでテストできる。
- **ドメイン層は UI に依存しない。** 進捗の通知が必要な処理はチャネルまたはコールバックで結果を流し、UI 層がそれを `tea.Msg` に変換する。
- **外部プロセスの実行は必ず `exec` 層を経由する。** ドメイン層が `os/exec` を直接呼ぶことを禁止する。監査ログとタイムアウトを漏れなく適用するための制約である。
- **UI 層の内部も階層化する。** `internal/ui` は Atomic Design に沿って token / keymap / atom / molecule / organism / template / page に分け、依存の方向をパッケージの import で強制する。ドメイン層を `tea.Cmd` で呼ぶのは page のみとする（[TUI コンポーネント設計](../ui/atomic-design.md)）。
- **`atom` / `molecule` / `template` は bubbletea / bubbles を import しない。** スクロール・テキスト入力・計時・フォームは既存部品（`bubbles` / `huh`）に委ねるが、それを使うのは organism 以上に限り、下位 3 階層は文字列を返す純粋関数に保つ。`bubbles/key` は例外的に keymap から親 Model・page・organism まで届く（キー定義を 1 箇所に集約した結果であり、`key.Binding` を組み立てるのは keymap だけである）。

## 技術選定

| 選定 | 理由 |
|------|------|
| Go | 単一バイナリで runner ホストに配布でき、`/proc` の読み取りと外部プロセス実行が素直に書ける。runner ホストに追加のランタイムを入れずに済む |
| Bubble Tea | 非同期処理を `Cmd` / `Msg` に分離するモデルが本ツールの中心的な要件（ディスク集計・ログ追従・コマンド実行を UI から切り離す）と一致する |
| huh | 追加ウィザードと設定編集の入力検証・確認ステップを担う。`tea.Model` を実装しているため既存のタブ構成にそのまま組み込める。配色は `token` が組み立てたテーマを渡し、フォームだけ配色が浮かないようにする |
| 代替スクリーンで描画 | 全画面を占有し、終了時に元の端末内容へ戻す。7 タブを切り替える全画面 UI と相性がよく、スクロールバックを汚さない。実行したコマンドの記録は監査ログに残る |
| 汎用 Executor 1 本 | 外部コマンドの抽象化を 1 つの interface に集約する。テストでは発行されたコマンド列を検証でき、監査ログの記録点も 1 箇所に収まる |
| 親 Model が状態を保持 | 検出結果を親が 1 つ持ち各タブへ渡す。同じ検出処理が重複実行されず、タブ間でデータの一貫性が保てる |

## 起動シーケンスと能力判定

権限や依存コマンドの有無による機能の有効・無効判定を各所に散らさないため、**起動時に 1 回だけ能力を判定し、結果を `Caps` として持ち回る**。

```mermaid
sequenceDiagram
    participant Main as main
    participant Caps as 能力判定
    participant App as 親 Model
    participant Tea as tea.Program

    Main->>Main: フラグ解析・設定ファイル読み込み
    Main->>Caps: root か / systemctl / docker / journalctl / gh 認証
    Caps-->>Main: Caps
    Main->>App: Caps・設定・色の有効無効で初期化
    Main->>Tea: tea.NewProgram(App) を起動
    Tea->>App: Init() → 背景色の問い合わせと最初の Tick
    App->>App: 背景色の応答から明暗を決め、配色を解決
    App->>App: Tick を受けて検出 Cmd を発行し、結果を保持して一覧を描画
    Tea->>App: 以降 3 秒ごとの Tick → 再検出 Cmd
```

代替スクリーンは親 Model が宣言し、panic からの端末復元は bubbletea が行う。`main` は `tea.NewProgram(App).Run()` を呼ぶだけである（扱う箇所を 2 つ持つと、片方だけが効いた状態を追えなくなる）。

初回の検出も Tick の経路で行う。**検出を始める場所を 1 つに保つ**ためであり、これによって「実行中の検出があるうちは重ねない」という規則（[画面仕様](../ui/screens.md#一覧の自動更新)）が初回にも効く。能力判定は同期的に行い、全体を 800 ms で打ち切る（[非機能要件](../requirements/non-functional.md#応答性性能)）。

色を使うかどうか（`NO_COLOR` / `--no-color` / 非 TTY）は `main` が判定し、背景の明暗は起動後に端末へ問い合わせて親 Model が保持する。どちらも下位の階層は自分で環境を読まない（[TUI コンポーネント設計](../ui/atomic-design.md#背景の明暗と-no_color)）。

判定する能力:

| 能力 | 判定方法 | 無い場合の縮退 |
|------|---------|--------------|
| root 権限 | 実効 UID | サービス制御・追加・削除・クリーンアップを無効化。一覧とログは読める範囲で表示 |
| systemd | `systemctl` の存在 | サービス制御タブを無効化。`run.sh` 直起動の検出のみ行う |
| docker | `docker` の存在と daemon 応答 | ディスク内訳から docker 項目を除外 |
| journalctl | `journalctl` の存在 | ログタブから journal ビューを除外 |
| GitHub トークン | `gh auth token` の取得成否 | 追加・削除・ラベル操作・最新版確認を無効化 |

UI では無効な操作をグレーアウトし、その理由（「root 権限が必要」「gh が未認証」など）を表示する。

### 終了シーケンス

起動と対になる終了は、**page が持つ長寿命の処理（`journalctl -f` のようなストリーム、監視の goroutine、開いたままのファイル）を畳んでから**ランタイムを止める。

```mermaid
sequenceDiagram
    participant Tea as tea.Program
    participant App as 親 Model
    participant Page as 有効な全タブ

    Tea->>App: q / ctrl+c
    App->>Page: page.ShutdownMsg（有効な全タブへ）
    Page-->>App: 後始末の tea.Cmd
    App-->>Tea: tea.Sequence(後始末, tea.Quit)
    Tea->>Tea: 後始末を流し切ってから終了
```

**束ねるのは `tea.Sequence` であって `tea.Batch` ではない。** `tea.Batch` は並走するため、後始末が実行される前にランタイムが止まりうる。裏のタブにも配るのは、タブ切替時の `page.DeactivateMsg` で畳み損ねた処理をここで確実に閉じるためである。

**シグナル（SIGINT / SIGTERM）による終了ではこの経路を通らない。** bubbletea は `Update` を通さずに畳むため、どの page にも通知は届かない。プロセスが消えても壊れない後始末（子プロセスの回収、監査ログの書き出し）は、この経路に依存させず生成した側（`exec` / `audit`）が保証する。

## 検出のデータフロー

runner の同一性は **runner ディレクトリの実パス**（シンボリックリンク解決後の絶対パス）で判定する。

```mermaid
sequenceDiagram
    participant App as 親 Model
    participant R as runner パッケージ
    participant FS as ファイルシステム
    participant SD as systemd
    participant P as /proc

    App->>R: Discover(ctx, opts)
    R->>P: プロセス走査（Runner.Listener / Runner.Worker）
    P-->>R: PID・実行ファイルパス・起動時刻
    R->>SD: systemctl list-units + show
    SD-->>R: ユニット状態・WorkingDirectory
    R->>FS: 走査ルートから .runner を持つディレクトリを探索
    FS-->>R: runner ディレクトリ一覧
    Note over R: プロセスとユニットからも<br/>ディレクトリを回収（走査ルート外の救出）
    R->>FS: 各ディレクトリの .runner / .service / bin/runnerversion を読む
    R->>R: 実パスをキーに 3 経路をマージ（純粋関数）
    R-->>App: Result（runners / 孤児ユニット / 警告）
```

マージ処理は正規化済みの入力を受け取る純粋関数として実装し、ファイル・プロセス・ユニットの組み合わせをテーブルテストで網羅する。

### 起動方式の判定

| systemd ユニット | Listener プロセス | 判定 |
|---|---|---|
| あり | あり | systemd 管理・稼働中 |
| あり | なし | systemd 管理・停止中 |
| なし | あり | `run.sh` 直起動（systemd 操作の対象外） |
| なし | なし | 未稼働・未登録 |
| あり（対応ディレクトリなし） | — | 孤児ユニット（異常として報告） |

## 非同期処理モデル

重い処理はすべて `tea.Cmd` として UI の外で実行し、結果を `tea.Msg` で受け取る。

```mermaid
sequenceDiagram
    participant U as ユーザー
    participant M as Model.Update
    participant C as tea.Cmd（別 goroutine）
    participant D as ドメイン層

    U->>M: キー入力
    M-->>M: 状態更新（即座に描画）
    M->>C: Cmd を返す
    C->>D: ドメイン関数を実行
    D-->>C: 結果 or エラー
    C-->>M: Msg として通知
    M-->>M: 結果を反映して再描画
```

### 更新の駆動

| 対象 | 方式 |
|------|------|
| runner 一覧 | `tea.Tick` による 3 秒間隔のポーリング（設定で変更可）。手動更新キーも用意する |
| ログ（`_diag` のファイル） | `fsnotify` によるファイル監視。イベントを Msg に変換して反映する。監視するのはファイルではなく**親ディレクトリ**で、ログの入れ替え（新しいファイルの作成）のあとも追従が続くようにする |
| ログ（systemd ユニット） | `journalctl -u <unit> -n <N>` を 2 秒ごとに再発行し、前回との差分だけを Msg にする。`-f` を使わない理由は [コンポーネント設計](../components/overview.md#journalctl--f-を使わない理由) |
| ディスク集計 | 対象ごとに非同期実行し、判明順に Msg を送る |
| 進捗のある処理（追加・更新・クリーンアップ） | 進捗をチャネルで受け、UI 層が Msg に変換する |
| ドレイン停止の待機 | ポーリングで `Runner.Worker` の消滅を監視。`context` のキャンセルで中断できる |

長時間の処理は必ずキャンセル可能にする。`tea.Cmd` に渡す `context` を Model 側で保持し、キャンセルキーで打ち切る。

### 続けて届く結果の受け取り方

ログの追記・集計の判明・一括処理の進捗は、1 回の実行で 1 つの結果を返す処理ではない。これらは **ドメイン層がチャネルを返し、`tea.Cmd` がそこから 1 件受け取って `Msg` に変換し、`Update` が次の `Cmd` を発行する**形で受け取る。

`tea.Program` の参照を UI 層に持たせて goroutine から直接 `Msg` を送る形は採らない。参照を持つと page の単体テストに `tea.Program` が必要になり、「キー入力に対して期待する `Msg` / `Cmd` が出るか」という形のテスト（[テストの配置](../ui/atomic-design.md#テストの配置)）が書けなくなる。

## 外部コマンドの実行

```go
// Executor は外部プロセス実行の唯一の経路。
type Executor interface {
    Run(ctx context.Context, name string, args ...string) (Result, error)
}

type Result struct {
    Stdout   []byte
    Stderr   []byte
    ExitCode int
}
```

- 実装は 2 つ。実プロセスを起動するものと、テスト用に発行コマンドを記録するもの。
- 実装側で**監査ログの記録とタイムアウトの適用を行う**。呼び出し側が忘れられない構造にする。
- 標準入力を必要とする対話的なコマンドは扱わない。`config.sh` は必要な引数をすべて与えて非対話で実行する。

## エラーハンドリング

| 層 | 方針 |
|----|------|
| ドメイン層 | `error` を返す。複数対象を扱う処理は「1 件の失敗で全体を止めず、失敗を集約して返す」 |
| UI 層 | エラーをステータス行に表示し、詳細は専用のパネルで確認できるようにする。エラーで画面遷移を巻き戻さない |
| 起動時 | 設定ファイルの破損など致命的な場合のみ、端末を汚さずにメッセージを出して終了する |
| panic | `tea.Program` の実行を包んで復帰処理を行い、端末状態を復元してからスタックトレースを出力する（TUI が端末を掌握しているため、素の panic は端末を壊す） |

## ディレクトリ構成

```
cmd/gsr-helper/          エントリポイント
internal/
  runner/                検出・マージ・runner ディレクトリの読み取り
  svc/                   サービス制御・ドレイン停止
  setup/                 追加・削除・バージョン更新
  disk/                  使用量集計・クリーンアップ
  logs/                  ログ一覧・テール
  doctor/                診断チェック
  config/                runner 側の設定ファイルの読み書き・差分・バックアップ
  appconfig/             本ツール自身の設定（YAML）と能力判定
  gh/                    GitHub API・トークン借用
  exec/                  汎用 Executor
  audit/                 監査ログ
  ui/                    親 Model・各タブ・フォーム
```

責務とパッケージ間の依存の詳細は [コンポーネント設計](../components/overview.md)。

## 主要な設計判断

| 判断 | 選んだ理由 | 却下した案 |
|------|-----------|-----------|
| runner の同一性をディレクトリ実パスで判定 | 3 つの情報源すべてから導出できる唯一の共通キー。runner 名は変更可能で、agentId は GitHub 側にしか無い | runner 名をキーにする案（改名で追跡できなくなる） |
| プロセス・ユニットからもディレクトリを回収 | 走査ルート外に置かれた runner を取りこぼさない。「動いているのに一覧に出ない」が最も困る失敗 | ディスク走査のみ（設定漏れで検出できない） |
| ドメイン層を bubbletea から独立させる | TUI なしでテストでき、将来 CLI サブコマンドを足す余地も残る | Model の中にロジックを書く（テストが困難になる） |
| 続けて届く結果を `Cmd` の再帰で受ける | `tea.Program` の参照を UI 層に持たず、page のテストが `Msg` / `Cmd` の検証だけで完結する | goroutine から `Program.Send` を呼ぶ（テストに Program が必要になる） |
| キーは最上位のモーダルにのみ配る | 確認中の打鍵が背後の一覧で別の操作として解釈されることを構造的に防ぐ。状態の組み合わせがテストで網羅できる範囲に収まる | グローバルキーだけ背後に通す（確認ダイアログを開いたままタブが切り替わる） |
| 終了時の後始末を `tea.Sequence` で `tea.Quit` の前に流す | page の長寿命の処理が畳まれてからランタイムが止まる。順序が型として表れるため、後から見ても競合の有無を読み取れる | `tea.Batch` で束ねる（後始末と終了が並走し、畳む前に止まりうる）／page 側で同期的に閉じる（終了が後始末の所要時間だけ固まる） |
| 能力判定を起動時 1 回に集約 | 権限・依存コマンドの有無による分岐が各所に散るのを防ぐ | 操作の直前に毎回判定（分岐が散り、挙動が読みにくい） |
| 外部コマンドを Executor 1 本に集約 | 監査ログとタイムアウトの適用漏れを構造的に防ぐ | 各ドメインが `os/exec` を直接呼ぶ（記録漏れが起きる） |
| 削除処理でパス検証を必須化 | root で動作するため、パス誤りの被害が大きい | 呼び出し側の責任にする |

## 改訂履歴

| 版 | 日付 | 変更内容 | 変更理由 |
|----|------|---------|---------|
| 1.0 | 2026-08-21 | 新規作成 | 初版 |
| 1.1 | 2026-08-21 | ディレクトリ構成に appconfig を追加 | コンポーネント設計で本ツール自身の設定と能力判定を担うパッケージを分離したため |
| 1.2 | 2026-08-21 | 代替スクリーンでの起動と背景色の問い合わせを起動シーケンスに追加。続けて届く結果を `Cmd` の再帰で受けることを明記。UI 層の階層に keymap を追加 | ログ追従・集計・進捗の受け取り方が未定義で、`Program.Send` を使う実装だと page のテスト方針が成立しなかったため |
| 1.3 | 2026-08-22 | 起動シーケンスから「代替スクリーンで起動」を外し、代替スクリーンの宣言と panic 復元の担当を明記。初回検出も Tick の経路を通ることと能力判定の上限を追記 | 代替スクリーンは親 Model が宣言し panic 復元は bubbletea が行うため、`main` の責務としていた記述が実装と食い違っていた。`Init` が直接検出せず Tick を返す形にしたことで、検出の二重起動を防ぐ規則が初回にも効くようになった |
| 1.4 | 2026-08-22 | 起動シーケンスに対応する終了シーケンスを追加し、`page.ShutdownMsg` を有効な全タブへ配って後始末を `tea.Sequence` で `tea.Quit` より前に流す契約と、シグナル終了では通らないことを明記。主要な設計判断に同じ行を追加 | 終了の契約は Issue #41 で実装されたが本書には起動シーケンスしかなく、page が長寿命の処理を持つ（Logs / Doctor / Setup）ときにいつ畳まれるかを本書から読み取れなかった。`tea.Batch` との違いは競合そのものであり、選択の理由を残さないと後から等価な変更に見える |
| 1.5 | 2026-08-23 | 更新の駆動の表で「ログ」を `_diag` のファイルと systemd ユニットの 2 行に分け、前者が親ディレクトリを監視すること、後者が `journalctl -f` ではなく 2 秒ごとの再発行と差分で追従することを明記 | ログ閲覧を実装した（Issue #9）。ファイル自身に張った監視はログの入れ替えで古い inode に残り、以後の追記に反応しない。`journalctl -f` は `Executor` の契約（1 回の実行の出力をまとめて返す）と噛み合わず、表の 1 行が 2 つの異なる方式を指していた |
| 1.6 | 2026-08-23 | 全体構成の図でドメイン層に `setup/job` を足し、`Setup --> GH` を `SetupJob --> Setup` / `SetupJob --> GH` へ訂正。`Setup --> Svc`（更新時のドレイン停止）を追記 | runner の追加・削除・バージョン更新を実装した（Issue #8）。`internal/setup` は `gh` を import しない——計画（Plan）を組んで実行するだけで、短命トークンや tarball といった外部資源を揃えるのは `setup/job` である。図が逆向きのままだと、計画の組み立てから API を呼んでよいと読める |
