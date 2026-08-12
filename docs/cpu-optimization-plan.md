# CPU 最適化 実装計画（B / C / D / E）

skills-rpg world デプロイ時の CPU 負荷を pprof で計測した結果に基づく実装計画。

計測日: 2026-08-12 / 環境: darwin arm64 10 コア / skills-rpg world 194 wants
(wall 152, rpg_door 17, startpoint 9, next_stage 8, rpg_generator 3, rpg_alarm 2, robot 1, rpg_try_keys 1, model_list 1)

## ベースライン

| 対象 | 実測 |
|---|---|
| システム全体（サーバ稼働時） | 23% busy |
| システム全体（サーバ停止時） | 7% busy |
| **skills-rpg の総コスト** | **約 1.6 コア** |
| うち mywant プロセス内 | 0.206 コア（30.11s で 6.20s samples） |
| うち spawn された python3 子プロセス | 約 1.4 コア |

本書が扱うのは **mywant プロセス内の 0.206 コア**、**デプロイ時のスパイク**、
および **spawn される python3 子プロセス 1.4 コア**（施策 A）。

プロファイルは `MYWANT_PPROF=1 ./bin/mywant start -D` で取得:

```sh
go tool pprof -top -cum ./bin/mywant "http://localhost:6060/debug/pprof/profile?seconds=30"
```

---

## A. MRS 子プロセス — 約 1.4 コア

### A-0. 何が重いのかの切り分け

「python で呼んでいるどの処理を共通化すべきか」を測るため、1 回の MRS 呼び出しを分解した。
（rpg-server の代わりに同形・8KB の JSON を返すスタブを立て、同じスクリプトを 2 通りで実行）

| 内訳 | CPU | 比率 |
|---|---|---|
| インタプリタ起動 + site | 22.5 ms | 39% |
| `import urllib.request` / `json` | 約 34 ms | 61% |
| **実処理（HTTP GET + JSON parse + フィールド抽出）** | **0.11 ms** | **0.2%** |
| 合計（spawn 1 回） | 57 ms | 100% |

計測:

```
spawn 込み  : python3 skill.py '<json>' × 20 回 = 1.141s  → 57 ms/call
常駐ループ内: 同じ GET + parse + 抽出 × 200 回      → 0.11 ms/call
```

**結論: 共通化すべきなのは観測処理そのものではない。** 観測は 0.11ms しかかかっておらず、
コストの **99.8% はインタプリタ起動と import** である。

### A-0-b. 「共通化するのはポーリングか」への答え

ポーリング回数の共通化でも削減はできるが、効き方が違う。

| 案 | 何を減らすか | 呼び出し | 1回あたり | 残コスト |
|---|---|---|---|---|
| 現状 | — | 20 回/秒 | 57 ms | **1.14 コア** |
| **ポーリング共通化**（observer 1 個に集約） | 回数を 1/40 | 0.5 回/秒 | 57 ms | 0.029 コア |
| **プロセス常駐化**（インタプリタ使い回し） | 単価を 1/500 | 20 回/秒 | 0.11 ms | **0.002 コア** |
| 両方 | 両方 | 0.5 回/秒 | 0.11 ms | ほぼ 0 |

常駐化のほうが削減幅が大きく、かつ**汎用**である。

- ポーリング共通化は「17 枚のドアが同じ 1 本の GET を見ている」という skills-rpg 固有の性質に依存する。
  他の MRS プラグイン（spotify / gmail / grafana / smartgolf / event_ticket …計 10 個）は
  **want ごとに引数が違う**ため、そもそも 1 回の取得に畳めない
- 常駐化は引数が違っても効く。全 11 プラグインに一律で効く

したがって **CPU 削減の主施策は常駐化**であり、ポーリング共通化は
（常駐化を入れた後は）CPU ではなく **rpg-server 側の負荷と往復回数**のための施策に位置づけが変わる。

### A-0-c. 計測時、rpg-server は停止していた

計測時点（18:00 頃）の `127.0.0.1:7100` は **Connection refused** だった。
つまり 17 枚のドアは 2 秒ごとに**接続に失敗するだけのプロセスを生成し続けていた**。
上記の 1.4 コアは失敗する観測に費やされていたことになる。

なお失敗時 49ms / 成功時 57ms とコストはほぼ変わらないため、見積りには影響しない。

**現在は復旧済み**（`mywant-rpg server start -D` で detach 起動）。
foreground 起動だとターミナルと運命を共にするため、常駐は `-D` を使うこと。

### A-0-d. 誰が何を、どの頻度で叩いているか

ライブ状態から集計した内訳。実測 **18.9 spawn/秒**（10 秒で子プロセス 189 個）で、
下表の理論値 19.0 とほぼ完全に一致する。

| want type | 個数 | 間隔 | spawn/秒 | リクエスト先 |
|---|---|---|---|---|
| `next_stage` | 8 | **1000 ms** | **8.0** | **通常なし**（`pending_jump` が空なら即 return） |
| `rpg_door` | 17 | 2000 ms | 8.5 | `GET {RPG}/api/v1/state` |
| `rpg_generator` | 3 | 2000 ms | 1.5 | `GET {RPG}/api/v1/state`（同一） |
| `rpg_alarm` | 2 | 2000 ms | 1.0 | `GET {RPG}/api/v1/state`（同一） |
| 合計 30 wants | | | **19.0** | |

