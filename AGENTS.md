# AGENTS.md

How to work on **gaia**. This file is read by agents working on this repository,
including gaia's own — `gaia agent` quotes the first part of it into the model's
prompt, so what matters is at the top and the whole thing stays short.

## Checks

```bash
make check   # what a commit must pass: tested-check, lint, cover-check
make test    # the whole suite
make build   # into ./bin, stamped with version and commit
```

`pre-commit run -a` runs those plus gofmt, goimports, semgrep and govulncheck. It is
what CI runs; there is no second set of rules.

A change is done when `make check` passes. A test you did not run is not a test that
passed.

## Where things live

- `kernel/` — bootstraps the app, resolves which plugins are on, registers commands.
  It knows the `Plugin` interface and no plugin by name; `depguard` enforces that.
- `plugins/<name>/` — one directory per feature. Each implements `kernel.Plugin`.
- `plugins/shared/` — helpers every plugin may use. A leaf: it imports nothing from
  `gaia/`, so that depending on it can never make a cycle.
- `plugins/shared/execpolicy/` — decides whether a command a model proposed may run,
  and runs it without a shell.
- `config/` — loading, validation, and the per-repository trust store. Loaded before
  the kernel resolves anything, so it does not depend on the kernel.
- `roles/` — system prompts as YAML.
- `bdd/` — acceptance scenarios in Gherkin, driving the real CLI in-process.

## Conventions

- Idiomatic Go, small functions, explicit errors wrapped with context.
- Config keys are namespaced by plugin (`ask.model`) and declared in that plugin's
  `ConfigSchema()`. An undeclared key is refused, so declaring it is not optional.
- A command that fails prints its reason with `shared.Fail` and returns the error, so
  the shell sees a non-zero exit. Never `PrintError` as a return value.
- Commands never go through a shell. Use `execpolicy`.
- Every package has a test file — `tested-check` fails the commit otherwise.
- Coverage floors are in the `Makefile`. Raise them deliberately; never lower one to
  get a commit through.

## Adding a plugin

1. `plugins/<name>/plugin.go` implementing `ID`, `DefaultEnabled`, `DependsOn`,
   `ConfigSchema`, `Register`, `MCPTools`.
2. Prefix every config key with the plugin ID.
3. Register it in `plugins/registry.go`.
4. Write the test file.

Tools returned by `MCPTools()` are served by `gaia serve` for enabled plugins only.

## Not without being asked

- Releasing: tags, `goreleaser`, anything touching `GH_TOKEN` or the release job.
- Adding a dependency. Say why, and what else was considered.
- Widening what `execpolicy` allows, or what the agent may write.
- Broadening CI permissions.

## Handing off

Say which checks you ran and what they said. Summarise by file. Name what you could
not verify — an answer that says what it is unsure of is worth more than one that
sounds certain.
