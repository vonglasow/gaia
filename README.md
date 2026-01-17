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
- ⚙️ Comprehensive configuration management with YAML support
- 🔄 Support for different interaction modes (default, describe, code, shell, commit, branch)
- 🤖 Automatic role detection based on message content
- 📦 Automatic model management (pull if not present)
- 💾 Response caching for faster repeated queries
- 🔌 Support for local (Ollama) and remote (OpenAI) APIs
- 🛠️ Tool integration for executing external commands
- 📥 Stdin support for piping content

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
- `commands/`: CLI command definitions
- `config/`: Configuration management
- `main.go`: Application entry point

### Dependencies

- [cobra](https://github.com/spf13/cobra): CLI framework
- [viper](https://github.com/spf13/viper): Configuration management
- [bubbletea](https://github.com/charmbracelet/bubbletea): Terminal UI framework

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