読み取れること:

1. **22 wants が完全に同一の URL を叩いている。** しかも引数非依存で、全ステージ分の
   スナップショットを取ってきてから Python 側で自分の `door_id` を抜いている。
   毎秒 11 回、寸分違わぬ同じリクエスト → A-2 で 0.5 req/秒に畳める
2. **`next_stage` は HTTP を叩いていないのに 8 spawn/秒**を出している → A-4 で 0 にできる
3. ドアの間隔 2000ms 自体は妥当。**問題は頻度ではなく 1 回の単価**（57ms の 99.8% が起動と import）

なお want 自身の progress ループは別の頻度で回っている。`GlobalExecutionInterval` = 20ms
なので **50 Hz**。`fetchFrom` は毎サイクル走るため（`want.go:592-612`）、
0.5 回/秒しか更新されないスナップショットから **50 回/秒 同じ 13 フィールドを再抽出**している。
これは A ではなく C-2 / C-4 の領分。

---

### A-1. MRS ランナーの常駐化（エンジン側・汎用）

#### 前提: ランナーは今 2 本ある

プラグインの書き方が 2 世代あり、実行部もそれぞれ別に実装されている。

| | `agent.yaml` 方式（新） | `skill_path` 方式（旧） |
|---|---|---|
| 実行部 | `engine/core/agent_loader.go:470` `mrsRunScript` | `engine/types/agent_monitor_mrs.go:326` `runMRSSkillWithArgs` |
| パッケージ | `engine/core` | `engine/types`（core を dot import） |
| stderr | 捨てる | 捕捉してエラーに含める |
| `_progress` 行 | 捨てる | `onProgress` に転送 |
| 引数 | do は `mrsPluginBuildArgs`、**monitor は nil**（`agent_loader.go:387`） | `mrsBuildArgs`（want ごとに異なる） |
| 使用プラグイン | 8（transit / smartgolf×3 / gmail×2 / grafana / event_ticket） | rpg_door / rpg_device / next_stage / claude_info / spotify |

新方式で共通化されたのは **宣言**（script path・timeout・`state_updates` の
`onFetchData` マッピング）であって、**プロセス起動ではない**。どちらも
`exec.CommandContext(ctx, "python3", …)` を呼び出しごとに実行しており、1 回 57ms は同じ。

したがって rpg_door を `agent.yaml` 方式へ移すだけでは CPU は 1mW も減らない。
ただし**常駐化を入れる場所としては新方式が正しい**（宣言で serve を要求できる）。

#### Step 1: ランナーを 1 本に統合（挙動不変）

`engine/core/mrs_runner.go` を新設し、2 本の上位集合を `RunMRSScript` として export する。

```go
type MRSRunOptions struct {
    Args       []string
    OnProgress func(pct int, msg string)
    Serve      bool
    CacheTTL   time.Duration
}

func RunMRSScript(ctx context.Context, scriptPath string, opt MRSRunOptions) (map[string]any, error)
```

- `mrsRunScript` を削除し、`agent_loader.go:369,387` を差し替え
- `runMRSSkillWithArgs` は `RunMRSScript` の薄いラッパにする（`agent_monitor_mrs.go:185,249` は無変更）
- **この時点で挙動は変わらない。** 先にここまでをテストで固定してから Step 2 に進む

副次効果として、`agent.yaml` 方式のプラグインが stderr と `_progress` を扱えるようになる
（現状 `mrsRunScript` は両方捨てているため、失敗理由が `exit error` しか出ない）。

#### Step 2: 常駐モードとキャッシュ

宣言は既存の `agent.yaml` の語彙を拡張する。新しい概念を足さずに済む。

```yaml
agent:
  metadata: { name: rpg_door_agent, capability: rpg_door, type: monitor }
  script:
    path: ./main.py
    timeout_seconds: 5
    serve: true          # 常駐プロセスとして起動する
    cache_ttl_ms: 2000   # 同一引数の呼び出しを束ねる
    max_procs: 1         # 直列化の単位（既定 1）
```

```go
type MRSScriptDef struct {
    Path           string `yaml:"path"`
    TimeoutSeconds int    `yaml:"timeout_seconds"`
    Serve          bool   `yaml:"serve"`         // 追加
    CacheTTLMs     int    `yaml:"cache_ttl_ms"`  // 追加
    MaxProcs       int    `yaml:"max_procs"`     // 追加
}
```

プロトコルは既存の JSON 行ストリームの延長。`_progress` 行という多重化の前例が
あるので、**stdin にリクエスト行を書き `_id` で対応付ける**のが自然な拡張になる。

```
→ stdin : {"_id":7,"args":["{\"stage_id\":\"stage4\",\"door_id\":\"power_door\"}"]}
← stdout: {"_id":7,"_progress":40,"_message":"fetching"}
← stdout: {"_id":7,"open":false,"key":"blue_key",...}
```

ランナープールは `map[scriptPath]*residentProc`:

