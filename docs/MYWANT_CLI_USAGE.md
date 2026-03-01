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
| `memo` | - | `m` | Manage global state (memo) |
| | `get` | `g` | Display current global state |
| | `clear` | - | Clear all global state |
| `params` | - | `pa` | Manage global parameters |
| | `get` | `g` | List all parameters |
| | `set` | - | Set a parameter |
| | `delete` | `del`, `rm` | Delete a parameter |
| | `import` | - | Import parameters from YAML/JSON file |
| | `export` | - | Export parameters to stdout or file |

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

### Global State (Memo)

Wants can persist key-value pairs via `StoreGlobalState`. The `memo` command lets you inspect and clear that data from the CLI.

```bash
# Display all current global state
./bin/mywant memo get
# Short version:
./bin/mywant m g

# Output as JSON
./bin/mywant memo get --json

# Clear all global state (prompts for confirmation)
./bin/mywant memo clear
# Skip confirmation
./bin/mywant memo clear -y
```

### Global Parameters

Global parameters are stored in `~/.mywant/parameters.yaml` and can be referenced by want type definitions via `defaultGlobalParameter`. They act as a last-resort default when neither `spec.params` nor the YAML `default` is set.

```bash
# List all parameters
./bin/mywant params get
# Short version:
./bin/mywant pa g

# Output as JSON
./bin/mywant params get --json

# Set a single parameter (value is parsed as JSON if possible)
./bin/mywant params set llm_provider anthropic
./bin/mywant params set opa_llm_use_llm true
./bin/mywant params set opa_llm_planner_command /usr/local/bin/opa-llm-planner

# Delete a parameter
./bin/mywant params delete llm_provider
# Short versions:
./bin/mywant pa del llm_provider
./bin/mywant pa rm llm_provider

# Import parameters from a YAML file (replaces all existing)
./bin/mywant params import -f ~/.mywant/parameters.yaml

# Merge parameters from a file with existing ones
./bin/mywant params import -f extra.yaml --merge

# Export current parameters to stdout (YAML)
./bin/mywant params export

# Export to a file
./bin/mywant params export -f backup.yaml
```

**`params import` フラグ一覧:**

| フラグ | 短縮 | デフォルト | 説明 |
| :--- | :--- | :--- | :--- |
| `--file` | `-f` | — | YAML または JSON ファイルパス（必須） |
| `--merge` | — | `false` | 既存パラメーターに追記（省略時は全置換） |

**`params export` フラグ一覧:**

| フラグ | 短縮 | デフォルト | 説明 |
| :--- | :--- | :--- | :--- |
| `--file` | `-f` | — | 出力先ファイルパス（省略時は stdout） |

#### want type YAML での `defaultGlobalParameter` の使い方

want type の YAML 定義でパラメーターに `defaultGlobalParameter` を指定すると、
`spec.params` にも YAML `default` にも値がないときに global parameters から値が取得されます。

**優先順位:** `spec.params` > `default`（YAML定義） > `defaultGlobalParameter`（global params） > `GetXxxParam` のハードコード値

```yaml
# yaml/want_types/system/opa_llm_planner.yaml (抜粋)
parameters:
- name: opa_llm_planner_command
  type: string
  default: "opa-llm-planner"
  defaultGlobalParameter: opa_llm_planner_command   # ~/.mywant/parameters.yaml の同名キー
  required: false

- name: llm_provider
  type: string
  default: "anthropic"
  defaultGlobalParameter: llm_provider
  required: false
```

```bash
# 対応する global parameters の設定例
./bin/mywant params set opa_llm_planner_command /Users/me/bin/opa-llm-planner
./bin/mywant params set opa_llm_policy_dir /etc/opa/policies
./bin/mywant params set opa_llm_use_llm true
./bin/mywant params set llm_provider anthropic
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