# MyWant backend — single always-on container for Fly.io.
# Serves the API (engine/server) only; the GUI (mywant-gui) is deployed
# separately as an autostop/event-driven app. See docs/DEPLOY_FLY.md.

# ---- Build stage ----------------------------------------------------------
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Download modules first for better layer caching. The client module uses a
# local `replace mywant/engine => ../engine`, so engine's go.mod/go.sum must be
# present before `go mod download` runs in client.
COPY engine/go.mod engine/go.sum ./engine/
COPY client/go.mod client/go.sum ./client/
RUN cd client && go mod download

# Copy sources and build the static CLI binary (same invocation as `make build-cli`).
COPY engine/ ./engine/
COPY client/ ./client/
RUN cd client && CGO_ENABLED=0 GOOS=linux go build \
        -trimpath -ldflags="-s -w" \
        -o /out/mywant ./cmd/mywant

# ---- Runtime stage --------------------------------------------------------
FROM alpine:3.20

# ca-certificates: outbound HTTPS (agents/APIs). su-exec: drop root in entrypoint.
# tzdata: correct time handling for time-based wants (reminders).
RUN apk add --no-cache ca-certificates su-exec tzdata \
    && adduser -D -u 10001 app

COPY --from=builder /out/mywant /usr/local/bin/mywant
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# State (~/.mywant) lives under HOME, which we point at the mounted volume so it
# survives restarts and redeploys.
ENV HOME=/data
EXPOSE 8080
VOLUME ["/data"]

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["mywant", "start", "--host", "0.0.0.0", "--port", "8080"]
