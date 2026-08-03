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
| | `connect` | - | Connect two wants (auto-applies best field-match) |
| | `disconnect` | - | Disconnect two wants (removes expose/import link) |
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
| | `reload` | - | Reload user custom types from `~/.mywant/custom-types/` |
| `custom` | - | `customs` | Manage customs (`~/.mywant/customs.yaml`) |
| | `list` | - | List installed customs |
| | `install` | - | Install a custom from git URL / GitHub shorthand / local dir |
| | `uninstall` | `remove` | Remove a custom and its links |
| `interact` | - | `i` | Interactive creation |
| | `start` | `st` | Start session |
| | `send` | `s` | Send message |
| | `deploy` | `d` | Deploy recommendation |
| | `end` | `e` | End session |
| | `shell` | `sh` | Interactive shell |
| `memo` | - | `m` | Manage the memo catalog |
| | `list` | `l` | List catalogs and their values |
| | `get` | `g` | Show values of one catalog |
| | `add` / `remove` | `rm` | Add or remove values |
| | `events` | - | Value provenance log |
| | `stats` | - | Per-value usage counts |
| | `labels` / `label` | - | List / set / remove value labels |
| | `groups` | - | list / create / update / delete groups |
| `world` | - | `worlds`, `wo` | Manage worlds (want snapshots) |
| | `list` | `l` | List saved worlds |
| | `open` | `switch`, `use` | Switch world (auto-saves current) |
| | `save` | - | Snapshot without switching |
| | `export` | `e` | Export world YAML |
| | `import` | - | Import a wants YAML as a world |
| `params` | - | `pa` | Manage global parameters |
| | `get` | `g` | List all parameters |
| | `set` | - | Set a parameter |
| | `delete` | `del`, `rm` | Delete a parameter |
| | `import` | - | Import parameters from YAML/JSON file |
| | `export` | - | Export parameters to stdout or file |
| `state` | - | `states` | Cross-want state CRUD |
| | `list` | - | List state for all wants |
| | `search` | - | Search state by field |
| | `get` / `set` / `delete` | - | Read or edit one want's state |
| | `clear` | - | Clear a want's state |
| | `global` | `g` | get / set / delete / clear global state |
| `achievements` | - | `ach` | Manage achievements and rules |
| `plugin` | - | - | List CLI plugins (`mywant-<name>` in PATH) |
| `skills` | - | - | Install MyWant skills for agents |
| `gui` | - | - | Control the web GUI |
| | `show` | - | Navigate the GUI to a specific view |
| | `dashboard` | - | Navigate to the dashboard |
| | `want` | - | Open sidebar for a specific want |
| | `get` | `g` | Show current GUI state |

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

# Watch wants list for real-time updates (auto-refresh every 2 seconds)
./bin/mywant wants list --watch
# Short version:
./bin/mywant w l -w

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

# Connect two wants (auto-applies the best expose/import field-match recommendation)
./bin/mywant wants connect <NAME-OR-ID-A> <NAME-OR-ID-B>

# Disconnect two wants (removes the expose/import link between them)
./bin/mywant wants disconnect <NAME-OR-ID-A> <NAME-OR-ID-B>
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

### Custom Management (Addons)

`custom` はサーバーを拡張するパッケージ（want type / design plugin / recipe / icon style）の
インストール・アンインストールを行います。実体は `~/.mywant/customs/<name>` に置かれ、
種別ごとのランタイムディレクトリへシンボリックリンクされます。

| kind | リンク先 | 検出条件（`custom.yaml` が無い場合） |
| :--- | :--- | :--- |
| `custom-type` | `~/.mywant/custom-types/<name>` | `wantType:` または `agent:` を含む YAML |
| `design` | `~/.mywant/design-plugin/<name>` | ルートに `plugin.jsx` / `.tsx` / `.js` / `.ts` |
| `recipe` | `~/.mywant/recipes/<name>/`（YAML を個別リンク） | `recipe:` を含む YAML |
| `icon` | `~/.mywant/icons/<name>` | `icons/` ディレクトリ or `icon-style.yaml`（※サーバー未対応） |

```bash
# 一覧（未管理ディレクトリも別枠で表示される）
./bin/mywant custom list

# インストール（bare name → https://github.com/onelittlenightmusic/mywant-<name>）
./bin/mywant custom install transit-plugin
./bin/mywant custom install onelittlenightmusic/mywant-transit-plugin
./bin/mywant custom install https://github.com/owner/repo.git
./bin/mywant custom install ./my-skin --kind design --name neon

# 同じ名前で再実行すると git pull で更新
./bin/mywant custom install transit-plugin

# アンインストール（リンクとストアの両方を削除）
./bin/mywant custom uninstall mywant-transit-plugin
```

**フラグ:** `--name`（インストール名）• `--kind`（種別を明示、カンマ区切り）• `--force`（既存を置換／git clone でないものを削除）• `--no-reload`（サーバーへの want type リロードを行わない）

