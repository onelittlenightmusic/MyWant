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
| | `create` | `c` | Create recipe (file / want / interactive) |
| | `delete` | `d` | Delete recipe |
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
./bin/mywant start --detach
# Short version:
./bin/mywant s -D

# Check status of all processes
./bin/mywant ps
# Short version:
./bin/mywant p

# Stop the server
./bin/mywant stop
# Short version:
./bin/mywant st
```

### Want Management

List, view, and manage lifecycle of wants.

```bash
# List all wants
./bin/mywant wants list
# Short version:
./bin/mywant w l

# Get detailed status of a specific want
./bin/mywant wants get <WANT_ID>
# Short version:
./bin/mywant w g <WANT_ID>

# Create/Deploy a new want from YAML file
./bin/mywant wants create -f config.yaml
# Short version:
./bin/mywant w c -f config.yaml

# Delete a want
./bin/mywant wants delete <WANT_ID>
# Short version:
./bin/mywant w d <WANT_ID>

# Batch lifecycle operations
./bin/mywant wants suspend <ID1> <ID2>
./bin/mywant w sus <ID1> <ID2>

# Export/Import wants
./bin/mywant wants export -o backup.yaml
# Short version:
./bin/mywant w e -o backup.yaml
```

### Recipe Management

Handle reusable templates.

```bash
# List available recipes
./bin/mywant recipes list
# Short version:
./bin/mywant r l

# ── Create modes ──────────────────────────────────────────────

# 1. From a YAML/JSON file (existing behavior)
./bin/mywant recipes create -f recipe.yaml
# Short version:
./bin/mywant r c -f recipe.yaml

# 2. From an existing deployed want (non-interactive)
# 保存先: ~/.mywant/recipes/{name}.yaml
./bin/mywant recipes create --from-want <WANT_ID> --name "my-new-recipe"
./bin/mywant recipes create --from-want <WANT_ID> --name "my-new-recipe" \
  --category travel --custom-type "trip" --description "Travel planner"

# 3. Full interactive mode — prompts for source, want selection, and metadata
./bin/mywant recipes create -i
# Short version:
./bin/mywant r c -i
```

**`recipes create` フラグ一覧:**

| フラグ | 短縮 | デフォルト | 説明 |
| :--- | :--- | :--- | :--- |
| `--file` | `-f` | — | YAML/JSON ファイルパス |
| `--from-want` | — | — | 既存 Want の ID |
| `--name` | `-n` | — | レシピ名（`--from-want` 時は必須） |
| `--description` | `-d` | — | 説明 |
| `--version` | `-v` | `1.0.0` | バージョン |
| `--category` | `-c` | — | カテゴリ（`general`/`approval`/`travel`/`mathematics`/`queue`） |
| `--custom-type` | — | — | カスタム型識別子 |
| `--interactive` | `-i` | `false` | フル対話モード |

**インタラクティブモード (`-i`) の流れ:**

```
--- Create Recipe ---
Source:
  * 1. From an existing Want
    2. Start from scratch
Choice [1]:

Fetching wants...
Select a want:
  * 1. abc123  my-travel-want  (target)
    2. def456  my-etl-want     (target)
Choice: 2

Analyzing want def456...
Found 3 child want(s).
Detected state fields:
  budget               (number)  Budget for the trip
  destination          (string)  Travel destination

--- Recipe Metadata ---
Name [my-etl-want-recipe]: my-trip-planner
Description []:
Version [1.0.0]:
Category ... [general]: travel
Custom Type:

Save recipe 'my-trip-planner'? (y/N): y
→ Recipe 'my-trip-planner' saved.
```

> **Note:** `recipes from-want` は廃止されました。`recipes create --from-want` を使用してください。

**レシピの保存場所:**

- `yaml/recipes/` — リポジトリ同梱のビルトインレシピ
- `~/.mywant/recipes/` — `--from-want` やダッシュボードの "Save as Recipe" で保存されるユーザーレシピ

サーバー起動時に両ディレクトリを自動スキャンしてレジストリに登録します。

### System Inspection

Explore available types and agents.

```bash
# List available want types
./bin/mywant types list
# Short version:
./bin/mywant t l

# List registered agents
./bin/mywant agents list
# Short version:
./bin/mywant a l
```

### Utilities

```bash
# View API operation logs
./bin/mywant logs
# Short version:
./bin/mywant l
```

## Shell Completion

`mywant` supports generating shell completion scripts for Bash, Zsh, Fish, and PowerShell.

To enable completion in your current session (Zsh example):
```zsh
source <(./bin/mywant completion zsh)
```

## Global Flags

- `--server`: Specify MyWant server URL (default: `http://localhost:8080`)
- `--config`: Specify a custom CLI config file (default: `~/.mywant/config.yaml`)
- `-h, --help`: Show help for any command