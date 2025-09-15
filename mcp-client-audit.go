package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// TokenCounter provides token estimation
type TokenCounter struct {
	CharsPerToken float64
}

func NewTokenCounter() *TokenCounter {
	return &TokenCounter{
		CharsPerToken: 4.0, // Conservative estimate
	}
}

func (tc *TokenCounter) EstimateTokens(text string) int {
	cleaned := strings.TrimSpace(text)
	charCount := utf8.RuneCountInString(cleaned)
	return int(float64(charCount) / tc.CharsPerToken)
}

// MCP Protocol structures
type MCPRequest struct {
	Jsonrpc string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type MCPResponse struct {
	Jsonrpc string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *MCPError       `json:"error,omitempty"`
}

type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type ListToolsResult struct {
	Tools []MCPTool `json:"tools"`
}

type MCPTool struct {
	Name        string                 `json:"name"`
	Description *string                `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// Audit results structures
type MCPServerAudit struct {
	Name        string           `json:"name"`
	Language    string           `json:"language"`
	Tools       []MCPToolAudit   `json:"tools"`
	TotalTokens int              `json:"total_tokens"`
	Summary     MCPServerSummary `json:"summary"`
	Bloat       []BloatIssue     `json:"bloat_issues"`
}

type MCPToolAudit struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	DescTokens   int    `json:"description_tokens"`
	SchemaTokens int    `json:"schema_tokens"`
	TotalTokens  int    `json:"total_tokens"`
	HasLongDesc  bool   `json:"has_long_description"`
}

type MCPServerSummary struct {
	ToolCount        int `json:"tool_count"`
	AvgTokensPerTool int `json:"avg_tokens_per_tool"`
	MaxTokensPerTool int `json:"max_tokens_per_tool"`
	MinTokensPerTool int `json:"min_tokens_per_tool"`
	LongDescTools    int `json:"long_description_tools"`
}

type BloatIssue struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	ToolName    string `json:"tool_name,omitempty"`
	Tokens      int    `json:"tokens"`
	Suggestion  string `json:"suggestion"`
}

// Server configurations
type ServerConfig struct {
	Name     string
	Language string
	Command  []string
	WorkDir  string
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run mcp-client-audit.go <path-to-nerv-systems>")
		fmt.Println("Example: go run mcp-client-audit.go /Users/pdfinn/github.com/NERVsystems")
		os.Exit(1)
	}

	basePath := os.Args[1]
	counter := NewTokenCounter()

	// Define server configurations
	servers := []ServerConfig{
		{
			Name:     "takmcp",
			Language: "Go",
			Command:  []string{"go", "run", "./cmd/takmcp", "--tak-host", "dummy.local", "--tak-port", "8089"},
			WorkDir:  basePath + "/takmcp",
		},
		{
			Name:     "osmmcp",
			Language: "Go",
			Command:  []string{"go", "run", "./cmd/osmmcp"},
			WorkDir:  basePath + "/osmmcp",
		},
		{
			Name:     "examcp",
			Language: "TypeScript",
			Command:  []string{"node", ".smithery/index.cjs"},
			WorkDir:  basePath + "/examcp",
		},
		{
			Name:     "aismcp",
			Language: "Python",
			Command:  []string{"./venv/bin/python", "-m", "aismcp"},
			WorkDir:  basePath + "/aismcp",
		},
	}

	var audits []MCPServerAudit

	fmt.Println("🔍 Starting MCP Client-Based Token Audit...")
	fmt.Println(strings.Repeat("=", 60))

	for _, serverConfig := range servers {
		fmt.Printf("📊 Auditing %s (%s)...\n", serverConfig.Name, serverConfig.Language)

		audit, err := auditMCPServer(serverConfig, counter)
		if err != nil {
			fmt.Printf("❌ Error auditing %s: %v\n", serverConfig.Name, err)
			continue
		}

		fmt.Printf("✅ %s: %d tools, %d tokens\n", serverConfig.Name, audit.Summary.ToolCount, audit.TotalTokens)
		audits = append(audits, audit)
	}

	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("📝 Generating reports...\n")

	// Generate reports
	generateReports(audits, counter)
}

func auditMCPServer(config ServerConfig, counter *TokenCounter) (MCPServerAudit, error) {
	audit := MCPServerAudit{
		Name:     config.Name,
		Language: config.Language,
		Tools:    []MCPToolAudit{},
		Bloat:    []BloatIssue{},
	}

	// Check if server directory exists
	if _, err := os.Stat(config.WorkDir); os.IsNotExist(err) {
		return audit, fmt.Errorf("server directory does not exist: %s", config.WorkDir)
	}

	// Start MCP server process with timeout
	toolsChan := make(chan []MCPTool, 1)
	errChan := make(chan error, 1)

	go func() {
		tools, err := getMCPTools(config)
		if err != nil {
			errChan <- err
			return
		}
		toolsChan <- tools
	}()

	// Wait for results or timeout
	timeout := 30 * time.Second
	if config.Language == "Python" {
		timeout = 45 * time.Second // Python servers may need more time
	}

	var tools []MCPTool
	select {
	case tools = <-toolsChan:
		// Success
	case err := <-errChan:
		return audit, err
	case <-time.After(timeout):
		return audit, fmt.Errorf("timeout after %v waiting for server response", timeout)
	}

	// Analyze each tool
	for _, tool := range tools {
		toolAudit := analyzeTool(tool, counter)
		audit.Tools = append(audit.Tools, toolAudit)
		audit.TotalTokens += toolAudit.TotalTokens
	}

	// Calculate summary and detect bloat
	calculateSummary(&audit)
	detectBloat(&audit)

	return audit, nil
}

func getMCPTools(config ServerConfig) ([]MCPTool, error) {
	// Start the MCP server process
	cmd := exec.Command(config.Command[0], config.Command[1:]...)
	cmd.Dir = config.WorkDir

	// Set environment for Python servers
	if config.Language == "Python" {
		cmd.Env = append(os.Environ(), "PYTHONPATH=src")
	}

	// Set up stdio pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start server: %w", err)
	}

	defer func() {
		stdin.Close()
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// Give the server more time to start and check if it's still running
	startDelay := 3 * time.Second
	if config.Language == "Python" {
		startDelay = 5 * time.Second // Python servers need more time
	}
	time.Sleep(startDelay)

	// Check if process is still alive
	select {
	case <-time.After(100 * time.Millisecond):
		// Process is still running
	default:
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			return nil, fmt.Errorf("server process exited early")
		}
	}

	// Send initialize request
	initReq := MCPRequest{
		Jsonrpc: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "mcp-audit-client",
				"version": "1.0.0",
			},
		},
	}

	if err := sendMCPRequest(stdin, initReq); err != nil {
		return nil, fmt.Errorf("failed to send initialize: %w", err)
	}

	// Read initialize response (but don't need to parse it for this audit)
	if _, err := readMCPResponse(stdout); err != nil {
		// Try to read stderr for error info
		errBuf := make([]byte, 1024)
		n, _ := stderr.Read(errBuf)
		if n > 0 {
			return nil, fmt.Errorf("initialize failed, stderr: %s", string(errBuf[:n]))
		}
		return nil, fmt.Errorf("failed to read initialize response: %w", err)
	}

	// Skip initialized notification as it may not be supported by all servers

	// Send tools/list request
	toolsReq := MCPRequest{
		Jsonrpc: "2.0",
		ID:      2,
		Method:  "tools/list",
	}

	if err := sendMCPRequest(stdin, toolsReq); err != nil {
		return nil, fmt.Errorf("failed to send tools/list: %w", err)
	}

	// Read tools/list response
	response, err := readMCPResponse(stdout)
	if err != nil {
		return nil, fmt.Errorf("failed to read tools/list response: %w", err)
	}

	if response.Error != nil {
		return nil, fmt.Errorf("tools/list error: %s", response.Error.Message)
	}

	// Parse tools list
	var result ListToolsResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse tools list: %w", err)
	}

	return result.Tools, nil
}

func sendMCPRequest(writer interface{ Write([]byte) (int, error) }, req MCPRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	_, err = writer.Write(append(data, '\n'))
	return err
}

func readMCPResponse(reader interface{ Read([]byte) (int, error) }) (*MCPResponse, error) {
	// Try reading with a timeout approach
	buf := make([]byte, 64*1024) // 64KB buffer

	// Read with multiple attempts for slow servers
	var totalData []byte
	attempts := 0
	maxAttempts := 3

	for attempts < maxAttempts {
		n, err := reader.Read(buf)
		if err != nil && n == 0 {
			if attempts == 0 {
				return nil, fmt.Errorf("failed to read response: %w", err)
			}
			time.Sleep(500 * time.Millisecond)
			attempts++
			continue
		}

		if n > 0 {
			totalData = append(totalData, buf[:n]...)
			break
		}

		attempts++
		time.Sleep(200 * time.Millisecond)
	}

	if len(totalData) == 0 {
		return nil, fmt.Errorf("no data received after %d attempts", maxAttempts)
	}

	// Find the end of the JSON response
	lines := strings.Split(string(totalData), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var response MCPResponse
		if err := json.Unmarshal([]byte(line), &response); err != nil {
			continue // Skip non-JSON lines
		}

		return &response, nil
	}

	return nil, fmt.Errorf("no valid JSON response found in data: %s", string(totalData[:min(200, len(totalData))]))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func analyzeTool(tool MCPTool, counter *TokenCounter) MCPToolAudit {
	description := ""
	if tool.Description != nil {
		description = *tool.Description
	}

	// Convert schema to JSON string to estimate tokens
	schemaJson, _ := json.Marshal(tool.InputSchema)
	schemaStr := string(schemaJson)

	toolAudit := MCPToolAudit{
		Name:         tool.Name,
		Description:  description,
		DescTokens:   counter.EstimateTokens(description),
		SchemaTokens: counter.EstimateTokens(schemaStr),
	}

	toolAudit.TotalTokens = toolAudit.DescTokens + toolAudit.SchemaTokens
	toolAudit.HasLongDesc = toolAudit.DescTokens > 50

	return toolAudit
}

func calculateSummary(audit *MCPServerAudit) {
	maxTokens := 0
	minTokens := 999999
	longDescCount := 0

	for _, tool := range audit.Tools {
		if tool.TotalTokens > maxTokens {
			maxTokens = tool.TotalTokens
		}
		if tool.TotalTokens < minTokens && tool.TotalTokens > 0 {
			minTokens = tool.TotalTokens
		}
		if tool.HasLongDesc {
			longDescCount++
		}
	}

	toolCount := len(audit.Tools)
	if toolCount == 0 {
		minTokens = 0
	}

	audit.Summary = MCPServerSummary{
		ToolCount:        toolCount,
		LongDescTools:    longDescCount,
		MaxTokensPerTool: maxTokens,
		MinTokensPerTool: minTokens,
	}

	if toolCount > 0 {
		audit.Summary.AvgTokensPerTool = audit.TotalTokens / toolCount
	}
}

func detectBloat(audit *MCPServerAudit) {
	for _, tool := range audit.Tools {
		// Long descriptions
		if tool.DescTokens > 100 {
			audit.Bloat = append(audit.Bloat, BloatIssue{
				Type:        "verbose_description",
				Description: fmt.Sprintf("Tool '%s' has very long description (%d tokens)", tool.Name, tool.DescTokens),
				ToolName:    tool.Name,
				Tokens:      tool.DescTokens,
				Suggestion:  "Consider breaking into sections or using more concise language",
			})
		}

		// Large schemas
		if tool.SchemaTokens > 200 {
			audit.Bloat = append(audit.Bloat, BloatIssue{
				Type:        "large_schema",
				Description: fmt.Sprintf("Tool '%s' has large input schema (%d tokens)", tool.Name, tool.SchemaTokens),
				ToolName:    tool.Name,
				Tokens:      tool.SchemaTokens,
				Suggestion:  "Consider simplifying parameter structure or descriptions",
			})
		}
	}
}

func generateReports(audits []MCPServerAudit, counter *TokenCounter) {
	// Generate JSON report
	jsonData, err := json.MarshalIndent(audits, "", "  ")
	if err != nil {
		fmt.Printf("❌ Error generating JSON: %v\n", err)
		return
	}

	err = os.WriteFile("mcp-client-audit-report.json", jsonData, 0644)
	if err != nil {
		fmt.Printf("❌ Error writing JSON report: %v\n", err)
	} else {
		fmt.Println("✅ JSON report saved to mcp-client-audit-report.json")
	}

	// Generate human-readable report
	generateHumanReport(audits)
}

func generateHumanReport(audits []MCPServerAudit) {
	var report strings.Builder

	report.WriteString("# MCP Client-Based Token Audit Report\n\n")
	report.WriteString(fmt.Sprintf("Generated on: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))

	// Overall summary
	totalTokens := 0
	totalTools := 0
	for _, audit := range audits {
		totalTokens += audit.TotalTokens
		totalTools += audit.Summary.ToolCount
	}

	report.WriteString("## 🎯 Executive Summary\n\n")
	report.WriteString("**This audit connects directly to MCP servers to get actual tool definitions sent to AI models.**\n\n")
	report.WriteString(fmt.Sprintf("- **Total Servers Audited**: %d\n", len(audits)))
	report.WriteString(fmt.Sprintf("- **Total Tools**: %d\n", totalTools))
	report.WriteString(fmt.Sprintf("- **Total Token Usage**: ~%d tokens\n", totalTokens))
	if totalTools > 0 {
		report.WriteString(fmt.Sprintf("- **Average Tokens per Tool**: ~%d tokens\n\n", totalTokens/totalTools))
	}

	// Sort servers by token usage (descending)
	sort.Slice(audits, func(i, j int) bool {
		return audits[i].TotalTokens > audits[j].TotalTokens
	})

	report.WriteString("## 📊 Server Breakdown\n\n")

	for i, audit := range audits {
		report.WriteString(fmt.Sprintf("### %d. %s (%s)\n", i+1, audit.Name, audit.Language))
		report.WriteString(fmt.Sprintf("- **Total Tokens**: %d\n", audit.TotalTokens))
		report.WriteString(fmt.Sprintf("- **Tools**: %d\n", audit.Summary.ToolCount))
		if audit.Summary.ToolCount > 0 {
			report.WriteString(fmt.Sprintf("- **Avg Tokens/Tool**: %d\n", audit.Summary.AvgTokensPerTool))
			report.WriteString(fmt.Sprintf("- **Token Range**: %d - %d\n", audit.Summary.MinTokensPerTool, audit.Summary.MaxTokensPerTool))
		}
		report.WriteString(fmt.Sprintf("- **Long Descriptions**: %d tools (>50 tokens)\n", audit.Summary.LongDescTools))
		report.WriteString("\n")
	}

	// Bloat analysis
	report.WriteString("## 🚨 Bloat Issues\n\n")

	bloatByType := make(map[string][]BloatIssue)
	totalBloatTokens := 0

	for _, audit := range audits {
		for _, issue := range audit.Bloat {
			bloatByType[issue.Type] = append(bloatByType[issue.Type], issue)
			totalBloatTokens += issue.Tokens
		}
	}

	if totalBloatTokens == 0 {
		report.WriteString("✅ No significant bloat issues detected!\n\n")
	} else {
		report.WriteString(fmt.Sprintf("**Total Potential Optimization**: ~%d tokens (%.1f%% of total)\n\n",
			totalBloatTokens, float64(totalBloatTokens)/float64(totalTokens)*100))

		for bloatType, issues := range bloatByType {
			report.WriteString(fmt.Sprintf("### %s (%d issues)\n", strings.Title(strings.ReplaceAll(bloatType, "_", " ")), len(issues)))

			// Show top 5 worst offenders
			sort.Slice(issues, func(i, j int) bool {
				return issues[i].Tokens > issues[j].Tokens
			})

			limit := len(issues)
			if limit > 5 {
				limit = 5
			}

			for i := 0; i < limit; i++ {
				issue := issues[i]
				report.WriteString(fmt.Sprintf("- **%s** - %s (%d tokens)\n", issue.ToolName, issue.Description, issue.Tokens))
				report.WriteString(fmt.Sprintf("  *Suggestion*: %s\n", issue.Suggestion))
			}

			if len(issues) > 5 {
				report.WriteString(fmt.Sprintf("  ... and %d more\n", len(issues)-5))
			}
			report.WriteString("\n")
		}
	}

	// Top tools by token usage
	report.WriteString("## 🔍 Heaviest Tools\n\n")

	var allTools []struct {
		MCPToolAudit
		Server string
	}

	for _, audit := range audits {
		for _, tool := range audit.Tools {
			allTools = append(allTools, struct {
				MCPToolAudit
				Server string
			}{tool, audit.Name})
		}
	}

	sort.Slice(allTools, func(i, j int) bool {
		return allTools[i].TotalTokens > allTools[j].TotalTokens
	})

	limit := len(allTools)
	if limit > 10 {
		limit = 10
	}

	for i := 0; i < limit; i++ {
		tool := allTools[i]
		report.WriteString(fmt.Sprintf("%d. **%s** (%s): %d tokens\n",
			i+1, tool.Name, tool.Server, tool.TotalTokens))
		report.WriteString(fmt.Sprintf("   - Description: %d tokens\n", tool.DescTokens))
		report.WriteString(fmt.Sprintf("   - Schema: %d tokens\n", tool.SchemaTokens))
		report.WriteString("\n")
	}

	// Recommendations
	report.WriteString("## 💡 Optimization Recommendations\n\n")

	if totalBloatTokens > 500 {
		report.WriteString("### 🔥 High Priority\n")
		report.WriteString("- **Immediate action recommended** - significant token usage detected\n")
		report.WriteString("- Focus on tools with >100 token descriptions\n")
		report.WriteString("- Consider simplifying complex parameter schemas\n\n")
	}

	report.WriteString("### 📋 General Recommendations\n")
	report.WriteString("- **Tool Descriptions**: Keep under 50 tokens when possible\n")
	report.WriteString("- **Parameter Schemas**: Simplify complex nested structures\n")
	report.WriteString("- **Parameter Descriptions**: Use concise, clear language\n")
	report.WriteString("- **Tool Count**: Evaluate if all tools are necessary\n\n")

	report.WriteString("### 🔬 Methodology Notes\n")
	report.WriteString("- **Accurate Measurement**: Connects directly to MCP servers as a client\n")
	report.WriteString("- **Real-World Data**: Measures actual tool definitions sent to AI models\n")
	report.WriteString("- **Language Agnostic**: Works with Go, Python, TypeScript, and any MCP server\n")
	report.WriteString("- **Token estimates**: ~4 chars/token (conservative for technical content)\n")

	// Write report
	err := os.WriteFile("mcp-client-audit-report.md", []byte(report.String()), 0644)
	if err != nil {
		fmt.Printf("❌ Error writing markdown report: %v\n", err)
	} else {
		fmt.Println("✅ Human-readable report saved to mcp-client-audit-report.md")
	}

	// Print summary to console
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("🎯 MCP CLIENT AUDIT SUMMARY")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("📊 Total tokens across %d servers: ~%d tokens\n", len(audits), totalTokens)
	if totalBloatTokens > 0 {
		fmt.Printf("⚡ Potential optimization: ~%d tokens (%.1f%%)\n", totalBloatTokens, float64(totalBloatTokens)/float64(totalTokens)*100)
	}
	fmt.Printf("🔧 Check mcp-client-audit-report.md for detailed recommendations\n")
	fmt.Println(strings.Repeat("=", 60))
}