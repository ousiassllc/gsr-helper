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

# 確認
docker version && docker buildx version
```

### 各手順の注意

| 手順 | 注意 |
|------|------|
| 1. パスワード不要 sudo | `visudo -c` で `parsed OK` を必ず確認する。sudoers を壊すと `sudo` 自体が使えなくなり、その端末からの復旧手段を失う |
| 1. パスワード不要 sudo | `NOPASSWD: ALL` は、この runner で実行される**任意のワークフローに実質 root を与える**ことを意味する。public リポジトリで使う runner では特に危険なため、可能なら必要なコマンドに限定する |
| 2-3. Docker と Buildx | `docker.io` パッケージには buildx が含まれない。`docker-buildx` を明示的に入れる |
| 4. docker グループ | **`usermod` は既存プロセスに反映されない。** runner を再起動しないと、グループに追加済みでも `permission denied` が出続ける |

## ラベル

runner 登録時のラベルに `linux` と `x64` が付いていること。

`runs-on: [self-hosted, linux, x64]` は **3 つのラベルを AND で要求する**。欠けているとジョブはエラーにならず、**無期限に `queued` で止まる**。失敗として通知されないため、最も気付きにくい不備である。

gsr-helper から追加・編集する場合は入力検証で予約ラベル（`self-hosted` / `linux` / `x64`）を確認する（[FR-36](../requirements/functional.md)）。手動で `config.sh` を実行して登録した runner はこの検証を通らないため、登録内容を確認する。

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
