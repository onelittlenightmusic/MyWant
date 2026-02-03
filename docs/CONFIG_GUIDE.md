# MyWant CLI Configuration Guide

## Overview

The `mywant config` command provides an interactive way to configure your MyWant environment, including agent execution modes, server addresses, and ports.

## Configuration File

All configuration is stored in: `~/.mywant/config.yaml`

### Configuration Options

| Option | Description | Default | Values |
|--------|-------------|---------|--------|
| `agent_mode` | Agent execution mode | `local` | `local`, `webhook`, `grpc` |
| `server_host` | Main server hostname | `localhost` | Any valid hostname |
| `server_port` | Main server port | `8080` | 1-65535 |
| `agent_service_host` | Agent service hostname | `localhost` | Any valid hostname |
| `agent_service_port` | Agent service port | `8081` | 1-65535 |
| `mock_flight_port` | Mock flight server port | `8090` | 1-65535 |

## Commands

### Interactive Configuration

Set configuration values interactively:

```bash
./mywant config set
```

Example session:
```
🔧 MyWant Configuration
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Agent Execution Mode [local]:
  1) local   - In-process execution (fastest, default)
  2) webhook - External HTTP service (language-agnostic)
  3) grpc    - gRPC service (high-performance)
Select (1-3) or press Enter to keep current: 2

Main Server Host [localhost]:
Main Server Port [8080]:

Agent Service Settings (for webhook/grpc mode):
Agent Service Host [localhost]:
Agent Service Port [8081]: 8082

Mock Flight Server Port [8090]:

✅ Configuration saved to /Users/user/.mywant/config.yaml
```

### View Current Configuration

Display the current configuration:

```bash
./mywant config get
# Aliases: ./mywant config show, ./mywant cfg g
```

Output:
```
📋 Current Configuration
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Agent Mode:         webhook
Server:             localhost:8080
Agent Service:      localhost:8082
Mock Flight Port:   8090
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Config file: /Users/user/.mywant/config.yaml
```

### Reset to Defaults

Reset configuration to default values:

```bash
./mywant config reset
# Alias: ./mywant cfg r
```

### Edit Configuration File

Edit the configuration file directly:

```bash
# Opens config file in $EDITOR (default: vim)
./mywant config edit
# Alias: ./mywant cfg e

# Or edit manually
vim ~/.mywant/config.yaml
```

## Agent Execution Modes

### 1. Local Mode (Default)

Agents execute in the same process as the main server.

**Pros:**
- Fastest execution (no network overhead)
- Simplest setup
- Best for development

**Cons:**
- All agents must be Go-based
- Scales vertically only

**Configuration:**
```yaml
agent_mode: local
server_host: localhost
server_port: 8080
```

**Usage:**
```bash
./mywant config set  # Select option 1
./mywant start -D
```

### 2. Webhook Mode

Agents execute as external HTTP services.

**Pros:**
- Language-agnostic (Python, Node.js, etc.)
- Horizontal scaling
- Fault isolation

**Cons:**
- Network latency
- Requires Agent Service

**Configuration:**
```yaml
agent_mode: webhook
server_host: localhost
server_port: 8080
agent_service_host: localhost
agent_service_port: 8081
```

**Usage:**
```bash
# Configure webhook mode
./mywant config set  # Select option 2

# Start main server
./mywant start -D

# Start agent service
./mywant start --worker -D
```

### 3. gRPC Mode (Planned)

High-performance agent execution via gRPC.

**Pros:**
- High performance
- Bidirectional streaming
- Type safety

**Cons:**
- More complex setup
- Requires protobuf definitions

**Configuration:**
```yaml
agent_mode: grpc
server_host: localhost
server_port: 8080
agent_service_host: localhost
agent_service_port: 8081
```

## Configuration Workflow

### Initial Setup

1. **Set up configuration:**
   ```bash
   ./mywant config set
   ```

2. **Verify configuration:**
   ```bash
   ./mywant config get
   ```

3. **Start services:**
   ```bash
   # Main server (uses config automatically)
   ./mywant start -D

   # Agent service (if using webhook/grpc mode)
   ./mywant start --worker -D
   ```

### Override Configuration

Command-line flags override configuration file values:

```bash
# Use config file values
./mywant start -D

# Override port (ignores config file)
./mywant start -D --port 9090

# Override host and port
./mywant start -D --host 0.0.0.0 --port 9090
```

### Multiple Environments

Create environment-specific configurations:

```bash
# Development (local mode)
./mywant config set
# Select: local mode, port 8080

# Production (webhook mode, remote server)
./mywant config set
# Select: webhook mode, port 80, agent service on 8081
```

## Example Configurations

### Development Setup (Local Mode)

```yaml
agent_mode: local
server_host: localhost
server_port: 8080
agent_service_host: localhost
agent_service_port: 8081
mock_flight_port: 8090
```

### Production Setup (Webhook Mode)

```yaml
agent_mode: webhook
server_host: 0.0.0.0
server_port: 80
agent_service_host: agent-service.local
agent_service_port: 8081
mock_flight_port: 8090
```

### Distributed Setup (Multi-Host)

```yaml
agent_mode: webhook
server_host: 0.0.0.0
server_port: 8080
agent_service_host: agents.mywant.internal
agent_service_port: 8081
mock_flight_port: 8090
```

## Troubleshooting

### Configuration Not Loading

Check if config file exists:
```bash
ls -la ~/.mywant/config.yaml
cat ~/.mywant/config.yaml
```

### Invalid Configuration

Reset to defaults and reconfigure:
```bash
./mywant config reset
./mywant config set
```

### Port Conflicts

Check which ports are in use:
```bash
./mywant ps
lsof -i :8080
lsof -i :8081
```

Change ports in configuration:
```bash
./mywant config set
# Enter new port numbers
```

### Agent Service Not Connecting (Webhook Mode)

1. Verify agent service is running:
   ```bash
   ./mywant ps
   curl http://localhost:8081/health
   ```

2. Check configuration:
   ```bash
   ./mywant config get
   ```

3. Verify agent service host/port:
   ```bash
   cat ~/.mywant/config.yaml
   ```

## Best Practices

1. **Use configuration file for consistency:**
   - Configure once with `./mywant config set`
   - Avoid hardcoding ports in scripts

2. **Override only when necessary:**
   - Let `start` command use config file
   - Override with flags for testing only

3. **Document environment-specific configs:**
   - Keep notes on production settings
   - Version control a template config

4. **Test after configuration changes:**
   ```bash
   ./mywant config get
   ./mywant start -D
   ./mywant ps
   ```

5. **Use webhook mode for production:**
   - Better fault isolation
   - Easier scaling
   - Language flexibility

## See Also

- [Want CLI Usage Guide](WANT_CLI_USAGE.md)
- [Agent Execution Modes](AgentExecutionModes.md)
- [Agent Worker Mode](AgentWorkerMode.md)
