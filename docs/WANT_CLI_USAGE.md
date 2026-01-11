# want-cli Usage Guide

`want-cli` is a powerful command-line tool to manage MyWant executions, recipes, agents, and the server itself.

## Installation

Build the CLI using the Makefile:

```bash
# Build the CLI with embedded GUI
make release
```

## Core Commands

### Server & GUI Management

Start, monitor, and stop the integrated MyWant services (API and GUI).

```bash
# Start MyWant server (API and GUI) in background
# (Includes a guard to prevent multiple instances on the same port)
./want-cli start --detach --port 8080

# Check status of all processes (Server, GUI, and Mock Server)
./want-cli ps

# Stop the server (Robust cleanup: kills processes by PID and Port)
./want-cli stop
```

### Want Management

List, view, and manage lifecycle of wants.

```bash
# List all wants
./want-cli wants list

# Get detailed status of a specific want
./want-cli wants get <WANT_ID>

# Create/Deploy a new want from YAML file
./want-cli wants create -f config.yaml

# Delete a want
./want-cli wants delete <WANT_ID>

# Batch lifecycle operations
./want-cli wants suspend <ID1> <ID2>
./want-cli wants resume <ID1>
./want-cli wants stop <ID1>
./want-cli wants start <ID1>

# Export/Import wants
./want-cli wants export --output backup.yaml
./want-cli wants import --file backup.yaml
```

### Recipe Management

Handle reusable templates.

```bash
# List available recipes
./want-cli recipes list

# Create a new recipe from a file
./want-cli recipes create -f recipe.yaml

# Generate a recipe from an existing deployed want (and its children)
./want-cli recipes from-want <WANT_ID> --name "my-new-recipe"
```

### System Inspection

Explore available types and agents.

```bash
# List available want types (standard and custom)
./want-cli types list

# List registered agents and their capabilities
./want-cli agents list

# List capabilities
./want-cli capabilities list
```

### Utilities

```bash
# View API operation logs
./want-cli logs

# Query the integrated LLM (Ollama)
./want-cli llm query "Tell me about my system status"
```

## Shell Completion

`want-cli` supports generating shell completion scripts for Bash, Zsh, Fish, and PowerShell.

### Zsh (Recommended)

To enable completion in your current session:
```zsh
source <(./want-cli completion zsh)
```

To make it persistent, add the following to your `~/.zshrc`:
```zsh
source <(path/to/want-cli completion zsh)
```

Alternatively, you can add the completion script to your fpath:
```zsh
mkdir -p ~/.zsh/completions
./want-cli completion zsh > ~/.zsh/completions/_want-cli
# Then add these lines to ~/.zshrc if they aren't there:
fpath=(~/.zsh/completions $fpath)
autoload -U compinit; compinit
```

### Bash

To enable completion in your current session:
```bash
source <(./want-cli completion bash)
```

To make it persistent, add the following to your `~/.bashrc`:
```bash
source <(path/to/want-cli completion bash)
```

## Global Flags

- `--server`: Specify MyWant server URL (default: `http://localhost:8080`)
- `--config`: Specify a custom CLI config file (default: `~/.want-cli.yaml`)
- `-h, --help`: Show help for any command
