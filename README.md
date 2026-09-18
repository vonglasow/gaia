# Gaia

A Go CLI for working with local models. The kernel bootstraps the app and manages
plugins; every feature is a plugin compiled into the one binary and switched on or
off by config.

```bash
make build
./bin/gaia --help
```

## What it does

| | |
|---|---|
| `gaia ask "…"` | One question to a model. Ollama, OpenAI or Mistral. |
| `gaia agent "…"` | A model working on a project: reads files, runs the project's checks, reports. Changes nothing unless `--write`. |
| `gaia investigate "…"` | An operator loop for a goal on this machine, driving commands under a policy. |
| `gaia chat` | A conversation with a model under a role — `/role physics` and stay there. |
| `gaia mem …` | Search and write the MemPalace memory. |
| `gaia serve` | An MCP daemon, so other tools can reach gaia's tools. |
| `gaia models` | What is loaded in Ollama, what it costs, and how to unload it. |
| `gaia roles`, `gaia config`, `gaia cache`, `gaia plugins` | Roles, configuration, cached answers, which plugins are on. |

`--debug` turns on debug output everywhere. Every command exits non-zero when it
fails, so `gaia … && …` and `set -e` behave.

## gaia agent

Give a model a project and a task.

```bash
gaia agent "why does TestAdd fail?"                 # reads and explains
gaia agent --write "fix TestAdd, then run the tests" # and changes files
gaia agent -C ../other-project "what does this do?"  # somewhere else
```

It has five tools: `list_files`, `read_file`, `search_text`, `run_command`, and
`write_file` when `--write` is given. It stops when it answers, when it repeats
itself, or when it runs out of turns — and it always says which.

Three things bound it, none of which is a sentence in a prompt:

- **A workspace.** Every path is resolved through symlinks and refused if it lands
  outside the project. `--write` cannot reach `~/.ssh`.
- **A command policy.** Reads run; a short list never runs; everything else is
  confirmed, and with nobody to confirm — a pipe, cron, an MCP client — it does not
  run. Commands never go through a shell, so a semicolon in an argument stays a
  semicolon. An allowance covers the program, not what it is pointed at: `cat` runs,
  `cat ~/.ssh/id_rsa` is confirmed first, and so is `find -delete`.
- **A loop with limits.** A ceiling on turns, and a stop when a turn repeats the
  previous turn's calls exactly.

What it is told comes from the project itself: gaia's own rules, plus the project's
`AGENTS.md` (or `CLAUDE.md`, or `CONTRIBUTING.md`) quoted as data. A `--role` only
has to exist for what those do not cover.

Config, all optional:

```yaml
agent:
  model: "qwen3-coder:30b"    # or set the shared `model`
  max_steps: 20
  allowlist: ["npm"]          # added to the built-in policy, never replacing it
  denylist: ["go generate"]
```

## Choosing a model per use

A 7B answers a question about a file; a 30B writes the patch. Every command takes
`--model` and `--provider`, and each has its own config key, so trying one is not a
config edit:

```yaml
model: "qwen2.5:latest"       # the fallback for everything
ask:        { model: "qwen2.5:latest" }
agent:      { model: "qwen3-coder:30b" }
investigate:{ model: "qwen2.5:14b" }
```

```bash
gaia agent --model qwen3-coder:30b "fix the failing test"
gaia ask --provider openai --model gpt-4o "explain this stack trace"
```

Precedence is the same everywhere: what you typed, then the command's own key, then
the shared one.

Tool calling needs a model that supports it — qwen2.5, qwen3-coder, llama3.1 and
others do. A model that does not will describe the calls it means to make instead of
making them; gaia reports that as its own stop reason rather than as an answer, so
you know to reach for a bigger one.

## Keeping the machine usable

A model stays resident after it answers — Ollama's default is five minutes, which
is 18 GB held for a 30B while you are trying to work.

```bash
gaia models                  # what is loaded, what it costs, how long it stays
gaia models unload           # take it all back now
gaia models unload qwen2.5   # or just one
gaia agent --unload "…"      # drop the model when this run ends
```

```yaml
ollama:
  keep_alive: "30s"          # sent with every request; "0" drops it immediately
```

What gaia cannot set is the Ollama service's own limits — `OLLAMA_MAX_LOADED_MODELS`
and `OLLAMA_NUM_PARALLEL` belong to whoever starts the daemon.

## gaia serve

