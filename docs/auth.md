# 認証ガイド (リモートバックエンドへの接続)

`mywant` CLI からリモートのバックエンド（Fly.io 上のデプロイなど）に接続し、
認証情報を安全に扱うための手順をまとめます。

ローカル開発だけなら認証は不要です。このドキュメントは `mywant` を
`localhost` 以外に向けるときに読んでください。

## 認証モデル

MyWant の API 自体には認証機構がありません。Fly.io 構成では、次のように
**GUI アプリが公開側の認証境界**になっています。

```
ブラウザ / mywant CLI
   │  HTTPS + Basic 認証
   ▼
osaki-mywant-gui (公開)  ──proxy /api/*──►  osaki-mywant-backend (private ingress のみ)
```

- backend にはパブリック IP がなく、Fly のプライベートネットワーク経由でしか届きません
- GUI は `/api/*` を公開プロキシしているため、**GUI の Basic 認証が唯一の防御線**です
- CLI も同じ公開 URL（GUI 側）に対して Basic 認証付きでリクエストします

つまり CLI の接続先に指定するのは backend ではなく **GUI アプリの URL** です。

デプロイ構成そのものは [Deploying MyWant on Fly.io](DEPLOY_FLY.md) を参照してください。

## Context の追加

接続先は kubectl と同じ「コンテキスト」として `~/.mywant/config.yaml` に
名前付きで登録します。設定ファイルを書き換えるだけで全コマンドの宛先が変わります。

```bash
# ローカル（先に登録しておく。下の注意を参照）
./bin/mywant config set-context local --server http://localhost:8080

# Fly.io（GUI アプリの URL を指定する）
./bin/mywant config set-context fly \
    --server https://osaki-mywant-gui.fly.dev \
    --username mywant \
    --password-env MYWANT_AUTH_PASSWORD
```

結果として `config.yaml` はこうなります。

```yaml
current_context: local

contexts:
  local:
    server: http://localhost:8080
  fly:
    server: https://osaki-mywant-gui.fly.dev
    username: mywant
    password_env: MYWANT_AUTH_PASSWORD   # 環境変数「名」。パスワード自体は書かない
```

> **注意:** `set-context` は `current_context` が未設定のとき、登録したコンテキストを
> 自動的に current にします。ローカルを既定にしておきたい場合は `local` を先に
> 登録してください。明示的に切り替えたいときは `--use` を付けます。

### 認証方式の指定

| フラグ | YAML キー | 用途 |
|---|---|---|
| `--username` | `username` | Basic 認証のユーザー名 |
| `--password-env` | `password_env` | パスワードを持つ**環境変数名**（推奨） |
| `--password` | `password` | パスワードの直値（config.yaml に平文で残る） |
| `--token-env` | `token_env` | Bearer トークンを持つ環境変数名 |
| `--token` | `token` | Bearer トークンの直値 |

- Bearer トークンが設定されている場合、Basic 認証より優先されます
- `--password` と `--password-env` は排他です。片方を指定するともう片方はクリアされます
  （同時に渡した場合は `--password-env` が残ります）。`--token` / `--token-env` も同様
- `config.yaml` には API キー等も入るため、`chmod 600 ~/.mywant/config.yaml` を推奨します

## Context の切り替え

```bash
# 一覧（* が現在のコンテキスト。認証情報は伏せて表示される）
./bin/mywant config get-contexts

# 恒久的に切り替え
./bin/mywant config use-context fly

# 現在の宛先を確認
./bin/mywant config current-context

# 一回のコマンドだけ別のコンテキストを使う
./bin/mywant --context fly wants list

# 削除（current だった場合は server_host/server_port にフォールバック）
./bin/mywant config delete-context fly
```

`get-contexts` の出力例:

```
CURRENT   NAME    SERVER                             AUTH
          fly     https://osaki-mywant-gui.fly.dev   basic mywant ($MYWANT_AUTH_PASSWORD)
*         local   http://localhost:8080              none
```

### 宛先の決定順

上にあるものほど優先されます。

1. `--server` フラグ（**URL のみ**上書き。認証情報はコンテキストのものが使われる）
2. `MYWANT_SERVER` / `MYWANT_TOKEN` / `MYWANT_USERNAME` / `MYWANT_PASSWORD` 環境変数
3. `--context <name>` で指定したコンテキスト
4. `current_context` のコンテキスト
5. `server_host` + `server_port`（スキームは http 固定）
6. `http://localhost:8080`

