# Goose Integration Guide

## Overview

MyWant's Gmail Dynamic Want uses **Goose** (an AI-powered coding assistant) to automatically generate code for MCP (Model Context Protocol) tool adapters. This document describes how to install and configure Goose for use with MyWant.

## What is Goose?

Goose is an AI coding assistant that can:
- Execute MCP tools via natural language prompts
- Generate code based on tool discovery
- Interact with various LLM providers (Claude, Gemini, etc.)

MyWant uses Goose to:
1. **Discover MCP tools** - Analyze user requests and find appropriate Gmail MCP tools
2. **Generate adapter code** - Create Go code that transforms MyWant parameters to MCP requests
3. **Parse responses** - Generate code to convert MCP responses to MyWant state

## System Architecture

```
User Request
     ↓
Gmail Dynamic Want
     ↓
CodeGeneration Agent (with Templates)
     ↓
Goose (via CLI)
     ↓
Gmail MCP Server
     ↓
Generated Go Code → WASM → Validation → Stable
```

## Installation

### Prerequisites

- Go 1.21 or later
- Node.js 18 or later (for MCP servers)
- Gmail MCP server configured in Goose

### Step 1: Install Goose

#### Option A: Using Homebrew (macOS/Linux)

```bash
brew install block-goose-cli
```

#### Option B: From Source

```bash
git clone https://github.com/block/goose.git
cd goose
pip install -e .
```

**Note**: The Homebrew method (Option A) is recommended for macOS users.

### Step 2: Verify Installation

```bash
goose --version
```

Expected output:
```
goose version x.x.x
```

### Step 3: Configure Goose

Create or edit Goose configuration file:

```bash
mkdir -p ~/.config/goose
cat > ~/.config/goose/config.yaml << 'EOF'
# Goose Configuration for MyWant

# Default LLM provider
provider: gemini-cli  # or claude-code, openai, etc.

# MCP Extensions
extensions:
  - name: gmail
    type: mcp
    command: npx
    args:
      - -y
      - "@gongrzhe/server-gmail-autoauth-mcp"
    env: {}
EOF
```

### Step 4: Configure LLM Provider

#### For Gemini (Recommended)

```bash
# Install Gemini CLI
npm install -g @google/generative-ai-cli

# Set API key
export GOOGLE_API_KEY="your-api-key-here"

# Add to your shell profile (.bashrc, .zshrc, etc.)
echo 'export GOOGLE_API_KEY="your-api-key-here"' >> ~/.zshrc
```

#### For Claude (Alternative)

```bash
# Set API key
export ANTHROPIC_API_KEY="your-api-key-here"

# Add to shell profile
echo 'export ANTHROPIC_API_KEY="your-api-key-here"' >> ~/.zshrc
```

### Step 5: Test Goose with Gmail MCP

```bash
# Test basic Goose functionality
echo "Search for emails from boss about project" | goose run -i - --provider gemini-cli
```

Expected behavior:
- Goose connects to Gmail MCP server
- Executes search_emails tool
- Returns JSON with email results

### Step 6: Set MyWant Environment Variable (Optional)

```bash
# Set preferred Goose provider for MyWant
export MYWANT_GOOSE_PROVIDER="gemini-cli"

# Add to shell profile
echo 'export MYWANT_GOOSE_PROVIDER="gemini-cli"' >> ~/.zshrc
```

## Usage with MyWant

### Creating a Gmail Dynamic Want

```yaml
wants:
  - metadata:
      name: gmail_search_example
      type: gmail_dynamic
    spec:
      params:
        prompt: "Search for emails from my boss about the Q1 project"
```

### Execution Flow

1. **PhaseCodeGeneration** (Goose呼び出し 1回)
   - Goose analyzes the prompt
   - Discovers appropriate Gmail MCP tool (e.g., `search_emails`)
   - Generates Go code with TransformRequest/ParseResponse functions
   - Uses pre-existing templates as reference for better quality

2. **PhaseCompiling**
   - TinyGo compiles generated code to WASM
   - If compilation fails, returns to CodeGeneration with error feedback

3. **PhaseValidation**
   - Executes WASM with direct MCP call
   - Validates that code works correctly
   - If validation fails, returns to CodeGeneration

