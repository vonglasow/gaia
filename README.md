# gaia

## Requirements

```sh
brew install ollama
ollama serve
ollama pull openhermes2.5-mistral
```

## Installation

```sh
brew tap vonglasow/tap
brew install gaia
# or
brew install vonglasow/tap/gaia
```

## Usage

```sh
$ gaia -h
Usage:
  gaia [command]

Available Commands:
  ask         Ask to a model
  completion  Generate the autocompletion script for the specified shell
  config      Set configuration options
  help        Help about any command
  version     Print the version information

Flags:
  -h, --help   help for gaia
```

```sh
$ cat CVE-2021-4034.py | gaia ask "Analyze and explain this code"
This code is a Python script that exploits the CVE-2021-4034 vulnerability in Python. It was originally written by Joe Ammond, who used it as an experiment to see if he could get it to work in Python while also playing around with ctypes.

The code starts by importing necessary libraries and defining variables. The `base64` library is imported to decode the payload, while the `os` library is needed for certain file operations. The `sys` library is used to handle system-level interactions, and the `ctypes` library is used to call the `execve()` function directly.

The code then decodes a base64 encoded ELF shared object payload from a previous command (in this case, using msfvenom). This payload is created with the PrependSetuid=true flag so that it can run as root instead of just the user.

An environment list is set to configure the call to `execve()`. The code also finds the C library, loads the shared library from the payload, creates a temporary file for exploitation, and makes necessary directories.

The code ends with calling the `execve()` function using the C library found earlier, passing in NULL arguments as required by `execve()`.
```
