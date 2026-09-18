# Contributing to gaia

gaia is a personal tool with one regular author, which is exactly why this page
exists: everything below is what you would otherwise have to reconstruct from the
code.

## Getting it running

```bash
make build            # into ./bin, stamped with version, commit and time
./bin/gaia --help
```

You need Go 1.27 and, for anything that talks to a model, a local
[Ollama](https://ollama.com) with at least one model pulled:

```bash
ollama pull qwen2.5:latest     # 4.7 GB, enough to answer questions
ollama pull qwen3-coder:30b    # 18 GB, what you want for `gaia agent --write`
```

Then point gaia at it, in `~/.config/gaia/config.yaml`:

```yaml
model: "qwen2.5:latest"
```

Nothing else is required. MemPalace, roles, the cache and the MCP daemon are all
optional and off unless configured.

## The gates

```bash
make check     # what a commit must pass
```

Three things run, cheapest first:

- **`tested-check`** — every package has a test file. A package added without one
  fails here rather than being absorbed as an average by the coverage total.
- **`lint`** — golangci-lint with the `standard` set plus `revive`, `godot`,
  `errorlint` and `depguard`.
- **`cover-check`** — a floor on the total and a floor per package, currently 70%
  and 45%. Raise them when the suite improves; never lower one to get a commit
  through.

`pre-commit run -a` runs those plus gofmt, goimports, semgrep, govulncheck and
commitlint. It is what CI runs; there is no second set of rules.

`make mutation PKG=./some/package` is not part of `check` — a pass recompiles the
package once per mutant, which is too slow for a commit hook. Run it when you have
written tests you believe in: it breaks each line on purpose and tells you whether
a test notices. It has earned its keep twice: ten lines in `execpolicy` the suite
executed and never asserted on, and a `config set agent.allowlist` that accepted a
list and stored a string.

One trap: **a package's name must match its directory.** Outside `--integration`
mode gremlins finds which tests to run from the file path, so a mismatch means it
runs none and reports every mutant as surviving. `plugins/config` scored 10%
against a suite that killed 19 of 20 by hand, purely because it was called
`configplugin`. Per-package baselines are in `.gremlins.yaml`.

## How the code is laid out

- `kernel/` bootstraps the app and resolves which plugins are on. It knows the
  `Plugin` interface and no plugin by name — `depguard` enforces that.
- `plugins/<name>/` is one directory per feature.
- `plugins/shared/` is what every plugin may use. It is a leaf: it imports nothing
  from `gaia/`, so depending on it can never make a cycle.
- `plugins/shared/execpolicy/` decides whether a command a model proposed may run.
  Read it before touching anything that executes.
- `config/` loads settings and the per-repository trust store, before the kernel
  resolves anything.
- `bdd/` holds acceptance scenarios in Gherkin, driving the real CLI in-process.

## Adding a plugin

1. `plugins/<name>/plugin.go`, implementing `kernel.Plugin`: `ID`,
   `DefaultEnabled`, `DependsOn`, `ConfigSchema`, `Register`, `MCPTools`.
2. Prefix every config key with the plugin ID — `myplug.setting`, or `myplug.*`
   for a subtree. An undeclared key is refused, so declaring it is not optional.
3. Register it in `plugins/registry.go`.
4. Write the test file. `tested-check` will tell you if you forget.

Tools returned by `MCPTools()` are served by `gaia serve` for **enabled** plugins
only, so disabling a plugin takes its tools off the MCP surface too.

## Conventions worth knowing before your first change

- **A command that fails prints its reason with `shared.Fail` and returns the
  error.** Never `PrintError` as a return value: it reports whether the write
  succeeded, so the shell would see success.
- **Commands never go through a shell.** Use `execpolicy`. What a model writes is
  data, never a program.
- **Comments are one line.** If something needs a paragraph, it belongs in the
  commit message or in the docs, where it is read once by someone looking for it.
- **Commits are conventional** (`feat:`, `fix:`, `refactor:`…) — commitlint
  enforces it, and semantic-release reads it to decide the next version.

## Testing

Tests are named for the behaviour they protect, not the function they call:
`TestReadingOutsideTheProjectNeedsConfirmation`, not `TestRunCommand`. A test
that needs a comment to explain why it exists usually needs a better name first.

Nothing in the suite contacts a model or the network. `ask.Provider` is an
interface, `Store.callTool` and `mempalace.callToolFn` are function fields, and
HTTP is stubbed with `httptest` — every one of those is a seam put there so a
test can stand where a process would be.

## What not to do without asking

- Release: tags, `goreleaser`, anything touching `GH_TOKEN` or the release job.
- Add a dependency. Say why, and what else was considered.
- Widen what `execpolicy` allows, or what the agent may write.
- Broaden CI permissions.