4. **PhaseStable**
   - Uses validated WASM for all future executions
   - No more Goose calls needed

### Monitoring Execution

```bash
# Check want status
./bin/mywant wants get <want-id>

# View logs
tail -f logs/server.log | grep "GMAIL-DYNAMIC"

# Check for Goose-related errors
grep "GOOSE" logs/server.log
```

## Troubleshooting

### Error: "goose: executable file not found in $PATH"

**Cause**: Goose is not installed or not in PATH

**Solution**:
```bash
# Verify Goose installation
which goose

# If not found, install Goose (see Installation section)

# Add Goose to PATH if needed
export PATH="$PATH:/path/to/goose/bin"
```

### Error: "Goose execution failed: provider not configured"

**Cause**: LLM provider (Gemini/Claude) API key not set

**Solution**:
```bash
# For Gemini
export GOOGLE_API_KEY="your-api-key"

# For Claude
export ANTHROPIC_API_KEY="your-api-key"
```

### Error: "MCP server connection failed"

**Cause**: Gmail MCP server not properly configured in Goose

**Solution**:
```bash
# Verify Goose config
cat ~/.config/goose/config.yaml

# Test MCP server directly
npx -y @gongrzhe/server-gmail-autoauth-mcp

# Check Goose can access MCP server
echo "List my emails" | goose run -i - --provider gemini-cli
```

### Error: "Request failed" or "Ran into this error"

**Cause**: Primary LLM provider failed (rate limit, API error, etc.)

**Solution**: MyWant automatically falls back to alternate provider
- Primary: gemini-cli → Fallback: claude-code
- Primary: claude-code → Fallback: gemini-cli

To manually set fallback priority:
```bash
export MYWANT_GOOSE_PROVIDER="gemini-cli"  # Primary
# Fallback is automatic
```

### Compilation Errors

**Cause**: Generated code has syntax errors or uses unavailable packages

**Solution**: MyWant automatically retries CodeGeneration with error feedback
- Max retries: 3 per phase
- Error feedback sent to Goose for regeneration
- Templates guide Goose to generate correct code

### Performance Issues

**Symptom**: Goose calls taking 30+ seconds

**Causes & Solutions**:
1. **Slow LLM provider**
   - Try different provider: `--provider claude-code`
   - Check provider API status

2. **Large MCP responses**
   - Normal for first-time discovery
   - Subsequent calls use cached WASM (fast)

3. **Network latency**
   - Check internet connection
   - Use local LLM if available

## Configuration Options

### Goose Configuration File (~/.config/goose/config.yaml)

```yaml
# Full configuration example

# Default provider (gemini-cli, claude-code, openai, etc.)
provider: gemini-cli

# MCP extensions
extensions:
  - name: gmail
    type: mcp
    command: npx
    args:
      - -y
      - "@gongrzhe/server-gmail-autoauth-mcp"
    env:
      # Optional: Set specific environment variables for this MCP server
      GMAIL_SCOPES: "https://www.googleapis.com/auth/gmail.readonly"

  # Add more MCP servers as needed
  - name: google-search
    type: mcp
    command: npx
    args:
      - -y
      - "@modelcontextprotocol/server-google-search"

# Logging
log_level: info  # debug, info, warn, error

# Timeouts
timeout: 60  # seconds
```

### MyWant Environment Variables

```bash
# Preferred Goose provider
export MYWANT_GOOSE_PROVIDER="gemini-cli"

# Goose binary path (if not in $PATH)
export GOOSE_BIN="/custom/path/to/goose"

# Enable verbose logging
export MYWANT_DEBUG=true
```

## Performance Optimization

### Template-Based Code Generation

MyWant includes pre-defined templates for common Gmail operations:
- `templates/gmail_templates.go`
  - GmailSearchTemplate
  - GmailSendTemplate
  - GmailReadTemplate

**Benefits**:
- Goose learns from existing patterns
- Higher quality generated code
- Fewer compilation errors
- Faster convergence to stable WASM

### Caching Strategy

1. **WASM Caching**: Once validated, WASM is reused (no more Goose calls)
2. **Template Matching**: Similar requests use similar templates
3. **Error Feedback Loop**: Compilation errors guide regeneration

