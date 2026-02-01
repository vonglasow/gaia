# Gaia

[![pre-commit.ci status](https://results.pre-commit.ci/badge/github/vonglasow/gaia/main.svg)](https://results.pre-commit.ci/latest/github/vonglasow/gaia/main)
![Go Version](https://img.shields.io/github/go-mod/go-version/vonglasow/gaia)
![License](https://img.shields.io/github/license/vonglasow/gaia)
![Homebrew Formula](https://img.shields.io/badge/Homebrew-Install%20via%20tap-lightgrey)
![Powered by Ollama](https://img.shields.io/badge/Powered%20by-Ollama-3a86ff?logo=ollama&logoColor=white)
![100% Local AI](https://img.shields.io/badge/100%25%20Local-AI-success)
![Works Offline](https://img.shields.io/badge/Works-Offline-orange)

Gaia is a command-line interface (CLI) tool that provides a convenient way to
interact with language models through a local or remote API. It features a beautiful
terminal UI, configuration management, automatic role detection, response caching,
and support for different interaction modes.

## Features

- 🚀 Simple and intuitive command-line interface
- 🎨 Beautiful terminal UI with progress bars
- 📺 **Interactive TUI prompts** (Bubble Tea): when using tool actions (`gaia tool git commit`) or operator mode (`gaia investigate`), context and confirmation prompts use a rich terminal UI in a real terminal — styled boxes, keyboard shortcuts, text wrapping to window width
- ⚙️ Comprehensive configuration management with YAML support
- 🔄 Support for different interaction modes (default, describe, code, shell, commit, branch)
- 🤖 Automatic role detection based on message content
- 📦 Automatic model management (pull if not present)
- 💾 Response caching for faster repeated queries
- 🔌 Support for local (Ollama) and remote (OpenAI) APIs
- 🛠️ Tool integration for executing external commands
- 📥 Stdin support for piping content
- 🔍 **Investigate (operator mode)**: autonomous investigation with tool execution (e.g. "Why is my disk full?") — plan, run commands, reason, and summarize with safety controls (denylist, confirmation, dry-run)

## Installation

### Prerequisites

- Go 1.22.2 or later
- A running instance of a compatible language model API (e.g., Ollama) or OpenAI API key

### Building from Source

```bash
git clone https://github.com/vonglasow/gaia.git
cd gaia
go build
```

### Using Homebrew (recommended 🍺)

```sh
brew tap vonglasow/tap
brew install gaia
```

#### Update

```sh
brew upgrade gaia
```

## Configuration

Gaia stores its configuration in `~/.config/gaia/config.yaml`. The configuration file is automatically created on first run with sensible defaults.

### Basic Configuration

- `model`: The language model to use (default: "mistral" for Ollama, "gpt-4o-mini" for OpenAI)
- `host`: API host (default: "localhost" for Ollama, "api.openai.com" for OpenAI)
- `port`: API port (default: 11434 for Ollama, 443 for OpenAI)

### Cache Configuration

- `cache.enabled`: Enable/disable response caching (default: `true`)
- `cache.dir`: Cache directory path (default: `~/.config/gaia/cache`)

### Auto-Role Detection

- `auto_role.enabled`: Enable automatic role detection (default: `true`)
- `auto_role.mode`: Detection mode - `off`, `heuristic`, or `hybrid` (default: `hybrid`)
  - `off`: Disable auto-detection, always use default role
  - `heuristic`: Use fast local keyword matching only
  - `hybrid`: Use heuristic first, fallback to LLM for ambiguous cases
- `auto_role.keywords.<role_name>`: Custom keywords for role detection (see below)

### Roles

Roles define different interaction modes with their respective prompts. The available roles depend on what is configured in your configuration file. By default, the following roles are pre-configured:

**Default Roles (Pre-configured):**
- `roles.default`: General programming and system administration assistance
- `roles.describe`: Command description and documentation
- `roles.shell`: Shell command generation
- `roles.code`: Code generation without descriptions
- `roles.commit`: Generate conventional commit messages
- `roles.branch`: Generate branch names

**Custom Roles:**
You can add custom roles by adding `roles.<role_name>` keys to your configuration file. Any role defined in the configuration will be available for use.

**Note:** The list of available roles is dynamic and depends on your configuration. Use `gaia config list` to see all configured roles in your setup.

### Role Keywords (Auto-Detection)

Keywords are used by the heuristic detection to identify the appropriate role. Default keywords are pre-configured for the default roles:

**Pre-configured Keywords:**
- `auto_role.keywords.shell`: Command-related keywords
- `auto_role.keywords.code`: Programming-related keywords
- `auto_role.keywords.describe`: Question/explanation keywords
- `auto_role.keywords.commit`: Commit message keywords
- `auto_role.keywords.branch`: Branch creation keywords

**Custom Keywords:**
You can customize these keywords or add keywords for custom roles by adding `auto_role.keywords.<role_name>` keys to your configuration file. This allows auto-detection to work with your custom roles as well.

### Tool Configuration

Tools allow you to execute external commands with AI-generated content. Example configuration:

```yaml
tools:
  git:
    commit:
      context_command: "git diff --staged"
      role: "commit"
      execute_command: "git commit -F {file}"
    branch:
      context_command: "git diff"
      role: "branch"
      execute_command: "git checkout -b {response}"
```

Tool configuration fields:
- `context_command`: Command to run to gather context (optional)
- `role`: Role to use for AI generation
- `execute_command`: Command to execute with AI response (use `{file}` for multi-line, `{response}` for single-line)

### Operator (Investigate) Configuration

The operator mode (`gaia investigate`) uses the following options (all under `operator.`):

- `operator.max_steps`: Maximum number of steps per run (default: `10`)
- `operator.confirm_medium_risk`: Ask for confirmation before running medium-risk commands (default: `true`)
- `operator.dry_run`: If `true`, never execute commands; only show what would be run (default: `false`)
- `operator.denylist`: List of forbidden command patterns (e.g. `["rm -rf", "sudo", "mkfs"]`). Commands containing these are blocked.
- `operator.allowlist`: Optional. If set, only commands matching one of these patterns are allowed (e.g. `["^df", "^du", "^find"]`).
- `operator.output_max_bytes`: Maximum length of command output per step (default: `4096`); longer output is truncated.
- `operator.command_timeout_seconds`: Timeout in seconds for each shell command (default: `30`).

Example in `config.yaml`:

```yaml
operator:
  max_steps: 10
  confirm_medium_risk: true
  denylist:
    - "rm -rf"
    - "sudo"
    - "mkfs"
  allowlist: []   # leave empty to allow any command not on denylist
  command_timeout_seconds: 30
```

### Use an Alternative Configuration File

```bash
gaia --config /path/to/custom/config.yaml ask "Hello!"
# or
GAIA_CONFIG=/path/to/custom/config.yaml gaia ask "Hello!"
```

## Usage

### Basic Commands

```bash
# Ask a question
gaia ask "What is the meaning of life?"

# Ask with piped input
echo "Hello world" | gaia ask "Translate to French"
git diff | gaia ask "Generate commit message"

# Start an interactive chat session
gaia chat

# Check version
gaia version

# Investigate a goal (operator mode: runs tools, reasons, summarizes)
gaia investigate "Why is my disk full?"
gaia investigate --dry-run "What's using the most space?"
gaia investigate --yes "List large files in /tmp"   # skip confirmation for medium-risk commands
gaia investigate --debug "Why is CPU high?"         # show decisions and observations
```

### Configuration Management

```bash
# View all configuration settings
gaia config list

# Get specific configuration value
gaia config get model
gaia config get auto_role.enabled

# Set configuration value
gaia config set model llama2
gaia config set host 127.0.0.1
gaia config set port 8080
gaia config set auto_role.mode heuristic

# Show configuration file path
gaia config path

# Create default configuration file
gaia config create
```

### Using Different Roles

The available roles depend on your configuration. By default, the following roles are available:

```bash
# Use default role (general assistance)
gaia ask "How do I create a new directory?"

# Explicitly specify a pre-configured role
gaia ask --role describe "ls -la"
gaia ask --role shell "list files in current directory"
gaia ask --role code "Hello world in Python"
gaia ask --role commit "generate commit message"
gaia ask --role branch "create branch name"

# Use a custom role (if configured)
gaia ask --role my_custom_role "custom prompt"

# Auto-detection (enabled by default)
gaia ask "generate git commit message"  # Automatically detects "commit" role
gaia ask "create a new branch"          # Automatically detects "branch" role
```

**Note:** To see all available roles in your configuration, use `gaia config list` and look for keys starting with `roles.`.

### Auto-Role Detection

When `auto_role.enabled` is `true` (default), Gaia automatically detects the appropriate role based on your message content:

```bash
# These will automatically use the appropriate role
gaia ask "what does ls -la do?"           # → describe role
gaia ask "list files in directory"        # → shell role
gaia ask "write a Python function"        # → code role
git diff | gaia ask "generate commit"     # → commit role
```

Use `--debug` flag to see which role was detected and how:

```bash
gaia ask --debug "generate commit message"
# [DEBUG] Auto-detected role: commit (method: heuristic, score: 0.85, reason: matched keywords)
```

### Cache Management

```bash
# View cache statistics
gaia cache stats

# List all cache entries
gaia cache list

# Dump all cached responses
gaia cache dump

# Clear the cache
gaia cache clear

# Bypass cache for a single command
gaia --no-cache ask "What is AI?"

# Refresh/overwrite cache entry
gaia --refresh-cache ask "What is AI?"
```

### Global Flags

- `--config, -c`: Path to alternative configuration file
- `--no-cache`: Bypass local response cache for this command
- `--refresh-cache`: Regenerate and overwrite cache entries
- `--debug`: Enable debug output (shows role detection info)

### Chat Mode

The chat mode provides an interactive session where you can have a continuous conversation with the model. The conversation history is maintained throughout the session, allowing the model to reference previous messages.

```bash
# Start a chat session
gaia chat

# Type your messages and press Enter
# Type 'exit' to end the chat session
```

### Investigate (Operator Mode)

The **investigate** command runs an autonomous operator: it plans steps, runs shell commands (e.g. `df`, `du`, `find`), reasons over the results, and returns a summary or suggested actions. Safety is enforced via a denylist, optional allowlist, and confirmation for medium-risk commands.

```bash
# Investigate a goal (operator will run commands like df -h, du, etc.)
gaia investigate "Why is my disk full?"

# See what would be run without executing (dry-run)
gaia investigate --dry-run "What's using the most space?"

# Skip confirmation for medium-risk commands (e.g. touch, mkdir)
gaia investigate --yes "List large files in /tmp"

# Show plan, decisions, and tool results (debug)
gaia investigate --debug "Why is CPU high?"

# Limit the number of steps
gaia investigate --max-steps 5 "Quick disk check"
```

**Flags:**

- `--max-steps`, `-n`: Maximum number of operator steps (default: 10)
- `--dry-run`: Do not execute commands; only show what would be run
- `--yes`, `-y`: Skip confirmation for medium-risk commands
- `--debug`: Print each decision (action, tool, args) and observation

**Confirmation prompt (TUI):** When the operator needs your approval for a medium-risk command, Gaia shows an interactive confirmation screen in a real terminal: the proposed command in a styled box, then **y** or **Enter** to allow or **n** to decline. Content wraps to the terminal width. When not in a TTY, a simple line-based prompt is used.

**Safety:** Commands matching `operator.denylist` (e.g. `sudo`, `rm -rf`) are always blocked. If `operator.allowlist` is set, only commands matching that list are allowed. Use `--dry-run` to preview behaviour without executing anything.

### Tool Commands

Execute configured tool actions that combine AI generation with external command execution:

```bash
# Execute a tool action
gaia tool git commit
gaia tool git branch "add user authentication"

# The tool will:
# 1. Run the context_command (if configured) to gather context
# 2. Allow you to modify the context
# 3. Generate content using the specified role
# 4. Ask for confirmation
# 5. Execute the execute_command with the generated content
```

**Interactive prompts (TUI):** When you run a tool in a real terminal (TTY), Gaia shows an interactive prompt:

- **Context step:** Current context (e.g. `git diff`) is shown in a styled box. You can press **Enter** to use it as-is, type new text to replace it, **+text** to append, or **q** to quit. All content wraps to the terminal width.
- **Confirmation step:** After the AI generates the message (e.g. commit text), a confirmation screen appears: **y** or **Enter** to confirm and run the command, **n** to cancel.

When stdout is not a terminal (e.g. piping, CI), the same flow uses simple line-based prompts so scripts keep working.

### Example Usage

#### Basic Question

```bash
$ gaia ask "What is the meaning of life?"
The meaning of life is a philosophical question that has been debated for centuries...
```

#### Code Analysis with Piped Input

```bash
$ cat CVE-2021-4034.py | gaia ask "Analyze and explain this code"
This code is a Python script that exploits the CVE-2021-4034 vulnerability in Python...
```

#### Git Commit Message Generation

```bash
$ git diff --staged | gaia ask "generate commit message"
feat: add user authentication system

Implement JWT-based authentication with login and registration endpoints.
Add password hashing using bcrypt and session management.
```

#### Branch Name Generation

```bash
$ git diff | gaia ask "create branch name"
feature/user-authentication
```

#### Using Tools

```bash
$ gaia tool git commit
# Shows git diff, allows modification, generates commit message, asks confirmation, executes git commit
```

#### Investigate (Operator)

```bash
$ gaia investigate "Why is my disk full?"
# Operator runs e.g. df -h, du, reasons over output, then returns a summary and suggested next steps.

$ gaia investigate --dry-run "What's using space in /var?"
# Same flow but no commands are executed; you see what would be run.
```

## API Providers

### Ollama (Local - Default)

Ollama is the default provider for local AI models. Configure it by setting:

```yaml
host: localhost
port: 11434
model: mistral  # or any model available in Ollama
```

Features:
- Works completely offline
- Automatic model pulling if not present
- Progress bars during model download
- No API key required

### OpenAI (Remote)

To use OpenAI, configure:

```yaml
host: api.openai.com
port: 443
model: gpt-4o-mini  # or gpt-4, gpt-3.5-turbo, etc.
```

And set the API key:

```bash
export OPENAI_API_KEY=your-api-key-here
```

Features:
- Access to OpenAI's latest models
- Streaming responses
- No local model storage required

## Advanced Configuration Examples

### Custom Role

Add a custom role to your configuration:

```yaml
roles:
  custom:
    "You are a specialized assistant for [your domain]. Provide concise, technical answers."
```

Once added, the role will be available for use:
- Explicitly: `gaia ask --role custom "your question"`
- Via auto-detection (if keywords are configured): `gaia ask "your question"` (will auto-detect if keywords match)

### Custom Keywords for Auto-Detection

```yaml
auto_role:
  enabled: true
  mode: hybrid
  keywords:
    custom_role:
      - "keyword1"
      - "keyword2"
      - "phrase with multiple words"
```

### Disable Auto-Role Detection

```yaml
auto_role:
  enabled: false
```

### Use Heuristic-Only Detection

```yaml
auto_role:
  enabled: true
  mode: heuristic  # Fast, local-only detection
```

## Development

### Project Structure

- `api/`: API interaction, streaming, caching, and auto-role detection
- `api/operator/`: Operator (investigate) loop: planner, tools, executor, safety
- `commands/`: CLI command definitions
- `config/`: Configuration management
- `main.go`: Application entry point

### Dependencies

- [cobra](https://github.com/spf13/cobra): CLI framework
- [viper](https://github.com/spf13/viper): Configuration management
- [bubbletea](https://github.com/charmbracelet/bubbletea): Terminal UI framework (progress bars, interactive prompts)
- [bubbles](https://github.com/charmbracelet/bubbles): TUI components (e.g. text input for context/edit prompts)
- [lipgloss](https://github.com/charmbracelet/lipgloss): Styling and layout (boxes, colors, width-aware wrapping)

### Running Tests

```bash
go test -v ./...
```

### Code Quality

The project uses:
- `go fmt` for formatting
- `golangci-lint` for linting
- `pre-commit` hooks for automated checks

Run all checks:

```bash
pre-commit run -a
```

## License

This project is licensed under the terms specified in the LICENSE file.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
Please ensure:

- Code is formatted and linted
- Tests are added or updated
- Pre-commit hooks pass