- 遅延起動。最初にそのスクリプトを要求した want が立ち上げる
- `sync.Mutex` で 1 プロセスへの要求を直列化（`max_procs` で本数を増やせる）
- `timeout_seconds` を超えたら kill、次回要求時に再起動
- 連続 N 回失敗でバックオフし、その間は spawn 方式にフォールバック
- サーバ停止時に全プロセスを kill（ゾンビを残さない）

#### スクリプト側の契約

後方互換のため **serve 対応は任意**とする。

- 従来どおり `python3 main.py '<json>'` の単発実行も動く（既存プラグインは無改修）
- `MYWANT_MRS_SERVE=1` が渡されたら stdin ループに入る
- **最初の応答に `_id` が無ければ「serve 非対応」と判断し、以後そのスクリプトは
  spawn 方式に固定する** — `serve: true` の書き間違いで壊れない

#### キャッシュキーの当たり外れ

`cache_ttl_ms` は `(scriptPath, args)` をキーにする。ここで効き方が割れる。

| プラグイン | 引数 | キャッシュ |
|---|---|---|
| `agent.yaml` 方式の monitor 全般 | **nil**（`agent_loader.go:387`） | **全 want が完全ヒット** |
| `rpg_door` / `rpg_device` | `door_id` 等が want ごとに違う | **当たらない** |

つまり grafana や gmail のような「引数なし monitor」はランナー層のキャッシュだけで
重複がゼロになるが、**rpg_door には効かない**。同一なのは *HTTP リクエスト* であって
*スクリプトの戻り値* ではないため、ドアの重複排除は A-2（スクリプト内キャッシュ）が要る。

#### Step 3（任意）: ランナーを want として可視化する

Step 2 だけで機能は完結する（プールはプラグインの宣言だけで動く）。
その上で、常駐プロセスを見えるようにしたい場合は `mrs_runner` want type を置く。

- state に `pid` / `requests_total` / `cache_hits` / `restarts` / `last_error` を出せば
  健康状態がカードとして可視化される
- suspend / resume / restart が既存の want 操作でそのまま効く
- 失敗が want history に残る

want が居なくてもプールは動くので、これは観測のための追加であって前提ではない。

なお `engine/core/agent_execution_config.go:11-23` には既に
`ExecutionModeLocal / Webhook / RPC` があるので、`resident` を第 4 のモードとして
足す整理も考えられる。ただし webhook/rpc が「外部サービスを呼ぶ」のに対し
resident は「子プロセスを飼う」ので、責務は近いが同一ではない。
`agent.yaml` の `script.serve` で足りるなら、モードを増やさないほうがよい。

#### Step 4: プラグイン側の serve 対応

エンジン側が終わったら、スクリプトを 1 つずつ移していく。

```python
_snap = {"at": 0.0, "data": None}

def handle(arg):                       # 1 リクエストの処理（既存の main() 相当）
    d = get_state_cached()
    return {"open": ..., "key": ...}

def get_state_cached():
    now = time.monotonic()
    if _snap["data"] is None or now - _snap["at"] > 2.0:
        _snap["data"] = fetch()        # GET /api/v1/state
        _snap["at"] = now
    return _snap["data"]

if os.environ.get("MYWANT_MRS_SERVE") == "1":
    for line in sys.stdin:             # 常駐ループ
        req = json.loads(line)
        out = handle(json.loads(req["args"][0]) if req.get("args") else {})
        out["_id"] = req["_id"]
        print(json.dumps(out), flush=True)
else:
    main()                             # 従来の単発実行（後方互換）
```

`rpg-door` と `rpg-device` はこの `get_state_cached` によって
22 wants の GET が **11 req/秒 → 0.5 req/秒**になる（A-2 の内容）。

**削減見込み: Step 1-2 で 1.08 コア → 0.002 コア。Step 4 で rpg-server の負荷が 1/22**

#### リスクと対策

| リスク | 対策 |
|---|---|
| 常駐プロセスの状態汚染（リクエスト間で変数が持ち越される） | 契約として明文化。キャッシュは意図的、それ以外は持ち越さない |
| ハング / クラッシュ | `skill_timeout_seconds` でタイムアウト → kill → 再起動。連続失敗でバックオフし、その間は spawn 経路に落とす |
| リクエストの直列化 | 1 プロセスが順に処理する。0.11ms なら問題ないが、遅いスキル（`claude_info` のスクレイプ等）は詰まる。プロセス本数を設定可能にするか、遅いものは serve 非対応のままにする |
| `mywant config env set` の反映 | 常駐プロセスには届かない。env 更新時にランナーを再起動する |
| プロセスリーク / ゾンビ | サーバ停止時に確実に kill。`mrs_runner` want の Stop で回収 |

### A-2. 常駐プロセス内のキャッシュ（プラグイン側）

A-1 Step 4 のコード例がこれにあたる。**キャッシュはエンジンの関心事ではない** —
`rpg-door` / `rpg-device` は同じ `GET /api/v1/state` を叩いており（下記 A-4 参照）、
serve ループが TTL 2 秒のスナップショットを 1 個持てば 22 wants 分が 1 回の GET で賄える。

エンジン側の追加実装はゼロ。プラグイン作者の裁量で書ける。

