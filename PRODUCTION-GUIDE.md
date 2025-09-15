# MCP Token Audit Tool - Production Guide

## Quick Start

```bash
# Clone or navigate to the audit tool
cd mcp-audit

# Run the audit
go run mcp-client-audit.go /path/to/your/mcp/servers

# View results
cat mcp-client-audit-report.md
```

## Installation

```bash
# Prerequisites
go version  # Requires Go 1.21+

# No additional dependencies - uses only Go standard library
```

## Usage

### Basic Usage
```bash
go run mcp-client-audit.go /Users/pdfinn/github.com/NERVsystems
```

### Build for Production
```bash
# Build standalone executable
go build -o mcp-audit mcp-client-audit.go

# Run built version
./mcp-audit /path/to/servers
```

### CI/CD Integration
```bash
#!/bin/bash
# Example CI script
cd /path/to/mcp-audit
go run mcp-client-audit.go /path/to/servers > audit-results.log 2>&1

# Check for excessive bloat
TOTAL_TOKENS=$(grep "Total tokens" audit-results.log | grep -o '[0-9]*')
if [ "$TOTAL_TOKENS" -gt 15000 ]; then
    echo "❌ Token usage too high: $TOTAL_TOKENS tokens"
    exit 1
fi

echo "✅ Token usage acceptable: $TOTAL_TOKENS tokens"
```

## Server Configuration

The tool automatically detects and configures different MCP server types:

### Supported Server Types

#### Go Servers (TAKMCP, OSMMCP)
```go
{
    Name: "takmcp",
    Language: "Go",
    Command: []string{"go", "run", "./cmd/takmcp", "--tak-host", "dummy.local"},
    WorkDir: "/path/to/takmcp",
}
```

#### TypeScript Servers (EXAMCP)
```go
{
    Name: "examcp",
    Language: "TypeScript",
    Command: []string{"node", ".smithery/index.cjs"},
    WorkDir: "/path/to/examcp",
}
```

#### Python Servers (AISMCP)
```go
{
    Name: "aismcp",
    Language: "Python",
    Command: []string{"bash", "-c", "PYTHONPATH=src ./venv/bin/python -m aismcp"},
    WorkDir: "/path/to/aismcp",
}
```

### Adding New Servers

Edit `mcp-client-audit.go` and add to the `servers` slice:

```go
servers := []ServerConfig{
    // ... existing servers ...
    {
        Name:     "my-new-server",
        Language: "Go",  // or "Python", "TypeScript"
        Command:  []string{"go", "run", "./cmd/my-server"},
        WorkDir:  basePath + "/my-new-server",
    },
}
```

## Output Files

The tool generates three output files:

### 1. `mcp-client-audit-report.json`
Machine-readable detailed results for programmatic processing:
```json
{
  "name": "takmcp",
  "language": "Go",
  "tools": [
    {
      "name": "execute_sql",
      "description": "Execute SQL queries...",
      "description_tokens": 1111,
      "schema_tokens": 115,
      "total_tokens": 1226
    }
  ],
  "bloat_issues": [...]
}
```

### 2. `mcp-client-audit-report.md`
Human-readable report with recommendations:
- Executive summary
- Server breakdown
- Bloat analysis
- Optimization recommendations

### 3. Console Output
Real-time progress and summary:
```
🔍 Starting MCP Client-Based Token Audit...
📊 Auditing takmcp (Go)...
✅ takmcp: 37 tools, 9396 tokens
📊 Total tokens across 3 servers: ~13708 tokens
⚡ Potential optimization: ~7080 tokens (51.6%)
```

## Thresholds and Monitoring

### Recommended Token Budgets

```bash
# Per-server budgets
TAKMCP_MAX_TOKENS=5000    # Currently 9,396 - needs reduction
OSMMCP_MAX_TOKENS=3000    # Currently 2,696 - acceptable
EXAMCP_MAX_TOKENS=2000    # Currently 1,616 - acceptable

# Total system budget
TOTAL_MAX_TOKENS=10000    # Currently 13,708 - needs reduction
```

### Alerting Thresholds