An MCP daemon at `http://localhost:8765/mcp`, exposing the tools of every enabled
plugin: MemPalace, and `gaia_inspect_project` — the agent, read-only.

```bash
gaia serve            # starts it detached
gaia serve token      # the bearer token a client must present
gaia serve status
gaia serve stop
```

Two checks stand in front of it. A **bearer token**, kept at
`~/.config/gaia/serve.token` (0600) and generated on first use: a browser cannot read
a file under `~/.config`, so it cannot present it. And an **origin check**: a request
whose `Host` or `Origin` is not this machine is refused, which is what stops a page
you visit from reaching a daemon on your loopback.

The agent is read-only over MCP on purpose. There is nobody at the other end of an
MCP call to confirm a command or look at a diff.

```yaml
serve:
  port: "8765"
```

## Configuration

`~/.config/gaia/config.yaml`, or `--config`, or `$GAIA_CONFIG`. Per-plugin files live
at `~/.config/gaia/plugins/<plugin>.yaml`.

Kernel keys: `plugins.enabled`, `plugins.disabled`, `config.validation`
(`strict` · `warn` · `off`). Everything else is namespaced by plugin and checked
against that plugin's schema — `gaia config set bogus.key x` is refused.

```yaml
plugins:
  disabled: ["chat"]

# shared by ask, chat, agent and investigate unless overridden
provider: "ollama"
host: "localhost"
port: 11434
model: "qwen2.5:14b"
timeout_seconds: 120
```

A repository can carry its own `.gaia.yaml`, merged only once you have trusted it:

```bash
gaia config trust .
gaia config trusted
```

### Per-feature keys

**ask / chat / agent / investigate** — `<plugin>.provider`, `.host`, `.port`,
`.model`, `.timeout_seconds`, `.role`, each falling back to the shared key.

**cache** — `cache.enabled` (default false), `cache.dir`, `cache.ttl_seconds`,
`cache.refresh`.

**sanitize** — `sanitize.enabled` (default false), `sanitize.level`
(`none` · `light` · `aggressive`), `sanitize.max_tokens_after`, `sanitize.log_stats`.

> What this removes is noise: debug lines, timestamps, repeated lines, and in
> aggressive mode over-long unbroken ones. It does **not** redact addresses, tokens
> or keys, and the last thing you typed is exempt from even that. Do not enable it
> expecting privacy.

**investigate / agent policy** — `<plugin>.allowlist` and `<plugin>.denylist` add to
the built-in policy rather than replacing it, so naming one command does not silently
drop the defaults.

**mempalace** — `mempalace.mcp.command` (default
`~/.local/pipx/venvs/mempalace/bin/python`), `.args`, `.timeout_seconds`,
`mempalace.debug`, `mempalace.inject.enabled`, `.max_results`, `.min_score`.
`ask` answers are written back to `wing=gaia`, `room=ask`.

## Roles

A role is a system prompt in a YAML file under `roles/`, selected with `--role` or
matched automatically.

```bash
gaia roles list
gaia roles show code
gaia ask --role code "explain this function"
gaia chat --role physics                    # and stay in that specialisation
```

In a chat session the role can change without losing the conversation:

```
/role            show the role in force
/role physics    work under that role from now on
/role none       stop using one
/roles           list what is available
/reset           forget the conversation, keep the role
/help            this
/exit            leave
```

`enabled: false` keeps a role out of the listing and out of selection. Two roles with
the same `name` in one directory are refused: which one won would otherwise depend on
directory order.

## Ollama

`ask`, `chat`, `agent` and `investigate` check that the model exists before asking,
and pull it if it does not. `--pull` forces a refresh. Progress goes to stderr.

Tool calling uses the model API's own tools field. A model that supports it — qwen2.5,
qwen3-coder, llama3.1 and others — gets a schema for each tool and answers with
structured calls.

## Reading narrowly

`read_symbol` reads one function, type or variable by name, with the comment
above it. On the file this project kept failing to edit, that is 182 tokens
against 2065 for the whole file — and the whole file would have stayed in the
conversation for every turn after. Methods are named `Type.Method`. Lines come
back numbered the way `read_file` numbers them, so a quote copied from either
can be handed straight to `edit_file`.

## The context window

Ollama's default window is 4096 tokens, and it does not complain when a request
exceeds it — it drops the oldest messages, which are the system prompt and the
task. One source file is enough: reading a 570-line file costs about 4000
tokens on its own, and what comes back is a model answering about the last thing
it can still see.

