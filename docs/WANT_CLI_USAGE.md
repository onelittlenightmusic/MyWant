# mywant Usage Guide

`mywant` is a powerful command-line tool to manage MyWant executions, recipes, agents, and the server itself.

## Installation

Build the CLI using the Makefile:

```bash
# Build the CLI with embedded GUI
make release
```

## Shortcuts (Aliases)

Most commands have short versions for convenience.

| Command | Subcommand | Alias | Description |
| :--- | :--- | :--- | :--- |
| `start` | - | `s` | Start the MyWant server (API & GUI) |
| `stop` | - | `st` | Stop the MyWant server |
| `ps` | - | `p` | Show process status |
| `logs` | - | `l` | View system logs |
| `wants` | - | `w` | Manage want executions |
| | `list` | `l` | List all wants |
| | `get` | `g` | Get want details |
| | `create` | `c` | Create a new want |
| | `delete` | `d` | Delete a want |
| | `export` | `e` | Export wants |
| | `import` | `i` | Import wants |
| | `suspend` | `sus` | Suspend executions |
| | `resume` | `res` | Resume executions |
| | `start` | `sta` | Start executions |
| | `stop` | `st` | Stop executions |
| `recipes` | - | `r` | Manage recipes |
| | `list` | `l` | List recipes |
| | `get` | `g` | Get recipe details |
| | `create` | `c` | Create recipe |
| | `delete` | `d` | Delete recipe |
| | `from-want`| `fw` | Create from want |
| `agents` | - | `a` | Manage agents |
| | `list` | `l` | List agents |
| | `get` | `g` | Get agent details |
| | `delete` | `d` | Delete agent |
| `capabilities`| - | `c` | Manage capabilities |
| | `list` | `l` | List capabilities |
| | `get` | `g` | Get capability details |
| | `delete` | `d` | Delete capability |
| `types` | - | `t` | Manage want types |
| | `list` | `l` | List types |
| | `get` | `g` | Get type details |
| `interact` | - | `i` | Interactive creation |
| | `start` | `st` | Start session |
| | `send` | `s` | Send message |
| | `deploy` | `d` | Deploy recommendation |
| | `end` | `e` | End session |
| | `shell` | `sh` | Interactive shell |

## Core Commands

### Server & GUI Management

Start, monitor, and stop the integrated MyWant services (API and GUI).

```bash
# Start MyWant server (API and GUI) in background
./mywant start --detach
# Short version:
./mywant s -D

# Check status of all processes
./mywant ps
# Short version:
./mywant p

# Stop the server
./mywant stop
# Short version:
./mywant st
```

### Want Management

List, view, and manage lifecycle of wants.

```bash
# List all wants
./mywant wants list
# Short version:
./mywant w l

# Get detailed status of a specific want
./mywant wants get <WANT_ID>
# Short version:
./mywant w g <WANT_ID>

# Create/Deploy a new want from YAML file
./mywant wants create -f config.yaml
# Short version:
./mywant w c -f config.yaml

# Delete a want
./mywant wants delete <WANT_ID>
# Short version:
./mywant w d <WANT_ID>

# Batch lifecycle operations
./mywant wants suspend <ID1> <ID2>
./mywant w sus <ID1> <ID2>

# Export/Import wants
./mywant wants export -o backup.yaml
# Short version:
./mywant w e -o backup.yaml
```

### Recipe Management

Handle reusable templates.

```bash
# List available recipes
./mywant recipes list
# Short version:
./mywant r l

# Create a new recipe from a file
./mywant recipes create -f recipe.yaml
# Short version:
./mywant r c -f recipe.yaml

# Generate a recipe from an existing deployed want
./mywant recipes from-want <WANT_ID> --name "my-new-recipe"
# Short version:
./mywant r fw <WANT_ID> -n "my-new-recipe"
```

### System Inspection

Explore available types and agents.

```bash
# List available want types
./mywant types list
# Short version:
./mywant t l

# List registered agents
./mywant agents list
# Short version:
./mywant a l
```

### Utilities

```bash
# View API operation logs
./mywant logs
# Short version:
./mywant l
```

## Shell Completion

`mywant` supports generating shell completion scripts for Bash, Zsh, Fish, and PowerShell.

To enable completion in your current session (Zsh example):
```zsh
source <(./mywant completion zsh)
```

## Global Flags

- `--server`: Specify MyWant server URL (default: `http://localhost:8080`)
- `--config`: Specify a custom CLI config file (default: `~/.mywant.yaml`)
- `-h, --help`: Show help for any command