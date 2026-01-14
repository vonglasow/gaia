# AGENTS.md

Agent guide for the **gaia** repository.

**Purpose**: Enable agents to work effectively in this Go-based CLI project while honoring repo conventions, CI expectations, and user preferences.
**When to read**: At task initialization, before major decisions, and whenever requirements shift.
**Concurrency reality**: Assume other contributors may land commits mid-run; refresh context (`git status`, `git diff`, CI config) before summarizing or editing.

---

## Purpose & Scope

- **Goal:** Keep the Go CLI clean, reproducible, and aligned with the repo’s automation (pre-commit, golangci-lint, CI).
- **Scope:** Applies to everything in this repo unless a deeper `AGENTS.md` overrides it.
- **Safety:** Avoid destructive or risky operations by default (e.g., publishing releases, pushing tags, modifying release secrets).

---

## Quick Obligations

| Situation               | Required action                                                                                                     |
| ----------------------- | ------------------------------------------------------------------------------------------------------------------- |
| Starting a task         | Read this guide end-to-end, then check fresh user/system instructions and CI workflow (`.github/workflows/ci.yml`). |
| Before committing       | Run `pre-commit run -a` and fix all failures (unless explicitly told otherwise).                                    |
| Go changes              | Keep code idiomatic, run `go test -v ./...`, and avoid introducing unnecessary dependencies.                        |
| Linting                 | Ensure `golangci-lint` + pre-commit rules pass (CI runs both).                                                      |
| Release-related changes | Do **not** trigger releases, tag pushes, or modify secrets-related config unless explicitly requested.              |
| Long commands           | If a command exceeds **5 minutes**, stop it, capture logs/output, and mention the timeout before retrying.          |
| Dependencies            | Do not add or upgrade dependencies without confirming fit, maintenance, and security posture.                       |
| Final handoff           | Summarize changes by file, mention checks run, note uncertainties/TODOs.                                            |

---

## Repo Overview (High Confidence)

This is a Go CLI project with a small, clear structure (from the repo root):

- `.github/` — GitHub Actions workflows (CI + release automation)
- `api/` — API interaction and streaming functionality
- `commands/` — CLI command definitions
- `config/` — configuration management
- `main.go` — application entry point
- `.pre-commit-config.yaml` — formatting/lint automation
- `.goreleaser.yaml` — release packaging config
- `go.mod` / `go.sum` — Go modules

(If new directories appear, do not assume they are wired into CI without checking.)

---

## Source of Truth: CI + pre-commit

### CI expectations

The CI workflow runs on push + PR and enforces:

- Go version: **1.24** (matrix)
- `golangci-lint` via GitHub Action
- `pre-commit-ci/lite-action`
- tests: `go test -v ./...`
- release job only on `main`, using semantic-release + goreleaser (requires token)

### pre-commit expectations

Formatting and other checks are defined in `.pre-commit-config.yaml`.

Default command:

```bash
pre-commit run -a
```

Do not invent alternative linters/formatters unless the user asks.

---

## Non-Negotiable Safety Rules

### Forbidden by default (unless explicitly granted)

- Triggering releases (manual tag pushes, running goreleaser publish flows)
- Modifying secrets, tokens, or release permissions in CI
- Introducing remote calls that may exfiltrate data (especially from config files)
- Adding telemetry, analytics, or network reporting without explicit consent

### Allowed by default

- Local builds and tests
- Linting and formatting via pre-commit / golangci-lint
- Refactoring, bug fixes, and feature work that stays within the CLI scope

---

## Development Workflow

### Standard dev commands (use these by default)

```bash
# Run all repo checks (recommended)
pre-commit run -a

# Run tests (CI does this)
go test -v ./...

# Local build
go build ./...
```

### Linting

CI runs `golangci-lint`. Prefer to run it locally if available:

```bash
golangci-lint run ./...
```

But if golangci-lint is not installed locally, rely on `pre-commit run -a` and keep changes small.

### Formatting

Prefer Go’s built-in formatting conventions:

```bash
gofmt -w .
```

However, if pre-commit provides formatting hooks, **let pre-commit be the final arbiter**.

---

## Coding Style (Go)

- Prefer clear, minimal, idiomatic Go.
- Avoid clever abstractions, especially in CLI wiring.
- Keep error handling explicit and meaningful (wrap errors with context).
- Avoid global state unless unavoidable; pass config/context explicitly.
- Prefer small functions with obvious responsibilities.
- Keep `commands/` focused on CLI wiring; core logic should live in the appropriate package (often `api/` or `config/`).

### Dependency philosophy

- Don’t add dependencies casually.
- Prefer well-maintained packages, widely adopted in Go ecosystem.
- Confirm with the user before adding a new dependency.

---

## Configuration & UX Expectations

From README-level behavior:

- Config lives in `~/.config/gaia/config.yaml` by default.
- CLI supports multiple roles/modes (`default`, `describe`, `shell`, `code`) and subcommands (`ask`, `chat`, `config`, etc.).

When modifying config behavior:

- Preserve backwards compatibility if possible.
- Avoid breaking existing YAML fields; if changes are needed, add migration or fallback behavior.
- Keep default values stable unless explicitly requested.

---

## GitHub Actions / Release Automation

This repo uses:

- semantic-release action (Go semantic release) + goreleaser hooks for releases
- release only on `main` and requires a token (`secrets.GH_TOKEN`)

When touching `.github/workflows/ci.yml` or `.goreleaser.yaml`:

- Be extra conservative.
- Avoid changing permissions scope or secrets usage.
- Don’t broaden release triggers.
- If you must change anything, explain the risk clearly in the handoff.

---

## Git Hygiene

- Keep commits focused and descriptive.
- Don’t rewrite others’ history.
- If a large formatting change is required because of tooling, call it out explicitly.

---

## Communication Preferences

- Be concise and direct.
- Humor is optional, keep it dry if used.
- When unsure, say so and propose options.

---

## Final Handoff Checklist

Before finishing:

1. Confirm what checks were run:
   - At minimum: `pre-commit run -a`
   - If Go code changed: `go test -v ./...`
2. Summarize changes by file (and ideally key sections).
3. Call out TODOs / risks / uncertainties.
4. Confirm that no release triggers or secrets changes were performed.

---

## Suggested “Safe Defaults” for Typical Tasks

### Bug fix / small feature

1. Implement change with minimal surface area.
2. `pre-commit run -a`
3. `go test -v ./...`
4. Hand off with file-level summary.

### Adding a command

1. Add command wiring under `commands/`.
2. Put logic in `api/` / `config/` (avoid bloating the command handler).
3. Update README usage if user-facing behavior changes.
4. Run checks.

### Refactor

1. Keep the diff reviewable.
2. Avoid renaming public flags/subcommands unless requested.
3. Ensure tests still pass and CLI behavior remains consistent.
