# climan

A unified, **declarative** way to install, update and manage popular developer
CLIs. Declare what you want in `climan.yaml`; climan reconciles your system
against it — with a scriptable CLI **and** a full-screen TUI.

```sh
climan init     # scaffold climan.yaml
climan install  # install everything declared in the manifest
climan update   # update everything that is installed
climan tui      # graphical management
```

## Install

The module resolves through the `go.solved.gg` vanity domain (Cloudflare
Worker serving `go-import` metadata, backed by the GitHub repository):

```sh
go install go.solved.gg/climan@latest
```

Or download a binary from the
[GitHub Releases](https://github.com/solvedggorg/climan/releases) page.

## Supported tools

| Tool | Category | Install method |
| --- | --- | --- |
| [mise](https://mise.jdx.dev) | version manager | `curl https://mise.run \| sh`; updates via `mise self-update` |
| [asdf](https://asdf-vm.com) | version manager | git clone to `~/.asdf` |
| [sdkman](https://sdkman.io) | version manager | `get.sdkman.io` installer (CI mode, rc files untouched) |
| [flyctl](https://fly.io/docs/flyctl) | cloud | `fly.io/install.sh` → `~/.fly/bin/flyctl` |
| [doctl](https://docs.digitalocean.com/reference/doctl) | cloud | GitHub release binary → `bin_dir` |
| [grok](https://x.ai/cli) | AI coding | `x.ai/cli/install.sh` |
| [codex](https://developers.openai.com/codex) | AI coding | `chatgpt.com/codex/install.sh`, npm fallback |
| [claude](https://code.claude.com) | AI coding | `claude.ai/install.sh`, npm fallback |

Install methods are captured from each project's official documentation and
live in [`internal/registry`](internal/registry/registry.go) as declarative
data, so adding a new tool is a small change in one file.

## The manifest

`climan.yaml` is the source of truth:

```yaml
version: 1
bin_dir: ~/.local/bin
tools:
  - name: mise
    version: latest
  - name: claude
    version: latest
```

`climan init` writes a manifest declaring every supported tool.
`climan add <tool> [--version X]` and `climan remove <tool>` edit it
(remove also uninstalls by default; `--keep-manifest` keeps the entry).

## CLI reference

| Command | Description |
| --- | --- |
| `climan init [--force]` | scaffold a `climan.yaml` |
| `climan add <tool> [--version X] [--no-install]` | declare a tool (and install it) |
| `climan install [tool...]` | install missing tools from the manifest |
| `climan update [tool...]` | update installed tools |
| `climan remove <tool> [--keep-manifest]` | uninstall a tool and drop it from the manifest |
| `climan list` / `ls` / `status` | table of tools: state, version, location |
| `climan list --all` | include tools not in the manifest |
| `climan doctor` | check prerequisites and diagnose tool setup |
| `climan tui` | full-screen graphical manager |
| `climan version` | print version |

Global flags: `--config <path>`, `--dry-run`, `--verbose`, `--yes`,
`--bin-dir <path>`.

## TUI

`climan tui` shows every tool with its live status:

```
climan  6 installed · 2 missing · 8 total    ◐ updating claude…
▸ ● mise       v2026.7.16   [version-manager]  installed
  ○ asdf       —            [version-manager]  not installed
  ● flyctl     v0.4.109     [cloud]            installed
  ◉ codex      v0.59.1      [ai]               update available
```

| Key | Action |
| --- | --- |
| `j`/`k`, `↑`/`↓` | navigate |
| `enter` / `i` | install if missing, else update |
| `u` | update selected |
| `d` (`d` again) | remove selected |
| `a` | install all missing |
| `U` | update all installed |
| `/` | filter by name / category |
| `t` | toggle manifest ⇄ full catalog |
| `r` | refresh statuses |
| `q` / `esc` / `ctrl+c` | quit |

Installer output streams live into the log pane at the bottom.

## Development

```sh
nix develop        # Go 1.25 + golangci-lint + goimports (see flake.nix)
make test          # unit tests
make lint          # golangci-lint
make build         # ./bin/climan
make install       # ~/.local/bin/climan
```

### CI / releases

- `.github/workflows/ci.yml` — gofmt, golangci-lint v2, `go vet`, race-enabled
  tests on Go 1.25/1.26, cross-compile checks and a binary smoke test, on
  every push and PR.
- `.github/workflows/release.yml` — push a `v*` tag to build static binaries
  for linux/darwin/windows × amd64/arm64, package them with LICENSE/README
  and checksums, and publish a GitHub Release with auto-generated notes.
- `.github/dependabot.yml` — weekly updates for Go modules and actions.

Layout:

```
main.go                    entry point
internal/registry/         declarative catalog of tools + install methods
internal/config/           climan.yaml manifest loading/validation
internal/installer/        install / update / remove / status engine
internal/system/           shell exec, downloads, path helpers
internal/cli/              scriptable cobra commands
internal/tui/              bubbletea TUI
```

## Roadmap

- [ ] Latest-version checks (GitHub API) for script-based tools → honest "update available" state
- [ ] Per-tool shell activation setup (`mise activate`, asdf/sdkman rc sourcing)
- [ ] Pinned-version verification (compare installed vs manifest version)
- [ ] Windows PowerShell support for script installers
- [ ] `climan export` / `climan diff` for manifests across machines