`start` / `stop` / `ps` はローカルプロセスを管理するコマンドなので、
コンテキストではなく `server_host` / `server_port` を見ます。

## サーバー側: パスワードの設定とローテーション

GUI アプリ側の Basic 認証は Fly secret で設定します。

| Secret | 既定値 | 説明 |
|---|---|---|
| `MYWANT_AUTH_PASSWORD` | なし | **未設定のあいだ認証は無効**（誰でもアクセスできる） |
| `MYWANT_AUTH_USER` | `mywant` | 省略可 |

### パスワードの生成

```bash
# 192bit のランダム値。16進なので記号がなく、シェル・URL・コピペで事故らない
PW=$(openssl rand -hex 24)
```

| コマンド | 出力 | 備考 |
|---|---|---|
| `openssl rand -hex 24` | 48文字の16進 | 記号なし。**推奨** |
| `openssl rand -base64 24` | 32文字 | `+` `/` を含む。24バイトは3の倍数なので `=` パディングは付かない |
| `openssl rand -base64 18` | 24文字 | 短めにしたい場合（144bit） |

`-base64` の出力から `tr -d '/+='` で記号を落とす例をよく見かけますが、
**文字が消えるぶん短くなり、そのぶんエントロピーも落ちます**。削るなら
長めに生成して `openssl rand -base64 32 | tr -d '/+=' | cut -c1-32` のように
長さを揃えてください。素直に `-hex` を使うのが確実です。

### Fly secret への設定

`fly secrets set KEY=VALUE` はパスワードがコマンドライン引数に出るため、
シェル履歴や同一ホストの `ps` から見えてしまいます。**stdin から読む
`fly secrets import` を使ってください。**

```bash
printf 'MYWANT_AUTH_PASSWORD=%s\n' "$PW" | fly secrets import -a osaki-mywant-gui
```

ユーザー名も同時に変える場合は行を足すだけです。

```bash
printf 'MYWANT_AUTH_USER=%s\nMYWANT_AUTH_PASSWORD=%s\n' "alice" "$PW" \
  | fly secrets import -a osaki-mywant-gui
```

この場合はクライアント側のコンテキストも更新してください。

```bash
./bin/mywant config set-context fly --username alice
```

### 動作確認

```bash
# 認証なし → 401 が返るのが正しい
curl -s -o /dev/null -w '%{http_code}\n' https://osaki-mywant-gui.fly.dev/api/v1/wants

# 認証あり → 200
curl -s -o /dev/null -w '%{http_code}\n' -u "mywant:$PW" \
  https://osaki-mywant-gui.fly.dev/api/v1/wants

# /healthz は意図的に認証不要（ヘルスチェック用）なので、疎通確認には使えても
# 認証の確認には使えない
curl -s -o /dev/null -w '%{http_code}\n' https://osaki-mywant-gui.fly.dev/healthz
```

### ローテーション時の注意

- `fly secrets import` は**マシンを再起動します**。`--stage` を付けると次回デプロイまで保留されます
- ブラウザで開いている GUI のセッションは再認証が必要になります
- `fly secrets list` は名前とダイジェストしか返しません。**現在の値は誰にも読めない**ので、
  控えを失った場合はローテーションするしかありません
- デプロイのスモークテスト用に `GUI_SMOKE_AUTH`（`user:password` 形式）を設定している場合、
  ローテーション時に一緒に更新しないとテストが落ちます

## mywant CLI からの使い方

コンテキストが `password_env: MYWANT_AUTH_PASSWORD` を参照する設定なので、
環境変数に入れるだけで認証が通ります。

```bash
export MYWANT_AUTH_PASSWORD='...'

./bin/mywant --context fly wants list
./bin/mywant --context fly wants get <ID>

# 恒久的に fly を既定にする
./bin/mywant config use-context fly
./bin/mywant wants list
```

### 一時的な上書き

環境変数だけで完結させることもできます（コンテキスト未登録でも動きます）。

```bash
MYWANT_SERVER=https://osaki-mywant-gui.fly.dev \
MYWANT_USERNAME=mywant \
MYWANT_PASSWORD='...' \
  ./bin/mywant wants list
```

