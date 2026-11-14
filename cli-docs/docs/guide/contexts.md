# Servers and contexts

The CLI talks to one server at a time. Which one is a **context**, named in `~/.mywant/config.yaml` — the way `kubectl` keeps clusters.

```sh
mywant config get-contexts        # the contexts, the current one marked
mywant config current-context
mywant config use-context fly     # switch
```

## Add one

```sh
mywant config set-context local --server http://localhost:8080
mywant config set-context fly --server https://example.fly.dev \
    --username admin --password-env MYWANT_AUTH_PASSWORD --use
```

Keep passwords out of the file: `--password-env` names an environment variable that holds it, read when the CLI runs.

## For one command

Every command takes the [global flags](/reference/#global-flags):

```sh
mywant --context fly wants list
mywant --server http://localhost:18080 wants list
```

`--server` overrides the context's address; `--context` picks another context without switching to it.

::: tip
`server_host` and `server_port` in the config are for `mywant start`, `stop` and `ps` on this machine — not where the CLI sends its requests. That is the context.
:::