**効果: CPU では誤差（A-1 適用後は 0.002 コアしか残っていない）。
狙いは rpg-server への負荷を 11 req/秒 → 0.5 req/秒 にすることと、往復の削減。**

### A-4. `next_stage` の空振りを止める（A-1 と独立・即効）

観測頻度を調べていて見つかった、**A-1 を待たずに今日消せる 0.44 コア**。

`next_stage`（8 wants、1000ms 間隔 = 8.0 spawn/秒）のスクリプトは、
通常まったく HTTP を叩いていない:

```python
pending = clean(arg.get("pending_jump", ""))
if not pending:
    print(json.dumps({}), flush=True)
    return                      # ← 通常はここで終わり
```

ライブ状態を確認すると 8 個すべて `pending_jump = ''`。
つまり **毎秒 8 個の Python インタプリタが `{}` を出力するためだけに起動**しており、
`import urllib.request`（22.5ms）まで済ませてから使わずに終了している。

エンジンに既にゲート機構がある。`mrsCheckRequiredParams` は
`runMRSSkillWithArgs` を呼ぶ**前**に return するので、**プロセスが起動しない**
（`engine/types/agent_monitor_mrs.go:160-162`）。

```yaml
# next_stage の want type YAML に 1 行
skill_required_params: pending_jump
```

現在 8 個とも `skill_required_params` が未設定。これを入れるだけで
**8 spawn/秒 → 0**（`pending_jump` が入った瞬間だけ起動）。

**削減見込み: 0.44 コア。変更は YAML 1 行。**

### A-3.（不採用）observer want + imports への作り替え

当初検討した「observer want が全体を取得し、各 door が `Spec.Imports` で受け取る」案。
調査した範囲では機構は揃っている:

- door want は top-level（`ownerReferences` なし）なので `Spec.Imports` は
  global state から解決される（`engine/core/want.go:1517-1527`）
- `fetchFrom` / `onFetchData` は state のフラットマップを読むだけの汎用機構
  （`want.go:600`）なので、observer が `mrs_raw_output` 相当を書けば
  door 側の state 定義 21 行は無改修で動く
- ただし `onFetchData` は `extractJSONPath` の静的ドットパスで **`%{}` 展開がない**
  （`engine/core/script_runtime.go:131`）。1 個の共有スナップショットから
  want ごとに違うパスを抜くには、observer 側で want ごとのキーに展開して
  publish するか、`onFetchData` に param 展開を足す必要がある
- `StoreGlobalState` は呼ぶたびに global_state.yaml を marshal + 書き込みするため
  （`engine/core/global_state.go:46,105-135`）、1 tick 1 回の `MergeGlobalState` に
  まとめないと B と同じ問題を作る

**不採用の理由:** A-1 を入れれば CPU 的な効果はほぼ残らない一方、
world の want グラフと rpg_door の型定義に手を入れる必要があり、変更範囲が大きい。
rpg-server への往復を減らしたいだけなら A-2 で足りる。

---

## B. writeStatsToMemory — 19.2%（0.040 コア）

### 現象

`reconcileLoop` の statsTicker が **毎秒** 全 194 wants を state.yaml（240KB）へ YAML 化している。

```
mywant/engine/core.(*ChainBuilder).writeStatsToMemory   1.19s (19.2%)
  ├ gopkg.in/yaml.v3.Marshal          0.80s
  ├ (*Want).storeStateMulti           0.13s
  ├ (*Want).BuildHistory              0.08s
  ├ (*Want).GetSpec                   0.06s
  ├ (*Want).GetAllStateDeep           0.04s
  ├ os.WriteFile                      0.02s
  └ crypto/md5.Sum                    0.01s
```

### 原因

`engine/core/chain_builder_file_sync.go:129-140` — 変更検知の md5 比較が **marshal した後**にある。

```go
data, err := yaml.Marshal(updatedWants)   // ← 240KB 分のコストを常に払う
statsHash := fmt.Sprintf("%x", md5.Sum(data))
if statsHash == cb.lastStatsHash {
    return                                // ← ここで初めて「変化なし」と分かる
}
```

skills-rpg は 152 個の `wall` が不変なので、大半のフレームで結果は捨てられている。

### 施策 B-1: 変更カウンタで marshal 前に打ち切る（本命）

`engine/core/want.go:997` の `StoreState` は **既に「値が変わっていなければ return」している**（`want.go:1006`）。
つまりここが「実際に state が変わった」唯一の関門なので、グローバルな `atomic.Uint64` を 1 つ置く。

- `want.go:1011`（`n.State.Store` の直後）で `stateEpoch.Add(1)`
- 同様に `SetStatus` / `SetLabel` / `SetLabels` / `Metadata.OrderKey` 更新でもインクリメント
- `writeStatsToMemory` の冒頭で `stateEpoch.Load()` を読み、前回書き込み時の値と同じなら **即 return**

判定は O(1)。`lastStatsHash` による md5 比較は保険としてそのまま残す。

**削減見込み: 0.035 コア（19.2% → 1% 未満）**