CI などトークン認証を使う場合は `MYWANT_TOKEN` を設定します（Basic 認証より優先）。

### 認証エラーの見え方

認証情報が一切設定されていない状態で 401/403 を受け取ると、CLI はヒントを添えて失敗します。

```
API error (status 401): Unauthorized
(no credentials sent — set username/password on the active context: mywant config set-context <name> --username ...)
```

認証情報を送ったうえで 401 が返る場合は、パスワードかユーザー名が違います。
`MYWANT_AUTH_USER` を設定していないなら、ユーザー名は既定の `mywant` です。

## ブラウザ拡張 (Web Inspector) からの使い方

Web Inspector のブラウザ拡張も、CLI と同じ「名前付きコンテキストを切り替える」
方式で接続先と認証情報を持ちます。設定は拡張のオプション画面
（`chrome://extensions` → MyWant Web Inspector → 拡張機能のオプション）で行います。

| 項目 | 内容 |
|---|---|
| 名前 | `local` / `fly` など。CLI のコンテキスト名と揃えると分かりやすい |
| サーバーURL | 認証ありの場合は **GUI アプリのURL**（backend ではない） |
| ユーザー名 / パスワード | Basic 認証。ローカル開発では空のままで構いません |

保存すると、そのオリジンへのアクセス許可がブラウザから一度だけ求められます。
上部のセレクタで接続先を選び「切替」を押すと、URL と認証情報がまとめて切り替わります。

認証情報は拡張のストレージ (`chrome.storage.local`) に保存されます。CLI の
`password_env` のような環境変数による間接参照はできないため、**パスワードそのものが
保存される**点に注意してください。

> **補足:** 認証ヘッダーは、選択中のコンテキストの **オリジンが完全一致する
> リクエストにのみ** 付与されます。拡張は第三者サイト上のコンテンツスクリプトから
> 任意のURLへの fetch を中継するため、無条件に付与するとサーバーのパスワードを
> 任意のサイトへ渡してしまうためです。

ブックマークレット版（拡張を入れない方式）は認証の扱いが異なります。ブラウザが
クロスオリジンのスクリプト読み込みに Basic 認証情報を渡さないため、GUI の設定画面が
発行する Web Inspector 専用トークンをブックマークレットに埋め込みます。こちらは
パスワードと等価ではなく、Web Inspector 関連のパスにしか使えません。

## Keychain の使い方 (macOS)

毎回 `export` するのを避けたい場合、macOS Keychain にパスワードを保管して
シェル起動時に読み込む方法があります。**平文のパスワードがシェルの
rc ファイルにも履歴にも残りません。**

### 保存

```bash
# -w を値なしで渡すと、非表示の対話入力になる（履歴にも残らない）
security add-generic-password -a mywant -s mywant-fly -w
```

- `-a` … アカウント名（任意のラベル）
- `-s` … サービス名。取り出すときのキーになる
- **`-w` は必ず最後のオプションにしてください。** 途中に置くと次の引数を
  パスワードとして解釈してしまい、対話入力になりません
- 既に同じ `-a`/`-s` の項目がある場合は `-U` を付けて上書きします
  （付けないとエラーになります）

### 取り出し

```bash
security find-generic-password -s mywant-fly -w
```

### シェルに組み込む

```bash
# ~/.zshrc
export MYWANT_AUTH_PASSWORD=$(security find-generic-password -s mywant-fly -w 2>/dev/null)
```

初回アクセス時に Keychain のアクセス許可ダイアログが出ます。「常に許可」を
選ぶと以降は聞かれません。

### コマンド単位で使う

シェル全体に環境変数を置きたくない場合は、実行時に展開します。
**値が画面にもログにも出ません。**

```bash
MYWANT_AUTH_PASSWORD=$(security find-generic-password -s mywant-fly -w) \
  ./bin/mywant --context fly wants list
```

### 削除

```bash
security delete-generic-password -s mywant-fly
```

## 関連ドキュメント

- [mywant Usage Guide](MYWANT_CLI_USAGE.md) — CLI 全般
- [MyWant CLI Configuration Guide](CONFIG_GUIDE.md) — `config.yaml` の各項目
- [Deploying MyWant on Fly.io](DEPLOY_FLY.md) — 2アプリ構成とデプロイ手順
