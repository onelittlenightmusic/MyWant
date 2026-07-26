#!/bin/sh
# Ensure the persistent volume (mounted at $HOME=/data, owned by root by Fly)
# is writable by the non-root `app` user, then drop privileges and exec.
set -e

: "${HOME:=/data}"
mkdir -p "$HOME"

if [ "$(id -u)" = "0" ]; then
    chown -R app:app "$HOME" 2>/dev/null || true
    # su-exec resets HOME to the target user's /etc/passwd entry, which would
    # send ~/.mywant to the ephemeral /home/app instead of the mounted volume.
    exec su-exec app env HOME="$HOME" "$@"
fi

exec "$@"