**管理ファイル:** `~/.mywant/customs.yaml` にインストール元・コミット・作成したリンクを記録します。
インストール／アンインストール後は自動的に `POST /api/v1/want-types/reload` を呼びます
（agent 定義は起動時のみ読まれるため、agent を含む custom はサーバー再起動が必要）。

**`custom.yaml`（任意、パッケージ側に置く）:**

```yaml
custom:
  name: my-pack
  description: Want types and a canvas skin
  components:
    - kind: custom-type
      path: types
    - kind: design
      path: skins/neon
      name: neon        # リンク先の名前（省略時は custom 名）
```

> **Note:** CLI 自体を拡張する `mywant plugin`（PATH 上の `mywant-<name>` 実行ファイル）とは別物です。

**`--context` / リモートバックエンド:** custom はサーバーを動かしているマシンの
ファイルシステム上に存在するため、コマンドは**アクティブな context が指すマシン**を対象にします。

| context | 動作 |
| :--- | :--- |
| ローカル（localhost / 127.0.0.1） | CLI プロセス内で直接 `~/.mywant` を操作。サーバー未起動でもインストール可能（リロードのみスキップ） |
| リモート（`--context fly` など） | サーバーの `/api/v1/customs` 経由で、**サーバー側のマシン**にインストール／削除 |

```bash
# fly 上のサーバーに custom をインストール（git URL のみ。ローカルディレクトリは不可）
./bin/mywant --context fly custom install onelittlenightmusic/mywant-transit-plugin
./bin/mywant --context fly custom list
./bin/mywant --context fly custom uninstall mywant-transit-plugin

# ローカルを対象にしたいときは他のコマンドと同じく context を切り替える
./bin/mywant --context local custom list
```

対象は毎回 1行目に表示されます（`Customs on https://…` / `Customs on this machine (~/.mywant)`）。

**API:** `GET /api/v1/customs` • `POST /api/v1/customs` （`{source, name, kind, force}`）• `DELETE /api/v1/customs/{name}?force=true`。
インストール／削除後にサーバーが want type を自動リロードし、結果（`reloaded` / `warnings` /
`restart_needed`）を返します。**このエンドポイントは任意の git リポジトリをサーバー上に
clone してプラグインコードを配置する**ため、他の API と同じ認証の内側に置いてください
（context の username/password で保護されます）。

### System Inspection

Explore available types and agents.

```bash
# List available want types
./bin/mywant types list
# Short version:
./bin/mywant t l

# Reload user custom types without restarting the server
# (re-scans ~/.mywant/custom-types/ and hot-reloads all plugin YAML files)
./bin/mywant types reload

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

### Memo (remembered input values)

Memo is the catalog of values the user has entered, keyed by catalog (`stations`,
`cities`, ...). Wants record what was typed; the server keeps provenance
(events)、使用回数 (stats)、値ごとのラベル、そしてラベルを土台にした groups を持ちます。

```bash
# カタログ一覧と値
./bin/mywant memo list
./bin/mywant memo get stations --limit 5

# 値の追加・削除（PUT /api/v1/memo の read-modify-write）
./bin/mywant memo add stations 渋谷 新宿
./bin/mywant memo remove stations 新宿      # 値を削除
./bin/mywant memo remove stations           # カタログごと削除

# どこから来た値か / どれだけ使われたか
./bin/mywant memo events --limit 20
./bin/mywant memo events --catalog cities --value Kokubunji
./bin/mywant memo stats

# 値ラベル（値の識別子は <catalog>::<value>）
./bin/mywant memo labels
./bin/mywant memo label cities::Kyoto favourite true
./bin/mywant memo label cities::Kyoto favourite      # 値を省略すると削除

# グループ（"group/<name>" ラベルのファサード。memo 値と want の両方に使える）
./bin/mywant memo groups list
./bin/mywant memo groups create favourites --kind memo --member cities::Kyoto
./bin/mywant memo groups update favourites --kind memo --name faves
./bin/mywant memo groups delete faves --kind memo
```

> **Note:** 以前の `mywant memo get` / `memo clear` は**グローバル状態**を操作していました。
> グローバル状態は `mywant state global` に移動し、`memo` は memo API を扱うようになりました。

### Global State

Wants can persist key-value pairs via `StoreGlobalState`.

```bash
./bin/mywant state global get
./bin/mywant state global get --json
./bin/mywant state global set trip_budget 1000     # 値は JSON として解釈（失敗したら文字列）
./bin/mywant state global delete trip_budget
./bin/mywant state global clear -y
```

### Worlds

World は非システム want 全体の名前付きスナップショット（`~/.mywant/worlds/<name>.yaml`）です。
`open` は**現在の world を自動保存してから**切り替えるので、切り替えで作業を失いません。

```bash
./bin/mywant world list                                  # * が現在の world
./bin/mywant world open travel-demo
./bin/mywant world save travel-demo                      # 切り替えずにスナップショット
./bin/mywant world export travel-demo -o travel.yaml     # -o 省略で標準出力
./bin/mywant world import travel-demo -f travel.yaml
```

### GUI Control

Inspect and control the integrated web dashboard from the command line.

```bash
# Show the current GUI state (active view, sidebar status, filters)
./bin/mywant gui get
# Short version:
./bin/mywant gui g