gaia sizes the window to what it is actually sending — the rules, the
conversation so far, and the tool schemas, which are not free either — and asks
Ollama for that, in the buckets it allocates in. A short question still gets
4096; an agent that has read a file gets 8192 or more.

A window is memory, so it is capped at 32768. Both ends are yours to move:

```yaml
ollama:
  num_ctx: 65536       # use exactly this, whatever gaia works out
  max_num_ctx: 8192    # or never ask for more than this
```

An agent run also trims as it goes. Every file it reads would otherwise stay in
the conversation for the rest of the run, and the window no longer grows to
absorb that. The task and the last few turns are kept whole; older tool output
is replaced by a line saying what it was and how long, so the model can fetch it
again rather than conclude the file was empty.

An agent run fixes its window once, before the reads that will fill it. Ollama
reloads the model whenever num_ctx changes — measured at 6.8 seconds against 0.3
for the same request at a size it already held — so a window that grew turn by
turn would pay that on every read.

Lower the cap on a machine where a wider window pushes the model off the GPU.
`ollama ps` shows the split and the size: on 16GB, qwen2.5:14b sits at 9.5GB and
100% GPU with a 4096 window, and at 12GB and 91%/9% GPU/CPU with 16384. The
second is slower than the first despite remembering more, so measure before
raising it.

## A model on another machine

Nothing about gaia assumes the model is local. Point it somewhere else:

```bash
gaia agent --host 192.168.1.42 "why does the build fail?"
```

Or once, in `~/.config/gaia/config.yaml`, for every command:

```yaml
host: "192.168.1.42"
port: 11434
```

Per-command keys win over that, so a fast machine can do the agent's work while
questions stay local:

```yaml
model: "qwen2.5:latest"      # here
agent:
  host: "192.168.1.42"       # there
  model: "qwen3-coder:30b"
```

The one thing to do on the **other** machine is let Ollama listen beyond its own
loopback, which it does not by default:

```bash
OLLAMA_HOST=0.0.0.0:11434 ollama serve
```

Then `curl http://192.168.1.42:11434/api/tags` from here should list its models.
If it does, gaia will reach it — including the model discovery: with no model
configured, gaia asks *that* machine what it is holding.

There is no authentication on an Ollama port. Anyone who can reach it can use
the GPU behind it, so keep it to a network you trust.

## Working on gaia

```bash
make build          # into ./bin, stamped with version, commit and time
make test           # unit and acceptance
make bdd            # only the Gherkin scenarios in bdd/
make check          # what a commit must pass: tested-check, lint, cover-check
make cover          # coverage per function
make mutation PKG=./plugins/shared/execpolicy   # does a test notice when a line changes?
```

`make check` is what `pre-commit` runs at commit time, alongside gofmt, goimports,
golangci-lint, semgrep and govulncheck.

Three gates guard the suite:

- **`tested-check`** — every package has a test file. The cheapest of the three, and
  the one that catches a package added with no tests at all.
- **`cover-check`** — a floor on the total and a floor per package, because a total
  dilutes: well-covered packages absorb one at zero until they cannot.
- **`depguard`**, in `.golangci.yml` — the kernel imports no plugin,
  `plugins/shared` is a leaf, and `config` does not depend on the kernel. These were
  claims in a document before they were a gate.

Coverage uses `-coverpkg=./...` so the acceptance scenarios in `bdd/` count for the
code they drive, which is most of the CLI.

`make mutation` answers the question coverage cannot: it breaks each line on purpose
and checks that a test turns red. It is not a commit hook, because a pass recompiles
the package once per mutant. Run on `execpolicy` it found ten lines that were
executed and never asserted on.

See [CONTRIBUTING.md](CONTRIBUTING.md) for the rest: what to install, what the
gates check, and the conventions that are not obvious from the code.

### Adding a plugin

1. `plugins/<name>/plugin.go`, implementing `kernel.Plugin`:
   `ID`, `DefaultEnabled`, `DependsOn`, `ConfigSchema`, `Register`, `MCPTools`.
2. Config keys prefixed with the plugin ID — `myplug.setting`, or `myplug.*` for a
   subtree.
3. Register it in `plugins/registry.go`.
4. Add a test file. `tested-check` will tell you if you forget.

Tools returned from `MCPTools()` are served by `gaia serve` for every **enabled**
plugin, so disabling a plugin takes its tools off the MCP surface too.
