# 機能要件

対象は Linux + systemd 上の self-hosted runner ホスト。全体像とスコープは [プロジェクト概要](../overview.md) を参照。

## ユースケース

```mermaid
graph TD
    Op[運用者]

    Op --> UC1[runner を増やす]
    Op --> UC2[runner を減らす]
    Op --> UC3[稼働状況を確認する]
    Op --> UC4[ジョブを止めずに停止・再起動する]
    Op --> UC5[不調の原因を切り分ける]
    Op --> UC6[ディスクを空ける]
    Op --> UC7[runner の設定を変える]
    Op --> UC8[runner を最新版にする]

    UC1 --> F_ADD[一括追加 / 個別ウィザード]
    UC2 --> F_DEL[登録解除 + サービス削除]
    UC3 --> F_LST[一覧・ジョブ実行状況]
    UC4 --> F_SVC[サービス制御 + ドレイン停止]
    UC5 --> F_DOC[doctor]
    UC5 --> F_LOG[ログ閲覧]
    UC6 --> F_DSK[ディスク内訳 + クリーンアップ]
    UC7 --> F_CFG[対話型設定編集]
    UC8 --> F_UPD[バージョン一括更新]
```

## 画面遷移

```mermaid
stateDiagram-v2
    [*] --> Runners: 起動
    Runners: Runners（一覧・既定画面）

    Runners --> Jobs: 2
    Runners --> Disk: 3
    Runners --> Logs: 4 / l
    Runners --> Doctor: 5
    Runners --> Config: 6 / e
    Runners --> Setup: 7 / a

    Jobs --> Runners: 1
    Disk --> Runners: 1
    Logs --> Runners: 1 / esc
    Doctor --> Runners: 1
    Config --> Runners: 1 / esc
    Setup --> Runners: 1 / esc

    Runners --> Confirm: s / x / d / r / D
    Confirm --> Runners: 実行 or キャンセル

    Setup --> Progress: 実行
    Progress --> Runners: 完了 / 中止
    Disk --> DryRun: c
    DryRun --> Confirm2: 対象確定
    Confirm2 --> Progress
    Config --> Diff: 入力確定
    Diff --> Apply: 承認
    Apply --> Runners: 反映方法を選択

    Runners --> [*]: q
```

数字キーはタブ直接指定、英字キーは一覧で選択中の runner に対する操作。詳細は [画面・キーバインド仕様](../ui/screens.md)。

## 機能一覧

### 検出・一覧（FR-01〜FR-05）

| ID | 機能 | 内容 |
|----|------|------|
| FR-01 | runner の自動検出 | 既定の設置場所を走査し `.runner` を持つディレクトリを runner として検出する。runner ディレクトリ配下は掘らない（`_work` が巨大になるため） |
| FR-02 | 走査経路の補完 | ディスク走査に加え、稼働プロセスの実行ファイルパスと systemd ユニットの `WorkingDirectory` からも runner ディレクトリを回収する。走査ルート外の runner でも稼働・登録されていれば検出できる |
| FR-03 | 起動方式の判定 | systemd ユニットが対応すれば `systemd`、ユニットが無く `Runner.Listener` が動いていれば `run.sh`（直起動）、どちらも無ければ未稼働と判定する |
| FR-04 | ジョブ実行状況の表示 | `Runner.Worker` の存在をジョブ実行中と判定し、プロセス起動時刻から経過時間を表示する |
| FR-05 | 孤児ユニットの検出 | `actions.runner.*` ユニットのうち対応する runner ディレクトリが見つからないものを異常として一覧に表示する（ディレクトリだけ削除した場合に発生する） |

一覧に表示する項目: runner 名 / スコープ / 起動方式 / サービス状態 / ジョブ実行状況と経過時間 / runner バージョン / `_work` 使用量 / ディレクトリパス。

### サービス制御（FR-06〜FR-09）

