# Gaia

[![pre-commit.ci status](https://results.pre-commit.ci/badge/github/vonglasow/gaia/main.svg)](https://results.pre-commit.ci/latest/github/vonglasow/gaia/main)
![Go Version](https://img.shields.io/github/go-mod/go-version/vonglasow/gaia)
![License](https://img.shields.io/github/license/vonglasow/gaia)
![Homebrew Formula](https://img.shields.io/badge/Homebrew-Install%20via%20tap-lightgrey)
![Powered by Ollama](https://img.shields.io/badge/Powered%20by-Ollama-3a86ff?logo=ollama&logoColor=white)
![100% Local AI](https://img.shields.io/badge/100%25%20Local-AI-success)
![Works Offline](https://img.shields.io/badge/Works-Offline-orange)

Gaia is a command-line interface (CLI) tool that provides a convenient way to
interact with language models through a local API. It features a beautiful
terminal UI, configuration management, and support for different interaction
modes.

## Features

- 🚀 Simple and intuitive command-line interface
- 🎨 Beautiful terminal UI with progress bars
- ⚙️ Configurable settings with YAML support
- 🔄 Support for different interaction modes (default, describe, code, shell)
- 📦 Automatic model management (pull if not present)
- 🔌 Local API integration

## Installation

### Prerequisites

- Go 1.22.2 or later
- A running instance of a compatible language model API (e.g., Ollama)

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

Gaia stores its configuration in `~/.config/gaia/config.yaml`. The following settings are available:

- `model`: The language model to use (default: "mistral")
- `host`: API host (default: "localhost")
- `port`: API port (default: 11434)
- `roles`: Different interaction modes with their respective prompts

### Available Roles

- `default`: General programming and system administration assistance
- `describe`: Command description and documentation
- `shell`: Shell command generation
- `code`: Code generation without descriptions

### Use an alternative yaml configuration file

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

# Start an interactive chat session
gaia chat

# Configure settings
gaia config set model llama2
gaia config set host 127.0.0.1
gaia config set port 8080
gaia config create

# View current configuration
gaia config list

# Get specific configuration value
gaia config get model

# Show configuration file path
gaia config path

# Check version
gaia version
```

### Using Different Roles

```bash
# Use default role (general assistance)
gaia ask "How do I create a new directory?"

# Use describe role for command documentation
gaia ask --role describe "ls -la"

# Use shell role for command generation
gaia ask --role shell "list files in current directory"

# Use code role for code generation
gaia ask --role code "Hello world in Python"
```

### Chat Mode

The chat mode provides an interactive session where you can have a continuous conversation with the model. The conversation history is maintained throughout the session, allowing the model to reference previous messages.

```bash
# Start a chat session
gaia chat

# Type your messages and press Enter
# Type 'exit' to end the chat session
```

### Example Usage

```bash
$ cat CVE-2021-4034.py | gaia ask "Analyze and explain this code"
This code is a Python script that exploits the CVE-2021-4034 vulnerability in Python. It was originally written by Joe Ammond, who used it as an experiment to see if he could get it to work in Python while also playing around with ctypes.

The code starts by importing necessary libraries and defining variables. The `base64` library is imported to decode the payload, while the `os` library is needed for certain file operations. The `sys` library is used to handle system-level interactions, and the `ctypes` library is used to call the `execve()` function directly.

The code then decodes a base64 encoded ELF shared object payload from a previous command (in this case, using msfvenom). This payload is created with the PrependSetuid=true flag so that it can run as root instead of just the user.

An environment list is set to configure the call to `execve()`. The code also finds the C library, loads the shared library from the payload, creates a temporary file for exploitation, and makes necessary directories.

The code ends with calling the `execve()` function using the C library found earlier, passing in NULL arguments as required by `execve()`.
```

## Development

### Project Structure

- `api/`: API interaction and streaming functionality
- `commands/`: CLI command definitions
- `config/`: Configuration management
- `main.go`: Application entry point

### Dependencies

- [cobra](https://github.com/spf13/cobra): CLI framework
- [viper](https://github.com/spf13/viper): Configuration management
- [bubbletea](https://github.com/charmbracelet/bubbletea): Terminal UI framework

## License

This project is licensed under the terms specified in the LICENSE file.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
Please ensure:

- Code is formatted and linted
- Tests are added or updated
