# ランナーホストのセットアップ

新しい runner ホストを用意するときに **ホスト側で必要な作業**をまとめる。2 台目以降はこの手順をそのまま実行すればよい。

ここに挙げた項目は gsr-helper の doctor が「ジョブ実行の前提」として検査する（[FR-43 / FR-44](../requirements/functional.md)）。**本ツールは検出と手順の提示までを行い、ホストの変更は行わない**（[セキュリティ設計](../architecture/security.md#パスワード不要-sudo-の要求への対応)）。

## 前提

- Linux + systemd、`apt` 系ディストリビューション
- `<runner-user>` は runner を実行するユーザー（`svc.sh install [user]` で指定した、または `systemctl show -p User` で確認できるユーザー）

## 手順

```bash
# 1. パスワード不要の sudo
#    ariga/setup-atlas が sudo install で /usr/local/bin へ atlas を置くため必須
echo '<runner-user> ALL=(ALL) NOPASSWD: ALL' | sudo tee /etc/sudoers.d/github-runner
sudo chmod 0440 /etc/sudoers.d/github-runner
sudo visudo -c        # parsed OK を確認（壊すと sudo で復旧できなくなる）

# 2-3. Docker と Buildx
#      docker.io 単体では buildx が入らず COPY --chmod=644 が失敗する
sudo apt-get update && sudo apt-get install -y docker.io docker-buildx

# 4. docker グループ
sudo usermod -aG docker <runner-user>
sudo systemctl restart 'actions.runner.*'   # 既存プロセスには反映されないので必須

# 5. C コンパイラ
#    make test は go test -race で走り、競合検出は cgo（= gcc）を必要とする
sudo apt-get install -y build-essential

# 確認
docker version && docker buildx version && gcc --version
```

### 各手順の注意

| 手順 | 注意 |
|------|------|
| 1. パスワード不要 sudo | `visudo -c` で `parsed OK` を必ず確認する。sudoers を壊すと `sudo` 自体が使えなくなり、その端末からの復旧手段を失う |
| 1. パスワード不要 sudo | `NOPASSWD: ALL` は、この runner で実行される**任意のワークフローに実質 root を与える**ことを意味する。public リポジトリで使う runner では特に危険なため、可能なら必要なコマンドに限定する |
| 2-3. Docker と Buildx | `docker.io` パッケージには buildx が含まれない。`docker-buildx` を明示的に入れる |
| 4. docker グループ | **`usermod` は既存プロセスに反映されない。** runner を再起動しないと、グループに追加済みでも `permission denied` が出続ける |
| 5. C コンパイラ | `actions/setup-go` は Go ツールチェーンだけを導入し、**C コンパイラは入れない**。欠けていると `make test`（= `go test -race ./...`）が `-race requires cgo` で失敗する（[C コンパイラ](#c-コンパイラ)） |

## ラベル

runner 登録時のラベルに `linux` と `x64` が付いていること。

`runs-on: [self-hosted, linux, x64]` は **3 つのラベルを AND で要求する**。欠けているとジョブはエラーにならず、**無期限に `queued` で止まる**。失敗として通知されないため、最も気付きにくい不備である。

gsr-helper から追加・編集する場合は入力検証で予約ラベル（`self-hosted` / `linux` / `x64`）を確認する（[FR-36](../requirements/functional.md)）。手動で `config.sh` を実行して登録した runner はこの検証を通らないため、登録内容を確認する。

## runner group の対象リポジトリ

org（`ousiassllc`）レベルに登録した runner は、**runner group の対象リポジトリを限定する**。同じ runner を掴めるリポジトリを絞り、別リポジトリのワークフローがこのホスト上で実行されないようにするためである（[環境構築 / fork からの PR で self-hosted ジョブを起動しない](../environment/setup.md#fork-からの-pr-で-self-hosted-ジョブを起動しない)）。

| 項目 | 設定 |
|------|------|
| 対象リポジトリ | 「選択したリポジトリ」にし、runner を使うリポジトリだけを明示的に追加する |
| public リポジトリへの提供 | 既定（提供しない）のまま変更しない |

- 設定場所は org の Settings → Actions → Runner groups である。
- **この設定の確認・変更には org 管理者の権限が必要**で、API から触るには `admin:org` スコープが要る（実測: `gh api orgs/ousiassllc/actions/runner-groups` が 403 `You must be an org admin or have the runners and runner groups fine-grained permission.`）。CI から機械的に検証できないため、**runner を追加・移動したときに手動で確認する**。
- **public リポジトリへ runner を提供する設定にはしない。** 既定のままであれば、リポジトリを public に戻した時点でジョブは実行されず `queued` で止まる。これは fork PR 経由で第三者のコードが runner 上で走ることを防ぐ層として機能する（実測: 対象リポジトリを public のまま運用していたとき、org に 12 台登録・1 台稼働の状態で run が 15 分以上 `queued` のまま引き取られず、private 化した直後に同じ run が実行された）。

## C コンパイラ

**`gcc` をホストに入れる**（導入は[手順](#手順)の 5 に含む）。CI の `test` ジョブは `make test`（= `go test -race ./...`）を実行し、**競合検出は cgo を必要とする**ため C コンパイラが無いとジョブが失敗する（実測: `CGO_ENABLED=0 go test -race` が `-race requires cgo` で失敗する）。`actions/setup-go` は Go ツールチェーンだけを導入し、C コンパイラは入れない。

## 言語ツールチェーン

**ホストへの事前インストールは不要。** Go / Node / Terraform / Atlas はいずれも各 `setup-*` アクションがジョブ実行時に取得する。

## 欠けているものと症状

| 欠けているもの | 出るエラー（代表的なメッセージ） |
|--------------|--------------------------|
| ラベル（`linux` / `x64`） | **エラーなし。** ジョブが無期限に `queued` のまま止まる |
| NOPASSWD sudo | `sudo: パスワードが必要です`（`sudo: a password is required`） |
| Docker | `docker: command not found` |
| Buildx | `the --chmod option requires BuildKit`（`COPY --chmod` を含む Dockerfile のビルド時） |
| docker グループ | `permission denied while trying to connect to the Docker daemon socket` |
| C コンパイラ（`gcc`） | `-race requires cgo; enable cgo by setting CGO_ENABLED=1`（`make test` 実行時） |

## doctor での検出

| 項目 | 判定方法 | Status |
|------|---------|--------|
| パスワード不要 sudo | `sudo -l -U <runner-user>` の `NOPASSWD` | WARN（必要性はワークフローによるため FAIL にしない） |
| `docker` | コマンドの存在と `docker info` | FAIL |
| `docker buildx` | `docker buildx version` | WARN |
| docker グループ所属 | `/etc/group` と、稼働中 `Runner.Listener` の `/proc/<pid>/status` の `Groups` | FAIL（未反映の場合は「要 runner 再起動」として区別） |

これらは起動時にも自動判定し、不備があれば状態行に警告を出す（[FR-44](../requirements/functional.md)、[画面仕様](../ui/screens.md#共通レイアウト)）。

## 出典

この手順は別リポジトリの仕様書 `spec/content/docs/operations/github-actions-runner.md` の「ランナーホストのセットアップ」に記録されている内容を、gsr-helper の doctor の検査項目と対応付けて転記したものである。

## 改訂履歴

| 版 | 日付 | 変更内容 | 変更理由 |
|----|------|---------|---------|
| 1.0 | 2026-08-21 | 新規作成 | 初版。doctor のジョブ実行の前提チェック（FR-43）の根拠となる実運用の手順を記録 |
| 1.1 | 2026-08-22 | 「runner group の対象リポジトリ」節を追加し、対象リポジトリの限定と public リポジトリへ提供しない既定を維持する運用を明記 | self-hosted runner を掴めるリポジトリを絞ることが fork PR ガードの一次防御の 1 層であるにもかかわらず、手順として記録されていなかったため（Issue #17）。設定の確認には `admin:org` スコープが必要で CI から機械的に検証できないため、手動確認のタイミングもあわせて明記した |
| 1.2 | 2026-08-22 | 「C コンパイラ」節を追加し、冒頭の「手順」ブロックに `build-essential` の導入（手順 5）と `gcc --version` の確認を追加、「各手順の注意」と「欠けているものと症状」にも対応する行を追記 | CI の `test` ジョブが `make test`（= `go test -race ./...`）を実行するようになり、競合検出は cgo を必要とするため C コンパイラがホストの前提に加わった。`actions/setup-go` は C コンパイラを導入しないため、欠けているとジョブが `-race requires cgo` で失敗する（Issue #21） |