| ID | 機能 | 内容 |
|----|------|------|
| FR-06 | 基本操作 | 選択した runner の start / stop / restart / enable / disable を行う |
| FR-07 | ドレイン停止 | `Runner.Worker` が消滅するのを待ってから停止する。**待ち時間は無制限**で、任意のタイミングでキャンセルできる。待機中は経過時間と対象ジョブを表示する |
| FR-08 | 一括操作 | 複数の runner を選択して同一操作を適用する（メンテナンス前の全停止など） |
| FR-09 | 直起動 runner の保護 | `run.sh` 直起動の runner は systemd 操作の対象外とし、該当キーを無効化して理由を表示する |

**ドレイン停止の制約（重要）**

GitHub には「この runner に新規ジョブを割り当てない」API が存在しない。したがって `Runner.Worker` の消滅を待つ方式では、待機中に `Runner.Listener` が次のジョブを拾う可能性が残る。本ツールは Worker の消滅を検知した時点で即座に停止処理へ移るが、**新規ジョブを受け付けないことは保証しない**。この制約は UI 上にも明示する。

将来的な回避策として、GitHub API でラベルを一時退避して実質的な受付停止（cordon）を行う案があるが、キュー中のジョブが待ち続ける副作用があるため初版では採用しない。

### runner の追加（FR-10〜FR-16）

| ID | 機能 | 内容 |
|----|------|------|
| FR-10 | 台数指定の一括追加 | 台数を指定して複数の runner をまとめて登録する |
| FR-11 | 命名規則 | `<ホスト名>-<連番>` とし、連番はホスト内の既存 runner の最大値の次から振る。既定のホスト名部分はフォームで上書きできる |
| FR-12 | 個別ウィザード追加 | 1 台ずつ名前・ラベル・work dir・ephemeral・runner group を個別指定して追加する |
| FR-13 | tarball の取得と検証 | runner の tarball を取得し SHA-256 を検証する。一括追加では 1 回の取得を各ディレクトリへ展開して使い回す |
| FR-14 | 登録トークンの取得 | GitHub API で registration token を取得する。一括追加では有効期限内であれば 1 つのトークンを台数分の登録に使い回す |
| FR-15 | 失敗時の挙動 | 途中で失敗した場合、**その台で中止し、成功済みの runner は残す**。何台目までが成功し、どこで何が失敗したかを実行ログとして提示する |
| FR-16 | 実行前プレビュー | 作成するディレクトリ、実行する `config.sh` / `svc.sh` のコマンド全文を表示し、承認を得てから実行する |

一括追加の流れ:

```mermaid
sequenceDiagram
    actor Op as 運用者
    participant TUI as gsr-helper
    participant GH as GitHub API
    participant FS as ファイルシステム
    participant SD as systemd

    Op->>TUI: 追加台数・スコープ・ラベル等を入力
    TUI->>TUI: 既存 runner から連番の開始値を決定
    TUI->>Op: 実行内容のプレビュー（ディレクトリ・コマンド全文）
    Op->>TUI: 承認
    TUI->>GH: registration token 取得
    TUI->>FS: tarball 取得・SHA-256 検証
    loop 台数分
        TUI->>FS: ディレクトリ作成・tarball 展開
        TUI->>FS: config.sh 実行（登録）
        TUI->>SD: svc.sh install / start
        TUI->>Op: 進捗を表示
    end
    TUI->>Op: 結果一覧（成功 / 失敗した台と理由）
```

### runner の削除（FR-17〜FR-19）

| ID | 機能 | 内容 |
|----|------|------|
| FR-17 | 登録解除とサービス削除 | remove token を取得し、`svc.sh stop` → `svc.sh uninstall` → `config.sh remove` を実行する |
| FR-18 | ディレクトリの保持 | **runner ディレクトリは削除しない。** 削除後、残ったディレクトリのパスを表示して手動削除の判断を委ねる |
| FR-19 | 実行中ジョブの保護 | ジョブ実行中の runner を削除対象に含めた場合は警告し、ドレイン停止を先に行うよう促す |