注意点:
- カウンタに乗せ忘れた変更経路があると **永続化漏れ = データ損失**になる。State / Status / Labels / OrderKey / Spec の全書き込み経路を洗うこと
- `History`（`BuildHistory`）は state 変更に追随するので `StoreState` 経由で捕捉できるが、ログ追記のみのケースを要確認

### 施策 B-2: 変更 want のみ再構築

B-1 で「全体として変化なし」は弾けるが、1 個でも変われば 194 wants 全部を作り直す。
want ごとの epoch を持ち、前回のスナップショット（`[]*Want`）を保持して差分だけ更新する。

`storeStateMulti` + `BuildHistory` + `GetSpec` + `GetAllStateDeep` = 0.31s（全体の 5%）が対象。
ただし `yaml.Marshal` は全体を舐めるため、ここは削れない。

**削減見込み: 追加で 0.008 コア** — B-1 の後なら効果は小さい。優先度は低い。

### 施策 B-3: statsTicker の間隔を伸ばす

`engine/core/chain_builder.go:42` の `GlobalStatsInterval = 1 * time.Second` を 3〜5s に。

線形に削減できるが、クラッシュ時に失う状態が増える。
なお API 起点の変更は `TriggerSave` → `debouncedSaveLoop`（`chain_builder.go:523-559`, debounce 3s）で別途永続化されるため、
statsTicker が担っているのは **want ランタイム由来の state 変更のみ**。

B-1 を入れれば不要。B-1 が難しい場合の代替として位置づける。

---

## C. StartProgressionLoop — 14.5%（0.030 コア）

### 現象

194 wants × `GlobalExecutionInterval` 20ms = **約 9,700 progress cycle/秒**。

```
(*Want).StartProgressionLoop.func1              0.90s (14.5%)
  ├ runProgressWithRecovery                     0.49s
  │   ├ reflect.Value.MethodByName              0.18s  ┐
  │   ├ reflect.Value.Call                      0.07s  ┘ 合計 4.0%
  │   ├ (*ScriptableWant).Progress              0.11s → ExecuteAgents 0.10s
  │   ├ (*SchedulerWant).Progress               0.08s
  │   └ SyncLocalsState                         0.03s
  ├ (*ScriptableWant).IsAchieved                0.11s
  ├ (*Want).EndProgressCycle                    0.11s
  └ (*Want).SetPaths                            0.05s
```

### 施策 C-1: `GetLocals` の reflection をキャッシュ（本命・最小変更）

`engine/core/want_loop.go:94-101` が **毎サイクル** メソッドを名前解決している。

```go
if progressableVal := reflect.ValueOf(n.progressable); progressableVal.Kind() == reflect.Pointer {
    method := progressableVal.MethodByName("GetLocals")   // ← 9,700 回/秒
    if method.IsValid() && method.Type().NumIn() == 0 && method.Type().NumOut() == 1 {
```

`reflect.Value.MethodByName` はメソッド集合の線形探索 + 文字列比較で、1 回あたり ~1µs。

- `Want` に `getLocalsMethod reflect.Value` + `getLocalsResolved bool` を持たせる
- `initializeForRun()`（`want.go:762` から呼ばれる）で 1 回だけ解決
- `n.progressable` が差し替わったらリセット

`MethodByName` の 0.18s がほぼ消える。`Call` の 0.07s は残る（戻り値の `[]reflect.Value` 確保）。

**削減見込み: 0.006 コア（全体の 3.0%）** — 変更行数あたりの効果が最も高い。

### 施策 C-2: want ごとの実行間隔（効果は最大、要設計）

`engine/core/chain_builder.go:37` の `GlobalExecutionInterval = 20 * time.Millisecond` はグローバル定数。
`wall` 152 個は静的な壁タイルで、20ms ごとに評価する意味がない。

- want type YAML に `execution_interval_ms` を追加
- `want.go:912` / `787` / `838` / `849` / `859` の `time.Sleep(GlobalExecutionInterval)` を want 固有値に
- 未指定なら現行の 20ms

`wall` を 500ms にすれば、152/194 の want のループコストが 1/25 になる。

**削減見込み: 0.015〜0.020 コア（14.5% → 3% 程度）**

注意点:
- 間隔はレイテンシに直結する。ユーザー操作に応答する want（`rpg_door`, `robot`, `next_stage`）は 20ms を維持
- 制御シグナル（`CheckControlSignal`）の受信も遅れるため、stop/suspend の応答が最大 interval 分遅れる

### 施策 C-3: `FindAgentsByGives` のキャッシュ

`ExecuteAgents`（0.10s）のうち `FindAgentsByGives` が 0.03s。
エージェントを持たない want でも毎サイクル探索している。

want ごとに「解決済みエージェント一覧」をキャッシュし、agent registry の世代が変わったときだけ再計算。

**削減見込み: 0.005 コア**

### 施策 C-4: `fetchFrom` / derived fields のスキップ

`EndProgressCycle`（0.11s）の内訳は `evaluateDerivedFields` 0.03s + `extractJSONPath` 0.03s + `getState` 0.02s。

`want.go:592-612` は毎サイクル `WantTypeDefinition.State` を全走査している。
`FetchFrom`/`OnFetchData` を持つフィールドが 0 個の want type（`wall` など）では初回にフラグを立ててスキップ。