#### Critical (Immediate Action)
- Total tokens >15,000
- Any tool >800 tokens
- Server growth >50% from baseline

#### Warning (Review Needed)
- Total tokens >12,000
- Any tool >400 tokens
- Server growth >25% from baseline

#### Information (Tracking)
- Total tokens >10,000
- Any tool >200 tokens
- New tools added

## Troubleshooting

### Common Issues

#### "broken pipe" errors
**Cause**: Server not starting properly or missing dependencies
**Solution**: Check server can run independently:
```bash
cd /path/to/server
go run ./cmd/server  # Test server startup
```

#### "server process exited early"
**Cause**: Missing configuration or environment variables
**Solution**: Add required flags/env vars to Command configuration

#### "no valid JSON response found"
**Cause**: Server not implementing MCP protocol correctly
**Solution**: Verify server supports:
- `initialize` method
- `tools/list` method
- JSON-RPC 2.0 format

#### Module import errors (Python)
**Solution**: Set correct PYTHONPATH and virtual environment:
```bash
PYTHONPATH=src ./venv/bin/python -m server
```

### Debug Mode

Add debugging to the audit tool by modifying the logging:

```go
// Add before sending MCP requests
fmt.Printf("🔍 Sending request: %s\n", string(data))

// Add after receiving responses
fmt.Printf("📥 Received: %s\n", string(totalData))
```

## Performance Considerations

### Tool Startup Time
- Allow 3+ seconds for server startup
- Increase timeout for slow servers
- Use parallel auditing for multiple servers

### Memory Usage
- Each server audit uses ~50MB peak memory
- JSON parsing scales with schema complexity
- No persistent memory usage

### Scalability
- Linear scaling with number of tools
- Network independent (stdio communication)
- Can audit 10+ servers in ~2 minutes

## Integration Examples

### GitHub Actions
```yaml
name: MCP Token Audit
on: [push, pull_request]
jobs:
  audit:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - uses: actions/setup-go@v2
      - name: Run MCP Audit
        run: |
          cd mcp-audit
          go run mcp-client-audit.go .
          # Upload reports as artifacts
```

### Pre-commit Hook
```bash
#!/bin/sh
# .git/hooks/pre-commit
cd mcp-audit
TOKENS=$(go run mcp-client-audit.go . | grep "Total tokens" | grep -o '[0-9]*')
if [ "$TOKENS" -gt 15000 ]; then
    echo "❌ MCP token usage too high: $TOKENS tokens"
    echo "🔧 Run optimization before committing"
    exit 1
fi
```

### Monitoring Dashboard
```bash
# Cron job for daily monitoring
0 8 * * * /path/to/audit-and-alert.sh

# audit-and-alert.sh
#!/bin/bash
cd /path/to/mcp-audit
go run mcp-client-audit.go /path/to/servers > daily-audit.log 2>&1

# Send to monitoring system
curl -X POST monitoring.company.com/metrics \
  -d "mcp_total_tokens=$(grep 'Total tokens' daily-audit.log | grep -o '[0-9]*')"
```

## Token Optimization Strategies

### High-Impact Optimizations
1. **Verbose descriptions** (>100 tokens): Break into sections, use bullets
2. **Complex schemas** (>200 tokens): Simplify parameter structure
3. **Redundant tools**: Merge similar functionality

### Medium-Impact Optimizations
1. **Parameter descriptions**: Use 5-10 words maximum
2. **Example removal**: Move examples to documentation
3. **Schema nesting**: Flatten deeply nested parameters

### Low-Impact Optimizations
1. **Tool names**: Use shorter, clear names
2. **Required fields**: Mark optional parameters properly
3. **Default values**: Reduce schema verbosity

## Maintenance

### Regular Tasks
- **Monthly audits**: Track token usage trends
- **Quarterly reviews**: Evaluate tool necessity
- **Release audits**: Check before major deployments

### Tool Updates
- Update server configurations as architectures change
- Add new server types to the configuration
- Adjust token thresholds based on system growth

### Reporting
- Archive monthly reports for trend analysis
- Share optimization wins with development teams
- Track ROI of token reduction efforts

The tool is production-ready and provides the foundation for ongoing MCP token optimization across your system.