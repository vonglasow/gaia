# Gaia (Minimal Kernel + Plugins)

Gaia is a Go CLI built around a minimal kernel. The kernel only bootstraps the app and manages plugins. Every feature is implemented as a plugin.

## Quick Start

```bash
go build ./...
./gaia --help
```

## Configuration

Default config location: `~/.config/gaia/config.yaml` (or `~/.config/gaia/config.yml`)

Kernel-owned keys:

- `plugins.enabled`: list of plugin IDs to force-enable
- `plugins.disabled`: list of plugin IDs to force-disable
- `config.validation`: `strict`, `warn`, or `off` (default: `warn`)

Plugin keys must be namespaced as `<plugin>.*` and are validated against each plugin’s schema.
Plugin-specific config files live at `~/.config/gaia/plugins/<plugin>.yaml`.

Example:

```yaml
plugins:
  enabled: ["ask"]
  disabled: []

llm:
  provider: "ollama"
  host: "localhost"
  port: 11434
  model: "llama3.1"
  timeout_seconds: 60
```

## Plugins

Plugins are compiled into the single binary, then enabled/disabled via config.

Plugin interface:

- `ID() string`
- `DefaultEnabled() bool`
- `DependsOn() []string`
- `ConfigSchema() []string`
- `Register(k *kernel.Kernel) ([]*cobra.Command, error)`

Schema keys must be prefixed with the plugin ID (e.g., `ask.default_prompt`). To allow any nested keys, use a wildcard suffix like `ask.settings.*`.

### Built-in Example Plugins

- `ask`: ask a model via a provider (Ollama, OpenAI, Mistral)
- `chat`: chat with a model (in-memory history)
- `cache`: cache inspection and management
- `tool`: run external tools with approval
- `investigate`: operator-style investigation with tool execution
- `roles`: role loader and auto-role resolver

## Commands

```bash
gaia plugins list
gaia plugins enable ask
gaia config list
gaia config get ask.default_prompt
gaia config set ask.default_prompt "Hello"
gaia ask "Ping"
gaia chat
gaia cache list
gaia tool run git status
gaia version
gaia investigate "why is disk full?"
gaia roles list
gaia ask --role code "Explain this function"
gaia chat --role default
gaia investigate --role operator "analyze CI failures"
gaia tool git commit
gaia ask --pull "Pull model if needed"
gaia chat --pull
gaia investigate --pull "force refresh the model"
gaia config create
gaia config path
gaia config trust .
```

Global flags:
- `--debug` enables debug output across features (including roles debug).

### Ask and chat output (interactive terminal)

When stdout is a TTY, `ask` and `chat` show the model reply in an **alternate-screen Bubble Tea panel** (same rounded style as cached answers) while tokens stream in. After the stream finishes, the full answer is printed again with the usual framed **Answer** / **Assistant** box so it stays in your scrollback. When stdout is not a terminal (pipes, redirection), output falls back to plain streaming text.

### Ask Config

Ask uses shared `llm.*` config, with optional `ask.*` overrides.

Required shared config:

- `host`
- `port`
- `model`
- `timeout_seconds` (optional, default: 120)

### Cache Config

- `cache.enabled` (default: false)
- `cache.dir` (optional)
- `cache.ttl_seconds` (optional)
- `cache.refresh` (default: false, refreshes cache when set with ask/chat flags)

### Sanitize Config

- `sanitize.enabled` (default: false)
- `sanitize.level` (`none`, `light`, `aggressive`)
- `sanitize.max_tokens_after` (0 = no cap)
- `sanitize.log_stats` (default: false)

### Tools Config

- `tools.allow` (list of exact allowed commands)
- `tools.allow_patterns` (list of allowed patterns, e.g. `git *`)
- `tools.deny` (list of exact denied commands)
- `tools.deny_patterns` (list of denied patterns)

Optional ask overrides:

- `ask.host`
- `ask.port`
- `ask.model`
- `ask.timeout_seconds`

### Ollama Model Pull

When using the Ollama provider, Gaia checks if the model exists via the Ollama API.
If the model is missing, it automatically pulls it. Use `--pull` to force a refresh
even when the model already exists. Pull progress is shown as a progress bar on stderr.

## Tests

```bash
go test -v ./...
```