### バージョン更新（FR-20〜FR-22）

| ID | 機能 | 内容 |
|----|------|------|
| FR-20 | 一括更新 | ホスト内の runner を指定バージョンへまとめて差し替える。対象は全台または選択した台 |
| FR-21 | 設定の保持 | `.runner` / `.credentials` / `.env` / `.path` / `_work` / `_diag` は上書きしない。展開対象は runner 本体のファイルのみ |
| FR-22 | 停止・再開の扱い | 更新前にドレイン停止し、更新後に元の状態（起動していたものだけ）へ戻す |

runner は既定で GitHub による自動更新が有効なため、本機能が主に必要になるのは `disableUpdate` を有効にしている場合である。一覧には現行バージョンと最新バージョンの差を表示する。

### ログ閲覧（FR-23〜FR-26）

| ID | 機能 | 内容 |
|----|------|------|
| FR-23 | ログファイル一覧 | `_diag/Runner_*.log` / `Worker_*.log` を更新時刻順に一覧表示する（サイズ付き） |
| FR-24 | ライブテール | 選択したログを追従表示する。ファイル追加・追記を検知して更新する |
| FR-25 | フィルタとハイライト | 正規表現によるフィルタ、`ERROR` / `WARN` の強調表示 |
| FR-26 | journalctl 参照 | 選択した runner の systemd ユニットのログを同じビューで追従表示する |

「選択中 runner の直近ジョブの Worker ログを開く」ショートカットを用意する。

### ディスク（FR-27〜FR-31）

| ID | 機能 | 内容 |
|----|------|------|
| FR-27 | 使用量の内訳表示 | `_work/<リポジトリ>` 単位、`_work/_tool`、`_work/_temp`、`_diag`、docker（イメージ / コンテナ / ボリューム / ビルドキャッシュ）に分解して表示する |
| FR-28 | 非同期集計 | 集計は UI をブロックせずに進行し、判明した分から順に反映する |
| FR-29 | inode 使用率 | 容量とあわせて inode 使用率を表示する（小さいファイルの大量生成で先に枯渇するため） |
| FR-30 | 段階的クリーンアップ | 削除対象を選択 → ドライラン（対象パスと解放見込み容量）→ 確認 → 実行 の順に進める。確認を経ない削除経路は設けない |
| FR-31 | 実行中ジョブの保護 | ジョブ実行中の runner の `_work` 配下は削除対象から除外する |

クリーンアップの候補: 古い `_work/<リポジトリ>`、`_work/_temp`、古い `_diag` ログ、`docker system prune`、`_work/_tool` の旧バージョン。

### doctor（FR-32〜FR-34）

| ID | 機能 | 内容 |
|----|------|------|
| FR-32 | 定型チェックの実行 | 各チェックを並列に実行し、OK / WARN / FAIL で結果を一覧表示する |
| FR-33 | 対処の提示 | FAIL / WARN の項目には原因の説明と推奨する対処を表示する |
| FR-34 | 再実行 | 対処後に個別または全体を再実行できる |

チェック項目:

| 分類 | 項目 |
|------|------|
| 認証・権限 | `.credentials` / `.runner` のパーミッションと所有者 |
| 認証・権限 | `/proc` の `hidepid` 設定。未設定の場合、runner 登録時にプロセス引数として渡るトークンを他ユーザーから読み取れる（[セキュリティ設計](../architecture/security.md#プロセス引数からのトークン読み取り既知の制約)） |
| 認証・権限 | GitHub トークンの保有スコープ。org レベルの runner 管理には `admin:org` が必要で、`gh auth login` の既定では付与されない（[外部インターフェース](../api/external-interfaces.md#必要なトークンスコープ)） |
| ネットワーク | `github.com:443`、`api.github.com`、`*.actions.githubusercontent.com`、`pkg-containers`、results-receiver への到達性、プロキシ環境変数の整合 |
| 時刻 | NTP 同期状態と時刻ずれ（トークン認証の失敗要因） |
| リソース | ディスク残量、inode 残量、`/tmp` 容量、メモリ、swap |
| 障害履歴 | カーネルログ上の OOM Killer による runner プロセスの停止履歴 |
| docker | daemon の稼働、runner 実行ユーザーの docker グループ所属 |
| 依存コマンド | `git` / `docker` / `node` などの存在とバージョン |
| systemd | ユニットの `Restart` 設定、環境変数、`WorkingDirectory` の整合 |
| 構成整合 | 孤児ユニット（FR-05）、`.runner` と systemd ユニット名の不一致 |

### 対話型設定編集（FR-35〜FR-40）

| ID | 機能 | 内容 |
|----|------|------|
| FR-35 | 設定項目の編集 | `.env` / `.path` / systemd drop-in / ラベル / runner group / job hooks をフォームで編集する |
| FR-36 | 入力検証 | ラベルの文字種・重複・予約ラベル（`self-hosted` / `linux` / `x64`）、work dir の書き込み可否と残容量、runner 名のホスト内重複を検証する |
| FR-37 | 差分プレビュー | 書き込み前に変更内容の差分を表示し、承認を得る |
| FR-38 | バックアップ | 書き込み前に対象ファイルのバックアップを作成する |
| FR-39 | 反映方法の選択 | 「ドレイン再起動」を**既定の提案**とし、「強制再起動（ジョブ中断の警告付き）」「反映しない（次回起動時に有効）」を選べる |
| FR-40 | 設定の複製 | ある runner の `.env` を他の runner へ適用する（対象を複数選択） |

反映コストは項目ごとに異なるため、フォーム上に明示する。

| 設定対象 | 反映方法 | 備考 |
|---------|---------|------|
| `.env` / `.path` | runner 再起動 | — |
| systemd drop-in | `daemon-reload` + 再起動 | — |
| ラベル / runner group | GitHub API で即時反映 | 再起動不要 |
| runner 名 / work dir / ephemeral | **再登録が必要**（`config.sh remove` → 再実行） | 影響が大きいため独立した確認を挟む |

### 自身の設定（FR-41〜FR-42）

| ID | 機能 | 内容 |
|----|------|------|
| FR-41 | 初回設定ウィザード | 設定ファイルが無い場合、初回起動時に走査ルート・ディスク閾値・ポーリング間隔・監査ログ出力先を対話で設定する |
| FR-42 | 設定の再編集 | 上記をいつでも再編集できる |

## 操作フロー（主要な確認フロー）

破壊的操作は共通して次の順序を踏む。確認を省略する経路は設けない。

```mermaid
graph LR
    A[操作の選択] --> B[対象と影響の提示]
    B --> C{ジョブ実行中?}
    C -->|Yes| D[警告・ドレインを促す]
    C -->|No| E[実行コマンド全文の表示]
    D --> E
    E --> F{承認}
    F -->|承認| G[実行・進捗表示]
    F -->|キャンセル| A
    G --> H[結果報告]
    H --> I[監査ログ記録]
```

## 改訂履歴

| 版 | 日付 | 変更内容 | 変更理由 |
|----|------|---------|---------|
| 1.0 | 2026-08-21 | 新規作成 | 初版 |
| 1.1 | 2026-08-21 | doctor に `/proc` の `hidepid` チェックを追加 | セキュリティ設計で、runner 登録時にトークンがプロセス引数として他ユーザーから読める制約が判明したため |
| 1.2 | 2026-08-21 | doctor にトークンスコープのチェックを追加 | org レベルの runner 管理に `admin:org` が必要で、`gh` の既定スコープでは不足することが判明したため |
