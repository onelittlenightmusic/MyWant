# Deploying MyWant on Fly.io

This guide deploys MyWant as **two separate Fly.io apps**, matching how the two
tiers actually behave:

| App | Repo | Scaling | Why | Cost |
|-----|------|---------|-----|------|
| **Backend (API)** | this repo | **Always-on** (`min_machines_running = 1`) | Runs the reconcile loop, MonitorAgents, and time-based wants (reminders) that must keep firing even when nobody is connected. | ~$2–3/mo (shared-cpu) |
| **Frontend (GUI)** | [`mywant-gui`](https://github.com/onelittlenightmusic/mywant-gui) | **Autostop / event-driven** (`min_machines_running = 0`) | Static React SPA + API proxy. Needs no CPU while idle; wakes on request. | ~$0 (stopped when idle) |

Splitting them means you only pay for always-on CPU where it's actually
required (the backend), while the lightweight frontend scales to zero.

---

## 1. Backend (this repo)

### Files

- `Dockerfile` — multi-stage build of the `mywant` CLI (static, non-root).
- `docker-entrypoint.sh` — makes the mounted volume writable, drops to a
  non-root user.
- `fly.toml` — always-on config, health check on `/health`, volume mount.

State (`~/.mywant`: `state.yaml`, `memo.yaml`, `achievements.yaml`, `recipes/`,
`custom-types/`) is persisted by pointing `HOME=/data` at a Fly **volume**.

### Deploy

```bash
# 0. Install flyctl and log in
#    https://fly.io/docs/flyctl/install/
fly auth login

# 1. Create the app (pick a unique name; update `app` in fly.toml to match)
fly apps create mywant-backend

# 2. Create the persistent volume in the same region as fly.toml (nrt = Tokyo)
fly volumes create mywant_data --region nrt --size 1   # 1 GB

# 3. Deploy
fly deploy

# 4. Verify
fly status
curl https://mywant-backend.fly.dev/health
# -> {"server":"mywant","status":"healthy",...}
```

### Scaling / cost knobs

- **Memory**: `fly.toml` uses `512mb`. Drop to `256mb` to trim cost, or raise it
  if you see OOM restarts (`fly logs`) under heavier want workloads.
- **Stay always-on**: `min_machines_running = 1` + `auto_stop_machines = false`
  keep the background loops alive. Do **not** set these to scale-to-zero unless
  you accept that reminders/monitors won't fire while stopped.

### Security note

`fly deploy` exposes the backend publicly at `https://<app>.fly.dev`. MyWant's
API has no built-in auth, so consider one of:

- **Keep it private** (recommended): remove the public service and let only the
  frontend reach it over Fly's private network. Replace the `[http_service]`
  block's public exposure with a Flycast/private setup and have `mywant-gui`
  proxy to `http://mywant-backend.internal:8080`. See
  https://fly.io/docs/networking/private-networking/.
- Or put an authenticating proxy / Fly Machines `[[services]]` in front.

---

## 2. Frontend — `mywant-gui` (separate repo)

`mywant-gui` is a separate repository. It serves the React SPA and proxies API
calls to the backend. Deploy it as its own Fly app configured to **autostop**.

Point it at the backend via its API base / proxy target env var (e.g. the
backend's `https://mywant-backend.fly.dev`, or `http://mywant-backend.internal:8080`
if you kept the backend private).

Example `fly.toml` for the GUI app (adjust to that repo's Dockerfile / port):

```toml
app = "mywant-gui"
primary_region = "nrt"

[http_service]
  internal_port = 8080          # match mywant-gui's listen port
  force_https = true
  auto_stop_machines = "suspend"  # scale to zero when idle
  auto_start_machines = true
  min_machines_running = 0        # <-- event-driven: no always-on cost

[[vm]]
  cpu_kind = "shared"
  cpus = 1
  memory = "256mb"
```

`auto_stop_machines = "suspend"` snapshots memory for fast wake-ups;
`min_machines_running = 0` means you pay nothing for compute while idle.

---

## Summary

```
Browser
   │
   ▼
mywant-gui (Fly app, AUTOSTOP)  ──proxy /api──►  mywant-backend (Fly app, ALWAYS-ON)
   React SPA, ~$0 idle                              reconcile loop / agents / reminders
                                                    Volume: /data (~/.mywant)
                                                    ~$2–3/mo
```