**削減見込み: 0.003 コア**

---

## D. デプロイ時の O(n²) — 1 秒に 0.42s 集中

### 現象

`world open skills-rpg` 実行中の 25 秒プロファイルで:

```
(*ChainBuilder).processWantOperation      0.44s
  └ (*ChainBuilder).DeleteWantByID        0.42s
      └ (*ChainBuilder).copyConfigToMemory 0.42s
          ├ gopkg.in/yaml.v3.Marshal       0.25s
          └ os.WriteFile                   0.17s
```

実際の削除処理は約 1 秒で完了しているため、その 1 秒間の CPU がこれで占められている。

### 原因

`engine/core/chain_builder_control.go:264` — want を 1 個削除するたびに config 全体を YAML 化して state.yaml を丸ごと書き直す。

194 wants の入れ替えで **240KB × 194 回 ≒ 47MB の marshal + 書き込み**。

さらに `chain_builder_control.go:231-242`（Phase 2）が削除ごとに全 want を走査して子を探すため、こちらも O(n²)。

### 施策 D-1: バルク削除の永続化を 1 回にまとめる

- `copyConfigToMemory` を呼ばない `deleteWantByIDLocked(wantID string)` を切り出す
- `DeleteWantByID` は従来どおり単体削除 + persist（既存 API 互換）
- `DeleteWantsByIDs(ids []string)` を追加し、ループの**最後に 1 回だけ** `copyConfigToMemory`
- `processWantOperation` の world 入れ替え経路をこちらに向ける

**削減見込み: 0.42s → 0.002s（約 200 分の 1）**

### 施策 D-2: 子 want 探索のインデックス化

Phase 2 の全 want 走査を、バルク削除の開始時に `親ID → 子IDリスト` のマップを 1 回作って再利用する。

**削減見込み: 現状のプロファイルには顕著に出ていない（0.42s の大半は persist）が、want 数が増えると効く**

### 検証

`world open` 前後で以下を比較:

```sh
curl -s -o deploy.pprof "http://localhost:6060/debug/pprof/profile?seconds=25" &
./bin/mywant world open skills-rpg
go tool pprof -peek "copyConfigToMemory" ./bin/mywant deploy.pprof
```

state.yaml の内容が施策前後で一致することを確認する（`world open` → `diff` ）。

---

## E. getLabels — デプロイ / GUI リフレッシュ時 4.8%（定常時 1.1%）

### 現象

```
(*Server).getLabels                 0.30s (4.8%)
  └ getLabels.func1                 0.28s
      ├ (*Want).GetLabels           0.17s
      └ (*ChainBuilder).GetAllWantStates 0.09s
```

定常時は HTTP 全体で 1.13% しか出ないので、これは **GUI が world 切り替え後に再取得するとき**のコスト。

### 原因

`engine/server/handlers_others.go:997-1029` が `(key, value)` ペアごとに全 want を舐める三重ループ。

実測カーディナリティ:

| | 数 |
|---|---|
| label keys | 16 |
| (key, value) ペア | 102 |
| うち `mywant.io/canvas-x` | 30 values |
| うち `mywant.io/canvas-y` | 40 values |

1 リクエストあたり:
- `GetAllWantStates()` × 102 回（毎回 194 エントリの map をコピー）
- `want.GetLabels()` × **19,788 回**（毎回 map を alloc: `want.go:1882`）

### 施策 E-1: ループを反転して 1 パスに（本命）

wants を 1 回だけ走査し、`(k,v) → owners/users` の index を組み立ててから応答を構築する。

```go
type ownerUsers struct{ owners, users map[string]bool }
index := map[[2]string]*ownerUsers{}

states := builder.GetAllWantStates()        // 1 回だけ
for _, want := range states {
    for k, v := range want.GetLabels() {    // want ごとに 1 回だけ
        index[[2]string{k, v}].owners[want.Metadata.ID] = true
    }
    for _, u := range want.Spec.Using {
        for k, v := range u.Labels {
            index[[2]string{k, v}].users[want.Metadata.ID] = true
        }
    }
}
```

- `GetLabels()` 19,788 回 → **194 回**
- `GetAllWantStates()` 102 回 → **1 回**

**削減見込み: 50〜100 倍。応答内容は不変（既存レスポンスとの一致をテストで固定する）**

### 施策 E-2: 座標ラベルを集計対象から外す

102 ペア中 **70 個が `canvas-x` / `canvas-y`**。タイル座標なのでマップが広がるほど単調に増える構造的な増加要因。

`GetRegisteredLabels()` の結果から `mywant.io/canvas-` 接頭辞を除外するか、
owners/users の解決対象外にする（キー自体は返す）。

GUI 側がこれらの owners/users を使っていないことの確認が前提。

**削減見込み: E-1 適用後の残りをさらに 1/3 に**

### 施策 E-3: ETag / キャッシュ

`engine/server/etag.go` は labels 未対応。ラベル変更時のみ再計算するキャッシュを噛ませれば、
GUI のポーリングは実質ゼロコストになる。E-1 で十分速くなれば不要。

---

## 実施順序

低リスク・局所的なものから。

