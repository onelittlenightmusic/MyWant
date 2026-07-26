# Deploying MyWant on Fly.io

MyWant runs on Fly.io as **two apps**, built from **two source repositories**,
deployed by a **third repository** that owns the environment configuration.

```
Browser
   │
   ▼
mywant-gui (Fly app, AUTOSTOP, ~$0 idle)  ──proxy /api──►  mywant-backend (Fly app, ALWAYS-ON)
  embedded React SPA + reverse proxy         private network   reconcile loop / agents / reminders
                                                              Volume: /data (~/.mywant), ~$2–3/mo
```

| App | Scaling | Why |
|-----|---------|-----|
| **Backend (this repo)** | **Always-on** (`min_machines_running = 1`) | Runs the reconcile loop, MonitorAgents, and time-based wants (reminders), which must keep firing even when nobody is connected. |
| **GUI** ([`mywant-gui`](https://github.com/onelittlenightmusic/mywant-gui)) | **Autostop** (`min_machines_running = 0`) | Stateless SPA + API proxy. No CPU needed while idle; wakes on request. |

## Why deployment lives in a separate repository

This repository builds a **container image**. It does not deploy it, and holds
no Fly.io credential.

The `fly.toml` files, app names, regions, scaling knobs, and the Fly API token
live in **[`mywant-deploy`](https://github.com/onelittlenightmusic/mywant-deploy)**
(private). Two reasons:

1. **The two apps must stay in lockstep.** The GUI's `MYWANT_BACKEND` has to
   match the backend's app name, so the pair only makes sense configured
   together. Split across two source repos, that coupling is invisible.
2. **This repository is public.** Deployment specifics and the Fly credential
   stay out of it.

This mirrors how releases already work here: the source repo produces the
artifact, a separate repo publishes it (`homebrew-mywant`, `mywant-gui-dist`).

## What this repository provides

- `Dockerfile` — multi-stage build of the `mywant` CLI (static, `CGO_ENABLED=0`,
  non-root runtime on alpine). Same build invocation as `make build-cli`.
- `docker-entrypoint.sh` — makes the Fly-mounted volume writable, then drops to
  a non-root user. Preserves `HOME` across `su-exec` so `~/.mywant` lands on the
  volume rather than the container's ephemeral layer.
- `.dockerignore` — limits the build context to `engine/` and `client/`.
- `.github/workflows/image.yml` — builds `linux/amd64` and pushes to
  `ghcr.io/<owner>/mywant-backend` on every push to the default branch, tagged
  `sha-<commit>` and `latest`.

### State persistence

State (`~/.mywant`: `state.yaml`, `memo.yaml`, `achievements.yaml`, `recipes/`,
`custom-types/`) is persisted by pointing `HOME` at a Fly **volume** mounted at
`/data`. The volume, mount, and `HOME` env are configured in `mywant-deploy`.

Note that `su-exec` resets `HOME` to the target user's `/etc/passwd` entry, so
`ENV HOME=/data` alone is not enough — `docker-entrypoint.sh` re-applies it
explicitly. Without that, every restart and redeploy silently loses all state.

## Building the image locally

```bash
docker build -t mywant-backend .
docker run --rm -p 8080:8080 -v mywant-data:/data mywant-backend
curl http://localhost:8080/health
# -> {"server":"mywant","status":"healthy",...}
```

## Deploying

See the README in `mywant-deploy`. In short: that repo pulls the published
image, retags it into `registry.fly.io`, and runs `flyctl deploy --image`
against its own `fly.toml`. Deployment is triggered manually, on a config
change, or by a `repository_dispatch` from this repo's image workflow (enabled
by setting a `DEPLOY_DISPATCH_TOKEN` secret here; without it the step no-ops).

## Security note

MyWant's API has **no authentication**. The backend is therefore deployed with
**no public IP** — only a private ingress address — so it is reachable solely
from the GUI over Fly's private network (`http://<backend-app>.internal:8080`).

Be aware that this protects the backend from *direct* access only. The GUI
proxies `/api/*` publicly, so anyone who knows the GUI's URL can still create
and delete wants. Put authentication in front of the GUI before treating this
deployment as anything other than a personal sandbox.
