# 画面・キーバインド仕様

機能との対応は [機能要件](../requirements/functional.md)、Model の構成は [コンポーネント設計](../components/overview.md) を参照。本書で定める表示を構成する部品の階層と配置は [TUI コンポーネント設計](atomic-design.md) を参照。

## 設計原則

1. **有効なキーを常に画面に出す。** フッターに現在の画面で使えるキーを表示する。
2. **無効な操作は隠さずグレーアウトし、理由を示す。** 「なぜ押せないのか」が分かる状態を保つ（例: `run.sh 直起動のため systemd 操作は不可`）。
3. **小文字は安全側、大文字は影響が大きい側。** `x`（停止）に対して `X`（強制停止）、`d`（ドレイン停止）に対して `D`（削除）。
4. **色に依存しない。** 状態は記号でも判別できるようにし、`NO_COLOR` を尊重する。
5. **破壊的操作は影響を表示して `y/N`（既定 N）。** Enter の連打では進まない。

## 共通レイアウト

```
┌────────────────────────────────────────────────────────────────────────────┐
│ gsr-helper   host: build01   root   disk: 82% ⚠   gh: ousiass              │ ← ヘッダ
│ [1]Runners [2]Jobs [3]Disk [4]Logs [5]Doctor [6]Config [7]Setup            │ ← タブ
├────────────────────────────────────────────────────────────────────────────┤
│                                                                            │
│                            （本体）                                        │
│                                                                            │
├────────────────────────────────────────────────────────────────────────────┤
│ ⚠ 孤児ユニット 1 件 / 警告 2 件                          選択: 2 件        │ ← 状態行
│ s:開始 x:停止 X:強制 d:ドレイン D:削除 n:追加 u:更新 e:設定 l:ログ ?:ヘルプ │ ← フッタ
└────────────────────────────────────────────────────────────────────────────┘
```

ヘッダには能力判定の結果を出す。`root` が無い場合は `read-only`、`gh` が未認証なら `gh: 未認証` と表示し、依存する操作が使えないことを示す。

## Runners タブ

既定画面。ホスト上の runner 一覧。

```
  NAME          SCOPE        MANAGED   SVC       JOB          VERSION   _WORK
▸ build01-1     org:foo      systemd   ● active  ▶ 4m12s      2.311.0   31.2G
  build01-2     org:foo      systemd   ● active  idle         2.311.0    8.1G
  build01-3     foo/bar      run.sh    -         idle         2.309.0    2.0G  ⚠
  build01-4     org:foo      systemd   ○ inactive idle        2.311.0    0.4G
  ─ 孤児ユニット ────────────────────────────────────────────────────────────
  actions.runner.foo-bar.old01.service   ✗ failed   （対応ディレクトリなし）
```

| 記号 | 意味 |
|------|------|
| `▸` | カーソル位置 |
| `[x]` / `[ ]` | 複数選択の状態（選択モード時のみ表示） |
| `● active` | サービス稼働中 |
| `○ inactive` | サービス停止中 |
| `✗ failed` | サービス異常終了 |
| `-` | systemd ユニットなし |
| `▶ 4m12s` | ジョブ実行中と経過時間 |
| `idle` | ジョブなし |
| `⚠` | 行に注意事項あり（古いバージョン、権限異常など。詳細は Enter） |

孤児ユニット（FR-05）は一覧の下部に別区画で表示する。

### 端末幅による列の省略

幅が足りない場合、次の順に列を落とす。

1. `_WORK`
2. `VERSION`
3. `MANAGED`
4. `SCOPE`

`NAME` / `SVC` / `JOB` は常に表示する。幅 60 未満では表示不能である旨を出す。

## Jobs タブ

このホストで実行中のジョブを runner 横断で一覧する（[FR-04](../requirements/functional.md) の表示形態）。