| 順 | 施策 | 削減 | 変更規模 | リスク |
|---|---|---|---|---|
| 0 | ~~rpg-server を起動する~~（対応済: `server start -D`） | — | 運用 | — |
| 1 | **A-4** `skill_required_params: pending_jump` | **0.44 コア** | YAML 1 行 | 低 |
| 1 | **A-1 Step 1** ランナー 2 本を 1 本に統合（挙動不変） | 0 | 中 | 低 |
| 1 | **A-1 Step 2** `serve` / `cache_ttl_ms` | **0.64 コア** | 大 | 中（プロセス管理） |
| 1 | **A-1 Step 4** rpg-door / rpg-device の serve 対応 | rpg-server 負荷 1/22 | 小 | 低 |
| 2 | **E-1** getLabels 1 パス化 | GUI 更新 50-100x | 1 関数 | 低（応答一致テストで担保） |
| 3 | **D-1** バルク削除の persist 1 回化 | デプロイ 200x | 1 ファイル | 低〜中（永続化漏れに注意） |
| 4 | **C-1** GetLocals reflection キャッシュ | 0.006 コア | 数十行 | 低 |
| 5 | **B-1** 変更カウンタで marshal 前打ち切り | 0.035 コア | 中 | **中**（漏れ = データ損失） |
| 6 | **C-2** want ごとの実行間隔 | 0.015-0.020 コア | 中 | 中（レイテンシ設計が必要） |
| 7 | A-2 プラグイン内キャッシュ / C-3 / C-4 / E-2 / B-2 | CPU では誤差 | 小 | 低 |

A-1 だけが一桁大きい。まずここを取り、その後 B〜E で足元を整える。

| 段階 | プロセス内 | 子プロセス | 合計 |
|---|---|---|---|
| 現状 | 0.206 | 1.4 | **約 1.6 コア** |
| A-1 適用後 | 0.18 | 0.002 | **約 0.18 コア** |
| + E-1 / D-1 / C-1 / B-1 | 0.13 | 0.002 | **約 0.13 コア** |
| + C-2 | 0.11 | 0.002 | **約 0.11 コア** |

（A-1 を入れるとプロセス内の `os/exec` 分 14% も同時に消えるため、
プロセス内も 0.206 → 0.18 に下がる）

## 実装結果（2026-08-12 実測）

A-1 / A-4 / B-1 / C-1 / C-3 / C-4 / D-1 / E-1 を実装し、同一条件（skills-rpg 194 wants、
30 秒プロファイル）で前後を比較した。

### 子プロセス

| | before | after |
|---|---|---|
| spawn 率 | **18.9 /秒** | **0.2 /秒** |
| 常駐プロセス | なし | 2 本（rpg-door / rpg-device が全 22 wants を処理） |
| 常駐プロセスの CPU | — | 0.3% / 0.2%（累計 0.19s / 0.16s） |

22 wants 分の `GET /api/v1/state` が 1 本のスナップショット（TTL 1.5s）に集約された。
`next_stage` は `skill_required_params` により完全に起動しなくなった。

### プロセス内（cum%、30 秒プロファイル）

| 施策 | 対象 | before | after | 削減 |
|---|---|---|---|---|
| A-1 | `monitorMRSAgentFn` | 11.29% | 1.15% | **10.1pt** |
| A-1 | `os/exec.Cmd.Start` | 8.39% | 0% | **8.4pt** |
| B-1 | `writeStatsToMemory` | 19.19% | 6.57% | **12.6pt** |
| B-1 | └ `yaml.Marshal` | 12.90% | 4.60% | 8.3pt |
| C-1 | `reflect …MethodByName` | 2.90% | 0% | **2.9pt** |
| E-1 | `getLabels` | 4.82% | 0.99% | **3.8pt** |

全テスト通過（`go test ./...` in `engine/`）。

---

## 追加で見つかったもの（実装後の再計測から）

上記まででプロセス内は 20.6% → 14.3% に下がったが、`writeStatsToMemory` が 6.57% 残った。
その理由を追ったところ、施策そのものではなく **B-1 のガードをすり抜ける書き方をしていた
want が原因**だと分かった。以下はいずれも当初の計画になかった発見である。

### F-1. `wall` がループを止められていなかった（0.44 コア相当）

`wall.yaml` には `state` も `achievedWhen` も無く、`ScriptableWant.IsAchieved()` が
最終行の `GetCurrent(&s.Want, "achieved", false)` に落ちて**永久に false** を返していた
（`scriptable_want.go:194`）。152 個すべてが `reaching` のまま 20ms で回り続けていた。

一方 `StartProgressionLoop` は `IsAchieved()` が true になると最終サイクルを 1 回だけ回して
**goroutine ごと return** する（`want.go:801-817`）。つまり機構は既にあった。

`wall.yaml` に `achieved: true` の state を宣言しただけで:

| | before | after |
|---|---|---|
| goroutine 総数 | 247 | **97** |
| `StartProgressionLoop` | 15.93% | 7.44% |
| プロセス CPU | ~20% | **9.8%** |

**副作用:** wall の status が `achieved` になるため、`CanvasTileCard.tsx:144-147` が
active とみなす条件から外れ、タイルの光彩が消える。壁が動作中の want のように
光っているほうが不自然ではあるが、意図した変更ではない。

