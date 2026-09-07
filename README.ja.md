# gsr-helper

English: [README.md](README.md)

GitHub Actions の self-hosted runner ホストを管理する TUI。runner の増減・稼働確認・障害切り分け・ディスク掃除を 1 画面で完結させる。

対象は **Linux + systemd**。1 ホストに複数の runner を並べる構成を想定している。

## なぜあるか

runner ホストの運用は、状態が 3 か所（runner ディレクトリのファイル・systemd・稼働プロセス）に分散していて 1 コマンドで全体像が出ない。増減は `config.sh` → `svc.sh install` → `svc.sh start` の手順作業になり、停止要因（ディスク満杯・時刻ずれ・OOM・inode 枯渇）は定型なのに切り分けが属人化する。gsr-helper はこれを 1 つの画面に集約する。

## できること

- **検出と一覧** — systemd 管理（`svc.sh install` 済み）と `run.sh` 直起動の両方を検出する
- **追加 / 削除 / 一括バージョン更新** — 台数指定の一括追加、ウィザードでの個別追加、GitHub からの登録解除、設定を保持したままの差し替え
- **サービス制御** — start / stop / restart / enable / disable と、ジョブ完了を待つドレイン停止
- **ログ閲覧** — `_diag/Runner_*.log` / `Worker_*.log` のライブテールと `journalctl` 参照
- **ディスク内訳とクリーンアップ** — `_work` / `_tool` / `_temp` / `_diag` / docker の分解表示と段階的な削除
- **環境診断（doctor）** — 到達性・時刻ずれ・OOM 履歴・パーミッション・依存コマンド・ジョブ実行の前提（パスワード不要 sudo / docker / buildx / docker グループ）
- **設定編集** — `.env` / `.path` / systemd drop-in / ラベル / job hooks

doctor は**検出と手順の提示までを行い、変更は実行しない**。sudoers の書き換え・パッケージの導入・`usermod` は提示するだけである（sudoers を壊すと sudo 自体で復旧できなくなるため）。

## インストール

ビルドに必要なのは Go だけ。実行は単一バイナリなので Go は要らない。コマンド名はどちらの手順でも `gsr-helper` になる。

**リリースを直接入れる**

```sh
go install github.com/ousiassllc/gsr-helper/cmd/gsr-helper@latest
```

**開発ツリーから入れる**

```sh
make install   # go build -o $(go env GOPATH)/bin/gsr-helper ./cmd/gsr-helper
gsr-helper     # これだけで起動する
```

置き先は `GOBIN`、無ければ `$(go env GOPATH)/bin`（`make install INSTALL_DIR=...` で変えられる）。**そこが `PATH` にあれば `gsr-helper` と打つだけで起動する**。無い場合は `make install` が「`PATH` に追加すると起動できます」と案内を出す。更新は `make install` を打ち直すだけで、同じ場所に上書きされる。削除は `make uninstall`。

**root で動かす**

runner を実運用するには root が要る。`svc.sh install` が systemd ユニットを作り、既定の導入先が `/opt/runners` だからである。ところが **`sudo` は呼び出し側の `PATH` を引き継がない**——`secure_path` だけを探すため、`GOBIN` や `$(go env GOPATH)/bin` に置いたバイナリは root から見えず、`sudo gsr-helper` は「コマンドが見つかりません」になる。`secure_path` が探すディレクトリへ置く。

```sh
make install-system   # sudo install -m 0755 gsr-helper /usr/local/bin/gsr-helper
sudo gsr-helper
```

ビルドは呼び出したユーザーのままで、配置だけを昇格するので root 所有のビルドキャッシュを作らない。配置先は `SYSTEM_DIR=...`、昇格の省略は `SUDO=` で変えられる。削除は `make uninstall-system`。

root でなくても起動し、読める範囲は表示する。できないのは導入とサービス制御である。

## 使い方

```sh
gsr-helper                                  # 既定のルートを走査して起動
gsr-helper -root /path/to/actions-runner    # 走査ルートを追加（複数指定可）
```

| オプション | 説明 |
|-----------|------|
| `-config <path>` | 設定ファイルのパス |
| `-root <path>` | 追加の走査ルート（複数指定可） |
| `-refresh <秒>` | 自動更新間隔（1〜3600） |
| `-no-color` | 色を使わない（`NO_COLOR` も尊重する） |
| `-version` | バージョンを表示して終了する |

設定ファイルは `~/.config/gsr-helper/config.yaml`。全項目に既定値があり、ファイルが無くても動く。

監査ログの既定は `/var/log/gsr-helper/audit.jsonl` で、書き込めない場合は**警告して記録なしで続行する**（起動は妨げない）。sudo で起動すれば警告は出ない。一般ユーザーで記録も残したいときは、設定の `audit_log` を書き込めるパスに変える。

## 開発

```sh
make check   # fmt-check / vet / lint / linterly / test
make run     # ビルドして起動（make run ARGS="-root /path/to/actions-runner"）
```

Git Hooks は [lefthook](https://github.com/evilmartians/lefthook) で、pre-commit に整形と lint、pre-push に `make test` が掛かる。詳細は [docs/environment/setup.md](docs/environment/setup.md) を参照。

## ドキュメント

| 文書 | 内容 |
|------|------|
| [docs/overview.md](docs/overview.md) | 目的・背景・スコープ |
| [docs/requirements/functional.md](docs/requirements/functional.md) | 機能要件 |
| [docs/architecture/overview.md](docs/architecture/overview.md) | アーキテクチャ |
| [docs/architecture/security.md](docs/architecture/security.md) | セキュリティ設計（実装しないことの境界を含む） |
| [docs/operations/runner-host-setup.md](docs/operations/runner-host-setup.md) | ランナーホストのセットアップ |
| [docs/environment/setup.md](docs/environment/setup.md) | 開発環境・CI・lint |
| [docs/ui/screens.md](docs/ui/screens.md) | 画面仕様 |

## ライセンス

[MIT](LICENSE)