```
  RUNNER        REPOSITORY          ELAPSED   WORKER PID   _work
▸ build01-1     foo/bar             4m12s     284193       /opt/runners/build01-1/_work/bar
  build01-7     foo/baz             22m03s    291044       /opt/runners/build01-7/_work/baz
```

実行中ジョブが無い場合はその旨を表示する。`l` で該当ジョブの Worker ログへ直接移動する。

## Disk タブ

```
  ファイルシステム  /               使用 82% (410G/500G)  inode 34%  ⚠ 警告閾値超過

  TARGET                          SIZE      FILES    PATH
▸ [ ] build01-1 / _work/bar       24.1G     412,003  /opt/runners/build01-1/_work/bar
  [ ] build01-1 / _work/_tool      5.8G      88,201  /opt/runners/build01-1/_work/_tool
  [x] build01-1 / _work/_temp      1.2G      12,004  /opt/runners/build01-1/_work/_temp
  [ ] build01-1 / _diag           102.4M      1,208  /opt/runners/build01-1/_diag
  [x] docker / build cache        12.4G           -  -
  [ ] docker / dangling images     3.1G           -  -
      build01-2 / _work/bar       （集計中…）              ← ジョブ実行中のため削除不可

  選択合計: 13.6G
```

- 集計は非同期で、判明した行から順に埋まる（[FR-28](../requirements/functional.md)）。
- ジョブ実行中の runner の `_work` は選択できず、理由を行末に表示する（[FR-31](../requirements/functional.md)）。

`c` を押すとドライラン画面へ進む。

```
 クリーンアップの確認

 削除対象:
   /opt/runners/build01-1/_work/_temp            1.2G   12,004 ファイル
   docker build cache                           12.4G

 解放見込み: 13.6G

 実行するコマンド:
   （ファイル削除: 上記パスの再帰削除。シンボリックリンクは辿りません）
   docker builder prune -f

 削除したファイルは復元できません。実行しますか? [y/N]
```

## Logs タブ

```
 build01-1  Worker_20260821-120433-utc.log        追従: ON   フィルタ: ERROR|WARN
├────────────────────────────────────────────────────────────────────────────┤
│ [2026-08-21 12:04:35Z INFO  Worker] Job started                            │
│ [2026-08-21 12:05:01Z WARN  StepRunner] Step timeout approaching           │
│ [2026-08-21 12:05:44Z ERROR JobRunner] Process completed with exit code 1  │
```

- ファイル一覧と本文の 2 ペイン。`tab` でペイン間を移動する。
- `journalctl` のビューも同じ画面で扱い、`J` で切り替える。
- 追従が ON の間は末尾に自動スクロールする。手動でスクロールすると追従が OFF になり、`G` で再開する。

## Doctor タブ

```
  診断結果   OK 14   WARN 2   FAIL 1   SKIP 1              最終実行: 12:06:20

  STATUS  CATEGORY      CHECK                            TARGET
  ✓ OK    ネットワーク  api.github.com:443 到達           -
  ✗ FAIL  時刻          NTP 未同期（ずれ 42 秒）          -
  ⚠ WARN  認証・権限    トークンに admin:org がない       -
  ⚠ WARN  リソース      / の使用率 82%                   -
  ✓ OK    docker        daemon 応答                      -
  ⊘ SKIP  docker        使用量取得（docker が無い）        -
▸ ✗ FAIL  障害履歴      OOM による停止履歴あり            build01-3
```

`Enter` で詳細を開く。

```
 ✗ FAIL   時刻 / NTP 未同期

 検出内容:
   systemd-timesyncd が同期していません。基準時刻とのずれは 42 秒です。

 影響:
   runner のトークン認証が失敗し、runner がオフラインになる場合があります。

 推奨する対処:
   timedatectl set-ntp true
   systemctl restart systemd-timesyncd
```

対処コマンドは表示のみで、本ツールからは実行しない（診断の責務を超えるため）。

## Config タブ

対象 runner を選び、編集項目を選ぶ。入力は `huh` のフォームで行う。