# Navigate to the dashboard (close sidebar)
./bin/mywant gui show dashboard
# With status filter and search query
./bin/mywant gui show dashboard --filter reaching --search "travel"

# Open the sidebar for a specific want
./bin/mywant gui show want <WANT_ID>
# Open a specific tab in the sidebar
./bin/mywant gui show want <WANT_ID> --tab logs
# Available tabs: settings, results, logs, agents, versions, chat
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

## Contexts (backend の切り替え)

kubectl と同じ要領で、接続先バックエンドを `~/.mywant/config.yaml` の
`contexts:` に名前付きで並べ、`current_context:` で選ぶ。設定ファイルを
書き換えるだけで全コマンドの宛先が変わる。

```yaml
current_context: fly

contexts:
  local:
    server: http://localhost:8080
  fly:
    server: https://osaki-mywant-gui.fly.dev
    username: admin
    password_env: MYWANT_AUTH_PASSWORD   # 環境変数名。パスワード自体は書かない
```

fly.io 構成では backend が private ingress なので、公開されている GUI アプリ
（Basic 認証付きで `/api/*` をプロキシしている）を `server` に指定する。

```bash
# コンテキストの作成・更新
./bin/mywant config set-context fly \
    --server https://osaki-mywant-gui.fly.dev \
    --username admin --password-env MYWANT_AUTH_PASSWORD

# 一覧（* が現在のコンテキスト。認証情報は伏せて表示される）
./bin/mywant config get-contexts

# 切り替え
./bin/mywant config use-context fly
./bin/mywant config current-context

# 削除
./bin/mywant config delete-context fly
```

Bearer トークン認証の場合は `--token-env` / `token_env:` を使う（Basic 認証より優先される）。
`--password` / `--token` で直接値を渡すこともできるが config.yaml に平文で残る。

パスワードの生成・ローテーション（`fly secrets`）、macOS Keychain との連携など、
認証まわりの詳細は [認証ガイド](auth.md) を参照。

**宛先の決定順（上が優先）:**

1. `--server` フラグ（URL のみ上書き。認証情報はコンテキストのものが使われる）
2. `MYWANT_SERVER` / `MYWANT_TOKEN` / `MYWANT_USERNAME` / `MYWANT_PASSWORD` 環境変数
3. `--context <name>` で指定したコンテキスト
4. `current_context` のコンテキスト
5. `server_host` + `server_port`（スキームは http 固定）
6. `http://localhost:8080`

なお `start` / `stop` / `ps` はローカルプロセスを管理するコマンドなので、
コンテキストではなく `server_host` / `server_port` を見る。

### 環境変数（`environments:`）

`mywant start` が起動時にサーバのプロセスへ撒く環境変数。カスタム want type が
API キーを受け取る口はここ（例: `TICKETMASTER_API_KEY`, `GOOGLE_MAPS_API_KEY`）。

```bash
# 一覧（値はマスクされる。--show で全文表示）
./bin/mywant config env list

# 設定・更新（値を省くと非表示のプロンプトで訊かれる。Enter で確定）
./bin/mywant config env set TICKETMASTER_API_KEY

# ファイルやパイプから渡す
./bin/mywant config env set TICKETMASTER_API_KEY --stdin < key.txt

# 値を引数で直接渡すこともできるが、シェル履歴に残る
./bin/mywant config env set MYWANT_SERVER http://localhost:8080

# 削除
./bin/mywant config env unset TICKETMASTER_API_KEY
```

キー名は環境変数として使える形（英大文字・数字・`_`、先頭は数字以外）に限られる。

**反映は即時。** `set` / `unset` は書き込んだあと動いているサーバに
`POST /api/v1/config/reload-env` を投げ、サーバプロセスの環境変数を更新する
（`custom install` が want type をホットリロードするのと同じ流儀）。
スキルは起動のたびに `os.Environ()` を読むので、次のエージェント実行から効く。
サーバが動いていなければ次の起動時に読まれるだけなので、何もしなくてよい。

ただし**シェルから export された環境変数の方が強い**。config.yaml の値は
「そのキーがまだ無いとき」と「以前 config から入れたとき」だけ適用される。
サーバを起動したシェルで `export FOO=bar` していたら、config の `FOO` は効かない。

`config set` の方は固定キー（`agent_mode` / `server_port` など）専用で、
`environments` は扱わない。

## Global Flags

- `--server`: Specify MyWant server URL (overrides the active context)
- `--context`: Use a named context from config.yaml for this invocation
- `--config`: Specify a custom CLI config file (default: `~/.mywant/config.yaml`)
- `-h, --help`: Show help for any command