`startpoint`（9 個、同じく「Purely positional」な canvas 固定物）は未対応。

### F-2. 自分で自分を変更し続ける警告メッセージ

state.yaml は 1.00 回/秒・275KB で書かれ続けていた。連続する 2 つを diff すると
差分は **36 行だけ**で、全部これだった:

```
< governance_warning_message: 1 state access policy violation(s) in cycle 11745 …
> governance_warning_message: 1 state access policy violation(s) in cycle 11843 …
```

`next_stage` 8 個が毎サイクル governance 違反を起こしており、その警告メッセージに
**実行サイクル番号が埋め込まれていた**（`want.go:595`）。意味は変わらないのに文字列は
毎回変わるので、`StoreState` は本物の変更と判断し、エポックも md5 も常にすり抜けていた。

- メッセージからサイクル番号を削除（サイクル番号はログ行に残る）
- `setGovernanceWarning` に同一警告のスキップを追加 — 同じ違反を 50回/秒 記録しても
  誰も何も分からない

GOVERNANCE ログが **約 400 行/秒 → 8 行/秒** に。

なお **`next_stage` が実際にガバナンス違反を起こしている**（子 want が親 state へ
許可なく書き込み）ことが判明した。ログと永続化の騒音を止めただけで、違反自体は未修正。

### F-3. 4 バイトのハートビートが 275KB を書き直していた

F-2 を直すと次の犯人が出た。10 秒間の変化を集計すると:

```
10/10  scheduler/system-scheduler   ← last_scan_time
 3/10  robot/robot                  ← last_poll_at
```

`SchedulerWant.Progress()` が毎サイクル `last_scan_time = time.Now().Unix()` を書いており、
秒解像度なのでちょうど 1 秒ごとに値が変わる。生存確認のためのタイムスタンプ 1 つで
ファイル全体が毎秒書き直されていた。

`schedulerScanTimeResolution = 60` を導入し、60 秒に 1 回だけ記録するようにした。
生存確認は分単位で十分で、churn は 60 分の 1 になる。

### F-4. `GlobalStatsInterval` 1s → 5s

全リポジトリを検索した結果、この定数の用途は `statsTicker`（`chain_builder.go:470`）
**1 箇所のみ**。`world save` はメモリ上の want から直接 marshal しており
（`handlers_worlds.go:152`）state.yaml を経由しない。API 起点の変更も
`TriggerSave` → `debouncedSaveLoop`（3s debounce）という別経路。

CPU 上の直接的な削減はほぼ無い（F-2/F-3 で churn 自体が消えたため）。
入れる価値は **「1 つの want が不用意にハートビートを書いたときの最悪コストを
5 分の 1 に抑える上限」** としてである。実際 F-3 は scheduler の 1 フィールドだけで
全体を 1 回/秒に張り付かせていた。

トレードオフ: 異常終了時に失う want ランタイム状態が最大 1 秒 → 5 秒。

---

## 最終結果

| | 開始時 | 最終 |
|---|---|---|
| プロファイル全体 | 6.20s / 30s (**20.6%**) | 2.35s / 30s (**7.8%**) |
| プロセス CPU | ~20% | **5.7%** |
| goroutine | 247 | **97** |
| 子プロセス spawn | 18.9 /秒 | **0.2 /秒** |
| state.yaml 書き込み | 1.00 回/秒 (275KB) | **0.20 回/秒** |

子プロセス側の約 1.4 コアを含めると、当初 1.6 コア規模だった負荷がほぼ解消した。

### 残る構造的な課題

**1 つの want が 4 バイト変えると 275KB 全体が書き直される。** これを本当に解くには
want ごとのファイル分割（増分永続化）が必要で、読み込み・world export/import まで
波及する。F-4 の上限はその緩和策にすぎない。

### 未実施

- **C-2**（want ごとの実行間隔）— レイテンシ設計が必要なため見送り。ただし `wall` 152 個は
  F-1 で goroutine ごと停止したため、当初の想定より問題は小さくなっている
- **`fetchFrom` のソース未更新スキップ** — C-4 は「fetchFrom を持たない want type で
  ループごとスキップ」までに留めた。「ソースの timestamp が変わるまで再抽出しない」案は
  派生フィールドを外部から上書きしたときの挙動が変わるため、意図的に見送っている
- **E-2**（canvas-x/y をラベル集計から除外）— E-1 で十分速くなったため保留
- **B-2 / A-2 の横展開** — 他 8 プラグインの serve 対応は未着手

## 共通の検証手順

各施策の前後で同一条件のプロファイルを取り、対象関数の cum% を比較する。

```sh
# ベースライン
MYWANT_PPROF=1 ./bin/mywant start -D
curl -s -o before.pprof "http://localhost:6060/debug/pprof/profile?seconds=30"

# 施策適用後
go tool pprof -top -cum ./bin/mywant after.pprof
go tool pprof -base before.pprof ./bin/mywant after.pprof   # 差分表示
```

機能面の回帰確認:

```sh
cd engine && go test ./core/... ./server/...
./bin/mywant world open skills-rpg && ./bin/mywant wants list   # 194 wants
```