```
 build01-1 の設定

▸ .env（環境変数・プロキシ・job hooks）        12 項目
  .path                                       /usr/local/bin:/usr/bin:...
  systemd drop-in                             Restart=always, MemoryMax=未設定
  ラベル                                      self-hosted, linux, x64, gpu
  runner group                                Default
  ─────────────────────────────────────────────────────────────────────────
  名前 / work dir / ephemeral                 ⚠ 変更には再登録が必要
  ─────────────────────────────────────────────────────────────────────────
  この設定を他の runner に複製
```

入力を確定すると差分を表示する。

```
 変更内容の確認   /opt/runners/build01-1/.env

   PATH=/usr/local/bin:/usr/bin:/bin
 - https_proxy=http://old-proxy:3128
 + https_proxy=http://new-proxy:3128
 + ACTIONS_RUNNER_HOOK_JOB_COMPLETED=/opt/hooks/cleanup.sh
   LANG=ja_JP.UTF-8

 バックアップ: /opt/runners/build01-1/.env.bak

 この変更を書き込みますか? [y/N]
```

書き込み後に反映方法を選ぶ（既定はドレイン再起動）。

```
 反映方法を選んでください

▸ ドレイン再起動（実行中ジョブの完了を待って再起動）        ← 既定
  強制再起動（⚠ 実行中のジョブは中断されます）
  反映しない（次回起動時に有効）
```

## Setup タブ

### 追加

```
 runner の追加

▸ 台数を指定して一括追加
  1 台ずつ個別に設定して追加
  バージョンを一括更新
```

一括追加のフォーム入力後、実行前プレビューを表示する（[FR-16](../requirements/functional.md)）。

```
 実行前の確認   3 台を追加します

 作成するディレクトリ:
   /opt/runners/build01-5
   /opt/runners/build01-6
   /opt/runners/build01-7

 各ディレクトリで実行するコマンド（build01-5 の例）:
   ./config.sh --url https://github.com/orgs/foo --token *** \
     --name build01-5 --labels self-hosted,linux,x64,gpu \
     --work _work --runnergroup Default --unattended
   ./svc.sh install
   ./svc.sh start

 runner バージョン: 2.311.0（SHA-256 を検証してから展開します）

 ⚠ 登録時、トークンはプロセス引数として渡ります。同一ホストの他プロセスから
   読み取れる可能性があるため、ジョブ実行中の登録は避けてください。
   現在ジョブ実行中の runner: build01-1

 実行しますか? [y/N]
```

実行中は進捗を表示し、失敗した時点で中止して結果を報告する。

```
 追加中… 2/3

 ✓ build01-5   登録・サービス起動 完了
 ▶ build01-6   config.sh 実行中…
   build01-7   待機

 ─────────────────────────────────────────────────────────────────
 ✗ build01-6 で失敗しました（config.sh 終了コード 1）
   Http response code: Forbidden from 'POST https://api.github.com/...'

 完了: 1 台（build01-5）
 未実行: 1 台（build01-7）
 build01-5 はそのまま残っています。
```

### 削除

```
 runner の削除   2 台

 対象:
   build01-3   foo/bar    idle
   build01-4   org:foo    idle

 実行する操作:
   GitHub からの登録解除（config.sh remove）
   systemd サービスの削除（svc.sh uninstall）

 runner ディレクトリは削除されません:
   /opt/runners/build01-3
   /opt/runners/build01-4

 実行しますか? [y/N]
```

## キーマップ

### グローバル

| キー | 動作 |
|------|------|
| `1`〜`7` | タブの直接選択 |
| `tab` / `shift+tab` | 次 / 前のタブ |
| `r` | 手動で再読み込み |
| `?` | ヘルプ（全キーの一覧） |
| `q` / `ctrl+c` | 終了 |
| `esc` | 選択のクリア / 1 つ前の状態へ戻る |

### 一覧（Runners / Jobs / Disk / Doctor）

