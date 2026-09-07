# gsr-helper

A TUI for operating GitHub Actions self-hosted runner hosts. Add and remove runners, check their state, triage failures, and clean up disk from a single screen.

Targets **Linux + systemd**, and assumes several runners living side by side on one host.

日本語版: [README.ja.md](README.ja.md)

## Why

Running a runner host means the state you need is scattered across three places — files in the runner directory, systemd, and the live processes — and no single command shows all of it. Adding a runner is a manual sequence of `config.sh` → `svc.sh install` → `svc.sh start`. The reasons runners stop are routine (a full disk, clock drift, the OOM killer, inode exhaustion), yet the steps to identify which one applies live in someone's head. gsr-helper puts all of it on one screen.

## What it does

- **Discovery and listing** — finds both systemd-managed runners (those that ran `svc.sh install`) and ones started directly with `run.sh`
- **Add, remove, bulk version update** — add several at once, add one at a time through a wizard, deregister from GitHub, and swap binaries while keeping configuration
- **Service control** — start / stop / restart / enable / disable, plus a drain stop that waits for the running job to finish
- **Log viewing** — live tail of `_diag/Runner_*.log` and `Worker_*.log`, and `journalctl` output
- **Disk breakdown and cleanup** — `_work` / `_tool` / `_temp` / `_diag` / docker usage split out, with staged deletion
- **Diagnostics (doctor)** — reachability, clock drift, OOM history, permissions, required commands, and the prerequisites a job needs (passwordless sudo, docker, buildx, docker group membership)
- **Configuration editing** — `.env`, `.path`, systemd drop-ins, labels, and job hooks

doctor **reports and tells you what to run; it does not change anything.** Editing sudoers, installing packages, and `usermod` are printed as instructions only — a broken sudoers file cannot be repaired with sudo.

## Install

Go is the only build requirement; the result is a single binary that needs no Go at runtime. Either route installs the command as `gsr-helper`.

**From a release**

```sh
go install github.com/ousiassllc/gsr-helper/cmd/gsr-helper@latest
```

**From a source tree**

```sh
make install   # go build -o $(go env GOPATH)/bin/gsr-helper ./cmd/gsr-helper
gsr-helper     # that is all it takes to start
```

The destination is `GOBIN`, or `$(go env GOPATH)/bin` when that is unset (override with `make install INSTALL_DIR=...`). **If that directory is on your `PATH`, typing `gsr-helper` starts it.** If it is not, `make install` tells you to add it. To update, run `make install` again — it overwrites in place. `make uninstall` removes it.

## Usage

```sh
gsr-helper                                  # scan the default roots and start
gsr-helper -root /path/to/actions-runner    # add a scan root (repeatable)
```

| Option | Description |
|--------|-------------|
| `-config <path>` | Path to the configuration file |
| `-root <path>` | Additional scan root (repeatable) |
| `-refresh <seconds>` | Auto-refresh interval (1–3600) |
| `-no-color` | Disable color (`NO_COLOR` is honored too) |
| `-version` | Print the version and exit |

The configuration file lives at `~/.config/gsr-helper/config.yaml`. Every key has a default, so it runs without one.

The audit log defaults to `/var/log/gsr-helper/audit.jsonl`. When it cannot be written, gsr-helper **warns and continues without recording** rather than refusing to start. To keep an audit trail as an unprivileged user, point `audit_log` at a path you can write.

## Development

```sh
make check   # fmt-check / vet / lint / linterly / test
make run     # build and start (make run ARGS="-root /path/to/actions-runner")
```

Git hooks are managed by [lefthook](https://github.com/evilmartians/lefthook): formatting and lint on pre-commit, `make test` on pre-push.

## Documentation

The design documents are written in Japanese.

| Document | Contents |
|----------|----------|
| [docs/overview.md](docs/overview.md) | Purpose, background, scope |
| [docs/requirements/functional.md](docs/requirements/functional.md) | Functional requirements |
| [docs/architecture/overview.md](docs/architecture/overview.md) | Architecture |
| [docs/architecture/security.md](docs/architecture/security.md) | Security design, including what is deliberately not implemented |
| [docs/operations/runner-host-setup.md](docs/operations/runner-host-setup.md) | Setting up a runner host |
| [docs/environment/setup.md](docs/environment/setup.md) | Development environment, CI, lint |
| [docs/ui/screens.md](docs/ui/screens.md) | Screen specifications |

## License

[MIT](LICENSE)
