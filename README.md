# MCP Token Audit Tool

A comprehensive tool to audit Model Context Protocol (MCP) servers for token usage and identify potential bloat in tool definitions, descriptions, and prompts.

## Overview

This tool analyzes MCP servers implemented in different languages (Go, TypeScript, Python) and generates detailed reports about token usage to help identify opportunities for optimization.

## Features

- **Multi-language Support**: Analyzes Go, TypeScript/Node.js, and Python MCP servers
- **Token Estimation**: Estimates token usage using ~4 characters per token (conservative for technical content)
- **Bloat Detection**: Identifies verbose descriptions, too many parameters, and large prompts
- **Detailed Reports**: Generates both JSON and human-readable markdown reports
- **Optimization Recommendations**: Provides specific suggestions for reducing token usage

## What Gets Audited

### Tool Definitions
- Tool names and descriptions
- Parameter names, types, and descriptions
- JSON schema definitions
- Input/output schemas

### Prompt Templates
- System prompts and instructions
- Template strings and formats
- Example content and samples

### Potential Bloat Issues
- Verbose tool descriptions (>100 tokens)
- Too many parameters per tool (>10)
- Verbose parameter descriptions (>20 tokens)
- Large prompt templates (>200 tokens)

## Usage

```bash
# Build the tool
go build -o mcp-audit

# Run audit on NERV systems directory
./mcp-audit /path/to/NERVsystems

# View reports
cat mcp-audit-report.md
cat mcp-audit-report.json
```

## Supported MCP Servers

- **takmcp** (Go) - TAK network integration
- **osmmcp** (Go) - OpenStreetMap operations
- **examcp** (TypeScript) - Exa AI search capabilities
- **aismcp** (Python) - AIS maritime data processing

## Report Structure

### JSON Report
- Complete machine-readable audit data
- Tool-by-tool token breakdown
- Parameter-level analysis
- Bloat issue categorization

### Markdown Report
- Executive summary
- Server-by-server breakdown
- Top token-consuming tools
- Optimization recommendations

## Token Estimation Methodology

- Uses ~4 characters per token (conservative estimate for technical content)
- Includes all content that gets sent to AI models as context:
  - Tool names and descriptions
  - Parameter schemas and descriptions
  - Prompt templates and system instructions
- Does not include dynamic response content or runtime context

## Optimization Recommendations

### High Impact
- Keep tool descriptions under 50 tokens
- Limit parameter descriptions to under 10 tokens
- Consider breaking complex tools into simpler ones
- Use dynamic prompt generation vs large static templates

### Medium Impact
- Combine related parameters into objects
- Use enums instead of verbose descriptions where possible
- Remove redundant or rarely-used tools
- Optimize JSON schema definitions

## Architecture

The tool uses different parsing strategies for each language:

- **Go**: AST parsing with regex fallback
- **TypeScript**: Regex-based extraction of MCP SDK patterns
- **Python**: Pattern matching for decorators and docstrings

## Future Enhancements

- Integration with actual MCP protocol introspection
- Real-time token usage monitoring
- Automated optimization suggestions
- Support for additional MCP server languages
- Performance impact analysis