### Reducing Goose Call Count

**Before optimization**: 4 Goose calls (worst case)
- Discovery: 1 call
- Coding: 1 call
- Retry (compilation error): 1 call
- Retry (validation error): 1 call

**After optimization**: 1-2 Goose calls
- CodeGeneration (unified): 1 call
- Retry (if needed): 1 call

**Reduction**: 50-75% fewer Goose calls

## Security Considerations

### API Key Management

```bash
# NEVER commit API keys to git
echo "GOOGLE_API_KEY=*" >> .gitignore
echo "ANTHROPIC_API_KEY=*" >> .gitignore

# Use environment variables
export GOOGLE_API_KEY="$(cat ~/.secrets/google-api-key)"

# Or use secret management tools
# - 1Password CLI
# - AWS Secrets Manager
# - HashiCorp Vault
```

### MCP Server Security

- Gmail MCP uses OAuth2 for authentication
- Tokens stored in `~/.config/goose/` (secure location)
- Never share token files
- Regularly rotate API keys

### Generated Code Review

- Review generated WASM code before deploying to production
- Check for:
  - Unnecessary data access
  - Overly permissive scopes
  - Hardcoded credentials

## Advanced Usage

### Custom MCP Servers

Add custom MCP servers to Goose config:

```yaml
extensions:
  - name: my-custom-mcp
    type: mcp
    command: /path/to/custom-mcp-server
    args: []
    env:
      CUSTOM_API_KEY: "${CUSTOM_API_KEY}"
```

### Multiple Provider Fallback

MyWant automatically tries alternate providers on failure:

```go
// Primary attempt
result, err := goose.ExecuteViaGoose(ctx, "mcp_unified_codegen", params)

// Automatic fallback on error
if failed {
    fallbackProvider := "gemini-cli" == "claude-code" ? "claude-code" : "gemini-cli"
    result, err = goose.ExecuteViaGoose(ctx, "mcp_unified_codegen", paramsWithFallback)
}
```

### Debug Mode

```bash
# Enable verbose logging
export MYWANT_DEBUG=true

# Check Goose execution details
grep "GOOSE-MANAGER" logs/server.log

# View generated code before compilation
grep "WASM Compiler Debug" logs/server.log
```

## FAQ

### Q: Do I need Goose for every Gmail Dynamic Want execution?

**A**: No. Goose is only called during the **CodeGeneration phase** (once per want type/pattern). Once WASM is validated, it's reused indefinitely.

### Q: Can I use a local LLM instead of cloud providers?

**A**: Yes, if Goose supports it. Configure Goose with a local LLM provider (e.g., Ollama, LocalAI).

### Q: What if Goose generates incorrect code?

**A**: MyWant has a 3-retry mechanism:
1. Compilation catches syntax errors → regenerate with feedback
2. Validation catches logic errors → regenerate with feedback
3. After 3 failures → Want marked as Failed (manual intervention needed)

### Q: Can I bypass Goose and provide pre-written code?

**A**: Yes, for production use cases, consider:
1. Use template-based approach (Option 2 from optimization doc)
2. Pre-compile WASM and reference directly
3. Use standard `gmail` want type instead of `gmail_dynamic`

### Q: How much does Goose cost (LLM API calls)?

**A**: Depends on provider:
- **Gemini**: ~$0.01-0.05 per code generation
- **Claude**: ~$0.05-0.10 per code generation
- **Cost optimization**: Templates reduce token count, caching eliminates repeat calls

## Related Documentation

- [Want Developer Guide](WantDeveloperGuide.md) - Custom Want development
- [Agent System](agent-system.md) - Agent architecture
- [MCP Integration](https://modelcontextprotocol.io/) - MCP specification
- [Goose Documentation](https://github.com/block/goose) - Official Goose docs

## Support

For issues related to:
- **Goose installation**: See [Goose GitHub Issues](https://github.com/block/goose/issues)
- **MCP servers**: See specific MCP server documentation
- **MyWant integration**: Open issue in MyWant repository

## Changelog

### 2026-02-01: Initial Release
- Unified CodeGeneration (Discovery + Coding)
- Template-based code generation
- Automatic provider fallback
- 50% reduction in Goose call count