| キー | 動作 |
|------|------|
| `j` / `↓` | 下へ |
| `k` / `↑` | 上へ |
| `g` / `G` | 先頭 / 末尾 |
| `ctrl+f` / `ctrl+b` | ページ送り |
| `space` | 選択のトグル |
| `ctrl+a` | 全選択 |
| `/` | 絞り込み |
| `enter` | 詳細を開く |

### Runners タブの操作

| キー | 動作 | 影響 |
|------|------|------|
| `s` | 開始 | 安全 |
| `x` | 停止（サービス停止） | 実行中ジョブに影響する可能性 |
| `X` | **強制停止** | ⚠ 実行中ジョブを中断 |
| `d` | ドレイン停止（ジョブ完了を待つ） | 安全 |
| `R` | 再起動 | 実行中ジョブに影響する可能性 |
| `E` | enable / disable の切り替え | 安全 |
| `n` | runner を追加 | — |
| `D` | **runner を削除**（登録解除 + サービス削除） | ⚠ 登録解除 |
| `u` | バージョン更新 | ドレイン停止を伴う |
| `e` | 設定を編集 | 反映方法を選択 |
| `l` | ログを開く | 安全 |

`x` / `X` / `R` / `D` / `u` は確認ダイアログを経る。`d`（ドレイン）は待機の開始のみなので確認は不要とし、待機中はキャンセルできる。

### ドレイン待機中

```
 ドレイン停止中   build01-1

 実行中のジョブの完了を待っています…   経過 6m41s
 対象ジョブ: foo/bar（Worker PID 284193、開始から 11m02s）

 ⚠ 待機中も新しいジョブを受け付ける可能性があります
   （GitHub に受付停止の API がないため）

 esc: 待機をキャンセル
```

待ち時間は無制限（[FR-07](../requirements/functional.md)）。制約を画面上に明示する。

### Logs タブ

| キー | 動作 |
|------|------|
| `tab` | ファイル一覧 / 本文の切り替え |
| `j` / `k` / `ctrl+f` / `ctrl+b` | スクロール |
| `G` | 末尾へ移動して追従を再開 |
| `f` | 追従の ON / OFF |
| `/` | フィルタ（正規表現） |
| `J` | journalctl ビューへ切り替え |

### Disk タブ

| キー | 動作 |
|------|------|
| `space` | クリーンアップ対象の選択 |
| `c` | 選択した対象のドライランへ進む |
| `r` | 再集計 |

### Doctor タブ

| キー | 動作 |
|------|------|
| `enter` | 詳細（検出内容・影響・推奨対処） |
| `r` | 全項目を再実行 |

### 確認ダイアログ

| キー | 動作 |
|------|------|
| `y` | 実行 |
| `n` / `esc` / `enter` | キャンセル（既定） |

`enter` はキャンセル側に割り当てる。連続操作の勢いで実行されることを防ぐ。

## 無効な操作の表示

操作できない場合はキーを消さず、グレーアウトして理由を示す。

| 状況 | 表示 |
|------|------|
| 非 root | `s/x/X/R/D/n/u: root 権限が必要です（sudo で起動してください）` |
| `run.sh` 直起動の runner | `s/x/R: systemd 管理外のため操作できません` |
| systemd が無い | `サービス制御は利用できません（systemctl が見つかりません）` |
| `gh` 未認証 | `n/D/u: GitHub の認証が必要です（gh auth login）` |
| スコープ不足 | `n/D: org レベルの操作には admin:org が必要です（gh auth refresh -h github.com -s admin:org）` |
| ジョブ実行中 | `D: ジョブ実行中です。先に d でドレイン停止してください` |

## 改訂履歴

| 版 | 日付 | 変更内容 | 変更理由 |
|----|------|---------|---------|
| 1.0 | 2026-08-21 | 新規作成 | 初版 |
| 1.1 | 2026-08-21 | TUI コンポーネント設計への参照を追加 | 画面を構成する部品の階層を別文書に定義したため |
