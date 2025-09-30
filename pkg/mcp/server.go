// Package mcp provides the JSON-RPC server for module coordination protocol.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/cloudnationhq/az-cn-wam-mcp/internal/database"
	"github.com/cloudnationhq/az-cn-wam-mcp/internal/indexer"
)

// Message represents a JSON-RPC 2.0 message.
type Message struct {
	JSONRPC string    `json:"jsonrpc"`
	Method  string    `json:"method,omitempty"`
	Params  any       `json:"params,omitempty"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type ToolCallParams struct {
	Name      string `json:"name"`
	Arguments any    `json:"arguments"`
}

// Server wraps all dependencies required to serve MCP requests.
type Server struct {
	db     *database.DB
	syncer *indexer.Syncer
	writer io.Writer
}

// NewServer constructs a Server.
func NewServer(db *database.DB, syncer *indexer.Syncer) *Server {
	return &Server{db: db, syncer: syncer}
}

// Run processes messages from r and writes responses to w until the context is done.
func (s *Server) Run(ctx context.Context, r io.Reader, w io.Writer) error {
	s.writer = w
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		log.Printf("Received: %s", line)

		var msg Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			log.Printf("Failed to parse message: %v", err)
			s.sendError(-32700, "Parse error", nil)
			continue
		}

		s.handleMessage(msg)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}

	return nil
}

func (s *Server) handleMessage(msg Message) {
	log.Printf("Handling method: %s", msg.Method)

	switch msg.Method {
	case "initialize":
		s.handleInitialize(msg)
	case "initialized":
		// Notification - no response needed
		log.Println("Client initialized")
	case "tools/list":
		s.handleToolsList(msg)
	case "tools/call":
		s.handleToolsCall(msg)
	case "notifications/cancelled":
		// Handle cancellation
		log.Println("Request cancelled")
	default:
		s.sendError(-32601, "Method not found", msg.ID)
	}
}

func (s *Server) handleInitialize(msg Message) {
	response := Message{
		JSONRPC: "2.0",
		ID:      msg.ID,
		Result: map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]any{
				"name":    "az-cn-wam",
				"version": "1.0.0",
			},
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
		},
	}
	s.sendResponse(response)
}

func (s *Server) handleToolsList(msg Message) {
	tools := []map[string]any{
		{
			"name":        "sync_modules",
			"description": "Sync all Terraform modules from GitHub to local database",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			"name":        "sync_updates_modules",
			"description": "Incrementally sync only updated Terraform modules from GitHub (skips unchanged modules)",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			"name":        "list_modules",
			"description": "List all available Terraform modules from local database",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			"name":        "search_modules",
			"description": "Search modules by name or description in local database",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Search query",
					},
					"limit": map[string]any{
						"type":        "number",
						"description": "Maximum number of results (default: 10)",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			"name":        "get_module_info",
			"description": "Get detailed information about a specific module including all files, variables, outputs, resources",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"module_name": map[string]any{
						"type":        "string",
						"description": "Name of the module",
					},
				},
				"required": []string{"module_name"},
			},
		},
		{
			"name":        "search_code",
			"description": "Search across all Terraform code files for specific patterns or text",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Text or pattern to search for in code",
					},
					"limit": map[string]any{
						"type":        "number",
						"description": "Maximum number of results (default: 20)",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			"name":        "get_file_content",
			"description": "Get the full content of a specific file from a module (e.g., variables.tf, main.tf, outputs.tf)",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"module_name": map[string]any{
						"type":        "string",
						"description": "Name of the module (e.g., terraform-azure-aks)",
					},
					"file_path": map[string]any{
						"type":        "string",
						"description": "Path to the file within the module (e.g., variables.tf, main.tf, README.md)",
					},
				},
				"required": []string{"module_name", "file_path"},
			},
		},
		{
			"name":        "extract_variable_definition",
			"description": "Extract the complete definition of a specific variable from a module's variables.tf",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"module_name": map[string]any{
						"type":        "string",
						"description": "Name of the module (e.g., terraform-azure-aks)",
					},
					"variable_name": map[string]any{
						"type":        "string",
						"description": "Name of the variable (e.g., cluster, config, instance)",
					},
				},
				"required": []string{"module_name", "variable_name"},
			},
		},
		{
			"name":        "compare_pattern_across_modules",
			"description": "Compare a specific code pattern (e.g., dynamic blocks, resource definitions) across all modules to find differences. Returns a summary table by default, or full code blocks if requested.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{
						"type":        "string",
						"description": "The pattern to search for (e.g., 'dynamic \"identity\"', 'resource \"azurerm_', 'lifecycle {')",
					},
					"file_type": map[string]any{
						"type":        "string",
						"description": "Optional: filter by file type (e.g., 'main.tf', 'variables.tf'). Leave empty for all .tf files.",
					},
					"show_full_blocks": map[string]any{
						"type":        "boolean",
						"description": "Optional: show full code blocks instead of summary (default: false for compact table view)",
					},
				},
				"required": []string{"pattern"},
			},
		},
		{
			"name":        "list_module_examples",
			"description": "List all available usage examples for a specific module",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"module_name": map[string]any{
						"type":        "string",
						"description": "Name of the module (e.g., terraform-azure-aks)",
					},
				},
				"required": []string{"module_name"},
			},
		},
		{
			"name":        "get_example_content",
			"description": "Get the complete content of a specific example including all files (main.tf, variables.tf, etc.)",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"module_name": map[string]any{
						"type":        "string",
						"description": "Name of the module (e.g., terraform-azure-aks)",
					},
					"example_name": map[string]any{
						"type":        "string",
						"description": "Name of the example (e.g., 'default', 'complete')",
					},
				},
				"required": []string{"module_name", "example_name"},
			},
		},
	}

	response := Message{
		JSONRPC: "2.0",
		ID:      msg.ID,
		Result: map[string]any{
			"tools": tools,
		},
	}
	s.sendResponse(response)
}

func (s *Server) handleToolsCall(msg Message) {
	paramsBytes, err := json.Marshal(msg.Params)
	if err != nil {
		s.sendError(-32602, "Invalid params", msg.ID)
		return
	}

	var params ToolCallParams
	if err := json.Unmarshal(paramsBytes, &params); err != nil {
		s.sendError(-32602, "Invalid params", msg.ID)
		return
	}

	log.Printf("Tool call: %s", params.Name)

	var result any
	switch params.Name {
	case "sync_modules":
		result = s.handleSyncModules()
	case "sync_updates_modules":
		result = s.handleSyncUpdatesModules()
	case "list_modules":
		result = s.handleListModules()
	case "search_modules":
		result = s.handleSearchModules(params.Arguments)
	case "get_module_info":
		result = s.handleGetModuleInfo(params.Arguments)
	case "search_code":
		result = s.handleSearchCode(params.Arguments)
	case "get_file_content":
		result = s.handleGetFileContent(params.Arguments)
	case "extract_variable_definition":
		result = s.handleExtractVariableDefinition(params.Arguments)
	case "compare_pattern_across_modules":
		result = s.handleComparePatternAcrossModules(params.Arguments)
	case "list_module_examples":
		result = s.handleListModuleExamples(params.Arguments)
	case "get_example_content":
		result = s.handleGetExampleContent(params.Arguments)
	default:
		s.sendError(-32601, "Tool not found", msg.ID)
		return
	}

	response := Message{
		JSONRPC: "2.0",
		ID:      msg.ID,
		Result:  result,
	}
	s.sendResponse(response)
}

func (s *Server) handleSyncModules() map[string]any {
	log.Println("Starting full repository sync...")

	progress, err := s.syncer.SyncAll()
	if err != nil {
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": fmt.Sprintf("Sync failed: %v", err),
				},
			},
		}
	}

	var text strings.Builder
	text.WriteString("# Sync Completed\n\n")
	text.WriteString(fmt.Sprintf("Successfully synced %d/%d repositories\n\n",
		progress.ProcessedRepos-len(progress.Errors), progress.TotalRepos))

	if len(progress.Errors) > 0 {
		text.WriteString(fmt.Sprintf("%d errors occurred:\n", len(progress.Errors)))
		for i, err := range progress.Errors {
			if i >= 10 {
				text.WriteString(fmt.Sprintf("... and %d more errors\n", len(progress.Errors)-10))
				break
			}
			text.WriteString(fmt.Sprintf("- %s\n", err))
		}
	}

	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": text.String(),
			},
		},
	}
}

func (s *Server) handleSyncUpdatesModules() map[string]any {
	log.Println("Starting incremental repository sync (updates only)...")

	progress, err := s.syncer.SyncUpdates()
	if err != nil {
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": fmt.Sprintf("Sync failed: %v", err),
				},
			},
		}
	}

	var text strings.Builder
	text.WriteString("# Incremental Sync Completed\n\n")

	synced := progress.ProcessedRepos - len(progress.Errors) - progress.SkippedRepos

	text.WriteString(fmt.Sprintf("Checked %d repositories\n", progress.TotalRepos))
	text.WriteString(fmt.Sprintf("Updated modules: %d\n", synced))
	text.WriteString(fmt.Sprintf("Skipped (up-to-date): %d\n\n", progress.SkippedRepos))

	if len(progress.Errors) > 0 {
		text.WriteString(fmt.Sprintf("%d errors occurred:\n", len(progress.Errors)))
		for i, err := range progress.Errors {
			if i >= 10 {
				text.WriteString(fmt.Sprintf("... and %d more errors\n", len(progress.Errors)-10))
				break
			}
			text.WriteString(fmt.Sprintf("- %s\n", err))
		}
	}

	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": text.String(),
			},
		},
	}
}

func (s *Server) handleListModules() map[string]any {
	modules, err := s.db.ListModules()
	if err != nil {
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": fmt.Sprintf("Error loading modules: %v", err),
				},
			},
		}
	}

	if len(modules) == 0 {
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": "No modules found. Run sync_modules tool to fetch modules from GitHub.",
				},
			},
		}
	}

	var text strings.Builder
	text.WriteString(fmt.Sprintf("# Azure CloudNation Terraform Modules (%d modules)\n\n", len(modules)))

	for i, module := range modules {
		if i >= 50 { // Show more modules now that we're not hitting GitHub
			text.WriteString(fmt.Sprintf("... and %d more modules\n", len(modules)-50))
			break
		}
		text.WriteString(fmt.Sprintf("**%s**\n", module.Name))
		if module.Description != "" {
			text.WriteString(fmt.Sprintf("  %s\n", module.Description))
		}
		text.WriteString(fmt.Sprintf("  Repo: %s\n", module.RepoURL))
		text.WriteString(fmt.Sprintf("  Last synced: %s\n\n", module.SyncedAt.Format("2006-01-02 15:04:05")))
	}

	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": text.String(),
			},
		},
	}
}

func (s *Server) handleSearchModules(args any) map[string]any {
	argsBytes, _ := json.Marshal(args)
	var searchArgs struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(argsBytes, &searchArgs); err != nil {
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": "Error: Invalid search query",
				},
			},
		}
	}

	if searchArgs.Limit == 0 {
		searchArgs.Limit = 10
	}

	modules, err := s.db.SearchModules(searchArgs.Query, searchArgs.Limit)
	if err != nil {
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": fmt.Sprintf("Error searching modules: %v", err),
				},
			},
		}
	}

	var text strings.Builder
	text.WriteString(fmt.Sprintf("# Search Results for '%s' (%d matches)\n\n", searchArgs.Query, len(modules)))

	for _, module := range modules {
		text.WriteString(fmt.Sprintf("**%s**\n", module.Name))
		if module.Description != "" {
			text.WriteString(fmt.Sprintf("  %s\n", module.Description))
		}
		text.WriteString(fmt.Sprintf("  Repo: %s\n\n", module.RepoURL))
	}

	if len(modules) == 0 {
		text.WriteString("No modules found matching your query.\n")
	}

	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": text.String(),
			},
		},
	}
}

func (s *Server) handleGetModuleInfo(args any) map[string]any {
	argsBytes, _ := json.Marshal(args)
	var moduleArgs struct {
		ModuleName string `json:"module_name"`
	}
	if err := json.Unmarshal(argsBytes, &moduleArgs); err != nil {
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": "Error: Invalid module name",
				},
			},
		}
	}

	module, err := s.db.GetModule(moduleArgs.ModuleName)
	if err != nil {
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": fmt.Sprintf("Module '%s' not found", moduleArgs.ModuleName),
				},
			},
		}
	}

	var text strings.Builder
	text.WriteString(fmt.Sprintf("# %s\n\n", module.Name))

	if module.Description != "" {
		text.WriteString(fmt.Sprintf("**Description:** %s\n\n", module.Description))
	}

	text.WriteString(fmt.Sprintf("**Repository:** %s\n", module.RepoURL))
	text.WriteString(fmt.Sprintf("**Last Updated:** %s\n", module.LastUpdated))
	text.WriteString(fmt.Sprintf("**Last Synced:** %s\n\n", module.SyncedAt.Format("2006-01-02 15:04:05")))

	// Get variables
	variables, err := s.db.GetModuleVariables(module.ID)
	if err == nil && len(variables) > 0 {
		text.WriteString("## Variables\n\n")
		for _, v := range variables {
			text.WriteString(fmt.Sprintf("- **%s**", v.Name))
			if v.Type != "" {
				text.WriteString(fmt.Sprintf(" (`%s`)", v.Type))
			}
			if v.Required {
				text.WriteString(" *[required]*")
			}
			if v.Sensitive {
				text.WriteString(" *[sensitive]*")
			}
			if v.DefaultValue != "" {
				text.WriteString(fmt.Sprintf(" - default: `%s`", v.DefaultValue))
			}
			if v.Description != "" {
				text.WriteString(fmt.Sprintf("\n  %s", v.Description))
			}
			text.WriteString("\n")
		}
		text.WriteString("\n")
	}

	// Get outputs
	outputs, err := s.db.GetModuleOutputs(module.ID)
	if err == nil && len(outputs) > 0 {
		text.WriteString("## Outputs\n\n")
		for _, o := range outputs {
			text.WriteString(fmt.Sprintf("- **%s**", o.Name))
			if o.Sensitive {
				text.WriteString(" *[sensitive]*")
			}
			if o.Description != "" {
				text.WriteString(fmt.Sprintf("\n  %s", o.Description))
			}
			text.WriteString("\n")
		}
		text.WriteString("\n")
	}

	// Get resources
	resources, err := s.db.GetModuleResources(module.ID)
	if err == nil && len(resources) > 0 {
		text.WriteString(fmt.Sprintf("## Resources (%d)\n\n", len(resources)))
		for i, r := range resources {
			if i >= 20 {
				text.WriteString(fmt.Sprintf("... and %d more resources\n", len(resources)-20))
				break
			}
			text.WriteString(fmt.Sprintf("- `%s.%s`", r.ResourceType, r.ResourceName))
			if r.SourceFile != "" {
				text.WriteString(fmt.Sprintf(" (in %s)", r.SourceFile))
			}
			text.WriteString("\n")
		}
		text.WriteString("\n")
	}

	// Get files
	files, err := s.db.GetModuleFiles(module.ID)
	if err == nil && len(files) > 0 {
		text.WriteString(fmt.Sprintf("## Files (%d)\n\n", len(files)))
		for i, f := range files {
			if i >= 30 {
				text.WriteString(fmt.Sprintf("... and %d more files\n", len(files)-30))
				break
			}
			text.WriteString(fmt.Sprintf("- %s", f.FilePath))
			if f.SizeBytes > 0 {
				text.WriteString(fmt.Sprintf(" (%d bytes)", f.SizeBytes))
			}
			text.WriteString("\n")
		}
		text.WriteString("\n")
	}

	// Show README excerpt if available
	if module.ReadmeContent != "" {
		text.WriteString("## README (excerpt)\n\n")
		lines := strings.Split(module.ReadmeContent, "\n")
		lineCount := 0
		for _, line := range lines {
			if lineCount >= 30 {
				text.WriteString("\n... (truncated, see full README at repository)\n")
				break
			}
			text.WriteString(line + "\n")
			lineCount++
		}
	}

	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": text.String(),
			},
		},
	}
}

func (s *Server) handleSearchCode(args any) map[string]any {
	argsBytes, _ := json.Marshal(args)
	var searchArgs struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(argsBytes, &searchArgs); err != nil {
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": "Error: Invalid search query",
				},
			},
		}
	}

	if searchArgs.Limit == 0 {
		searchArgs.Limit = 20
	}

	files, err := s.db.SearchFiles(searchArgs.Query, searchArgs.Limit)
	if err != nil {
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": fmt.Sprintf("Error searching code: %v", err),
				},
			},
		}
	}

	var text strings.Builder
	text.WriteString(fmt.Sprintf("# Code Search Results for '%s' (%d matches)\n\n", searchArgs.Query, len(files)))

	if len(files) == 0 {
		text.WriteString("No code matches found.\n")
	}

	for _, file := range files {
		// Get module name
		module, err := s.db.GetModuleByID(file.ModuleID)
		moduleName := "unknown"
		if err == nil {
			moduleName = module.Name
		}

		text.WriteString(fmt.Sprintf("## %s / %s\n", moduleName, file.FilePath))
		text.WriteString("```\n")

		// Show relevant lines with context
		lines := strings.Split(file.Content, "\n")
		queryLower := strings.ToLower(searchArgs.Query)

		for i, line := range lines {
			if strings.Contains(strings.ToLower(line), queryLower) {
				// Show 2 lines before and after for context
				start := max(i-2, 0)
				end := min(i+3, len(lines))

				for j := start; j < end; j++ {
					if j == i {
						text.WriteString(fmt.Sprintf("→ %d: %s\n", j+1, lines[j]))
					} else {
						text.WriteString(fmt.Sprintf("  %d: %s\n", j+1, lines[j]))
					}
				}
				text.WriteString("...\n")
				break // Only show first match in this file
			}
		}

		text.WriteString("```\n\n")
	}

	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": text.String(),
			},
		},
	}
}

func (s *Server) handleGetFileContent(args any) map[string]any {
	argsBytes, _ := json.Marshal(args)
	var fileArgs struct {
		ModuleName string `json:"module_name"`
		FilePath   string `json:"file_path"`
	}
	if err := json.Unmarshal(argsBytes, &fileArgs); err != nil {
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": "Error: Invalid parameters",
				},
			},
		}
	}

	file, err := s.db.GetFile(fileArgs.ModuleName, fileArgs.FilePath)
	if err != nil {
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": fmt.Sprintf("File '%s' not found in module '%s'", fileArgs.FilePath, fileArgs.ModuleName),
				},
			},
		}
	}

	var text strings.Builder
	text.WriteString(fmt.Sprintf("# %s / %s\n\n", fileArgs.ModuleName, file.FilePath))
	text.WriteString(fmt.Sprintf("**Size:** %d bytes\n", file.SizeBytes))
	text.WriteString(fmt.Sprintf("**Type:** %s\n\n", file.FileType))
	text.WriteString("```hcl\n")
	text.WriteString(file.Content)
	text.WriteString("\n```\n")

	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": text.String(),
			},
		},
	}
}

func (s *Server) handleExtractVariableDefinition(args any) map[string]any {
	argsBytes, _ := json.Marshal(args)
	var varArgs struct {
		ModuleName   string `json:"module_name"`
		VariableName string `json:"variable_name"`
	}
	if err := json.Unmarshal(argsBytes, &varArgs); err != nil {
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": "Error: Invalid parameters",
				},
			},
		}
	}

	// Get the variables.tf file
	file, err := s.db.GetFile(varArgs.ModuleName, "variables.tf")
	if err != nil {
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": fmt.Sprintf("variables.tf not found in module '%s'", varArgs.ModuleName),
				},
			},
		}
	}

	// Extract the specific variable block
	variablePattern := fmt.Sprintf(`variable "%s"`, varArgs.VariableName)
	startIdx := strings.Index(file.Content, variablePattern)
	if startIdx == -1 {
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": fmt.Sprintf("Variable '%s' not found in %s", varArgs.VariableName, varArgs.ModuleName),
				},
			},
		}
	}

	// Find the closing brace of the variable block
	braceCount := 0
	inBlock := false
	endIdx := startIdx

	for i := startIdx; i < len(file.Content); i++ {
		char := file.Content[i]
		if char == '{' {
			braceCount++
			inBlock = true
		} else if char == '}' {
			braceCount--
			if inBlock && braceCount == 0 {
				endIdx = i + 1
				break
			}
		}
	}

	variableBlock := file.Content[startIdx:endIdx]

	var text strings.Builder
	text.WriteString(fmt.Sprintf("# %s / variable \"%s\"\n\n", varArgs.ModuleName, varArgs.VariableName))
	text.WriteString("```hcl\n")
	text.WriteString(variableBlock)
	text.WriteString("\n```\n")

	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": text.String(),
			},
		},
	}
}

func (s *Server) handleComparePatternAcrossModules(args any) map[string]any {
	argsBytes, _ := json.Marshal(args)
	var patternArgs struct {
		Pattern        string `json:"pattern"`
		FileType       string `json:"file_type"`
		ShowFullBlocks bool   `json:"show_full_blocks"`
	}
	if err := json.Unmarshal(argsBytes, &patternArgs); err != nil {
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": "Error: Invalid parameters",
				},
			},
		}
	}

	// Get all modules
	modules, err := s.db.ListModules()
	if err != nil {
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": fmt.Sprintf("Error loading modules: %v", err),
				},
			},
		}
	}

	var results []struct {
		ModuleName string
		FileName   string
		Match      string
	}

	// Search through all modules
	for _, module := range modules {
		files, err := s.db.GetModuleFiles(module.ID)
		if err != nil {
			continue
		}

		for _, file := range files {
			// Filter by file type if specified
			if patternArgs.FileType != "" && file.FileName != patternArgs.FileType {
				continue
			}

			// Only search .tf files
			if !strings.HasSuffix(file.FileName, ".tf") {
				continue
			}

			// Find ALL matches of the pattern (not just the first one)
			searchContent := file.Content
			offset := 0
			matchCount := 0

			for {
				idx := strings.Index(searchContent, patternArgs.Pattern)
				if idx == -1 {
					break
				}

				actualIdx := offset + idx
				matchCount++

				// Extract the block containing the pattern
				startIdx := actualIdx

				// Find start of block (look backwards for opening brace or newline)
				for startIdx > 0 && file.Content[startIdx] != '\n' {
					startIdx--
				}

				// Find end of block (look for closing brace)
				endIdx := actualIdx
				braceCount := 0
				inBlock := false

				for i := actualIdx; i < len(file.Content); i++ {
					char := file.Content[i]
					if char == '{' {
						braceCount++
						inBlock = true
					} else if char == '}' {
						braceCount--
						if inBlock && braceCount == 0 {
							endIdx = i + 1
							// Find end of line
							for endIdx < len(file.Content) && file.Content[endIdx] != '\n' {
								endIdx++
							}
							break
						}
					}
				}

				if endIdx > startIdx {
					match := strings.TrimSpace(file.Content[startIdx:endIdx])

					// Add match count to module name if multiple matches in same file
					displayName := module.Name
					if matchCount > 1 {
						displayName = fmt.Sprintf("%s #%d", module.Name, matchCount)
					}

					results = append(results, struct {
						ModuleName string
						FileName   string
						Match      string
					}{
						ModuleName: displayName,
						FileName:   file.FileName,
						Match:      match,
					})
				}

				// Move past this match to find next one
				offset = actualIdx + len(patternArgs.Pattern)
				if offset >= len(file.Content) {
					break
				}
				searchContent = file.Content[offset:]
			}
		}
	}

	// Format output
	var text strings.Builder
	text.WriteString(fmt.Sprintf("# Pattern Comparison: '%s'\n\n", patternArgs.Pattern))
	text.WriteString(fmt.Sprintf("Found %d matches across modules\n\n", len(results)))

	if len(results) == 0 {
		text.WriteString("No matches found.\n")
	} else {
		if patternArgs.ShowFullBlocks {
			// Show full code blocks
			for _, result := range results {
				text.WriteString(fmt.Sprintf("## %s (%s)\n\n", result.ModuleName, result.FileName))
				text.WriteString("```hcl\n")
				text.WriteString(result.Match)
				text.WriteString("\n```\n\n")
			}
		} else {
			// Show compact summary table
			text.WriteString("| Module | File | Preview |\n")
			text.WriteString("|--------|------|---------|\n")
			for _, result := range results {
				// Get first line as preview
				firstLine := strings.Split(result.Match, "\n")[0]
				if len(firstLine) > 60 {
					firstLine = firstLine[:60] + "..."
				}
				firstLine = strings.ReplaceAll(firstLine, "|", "\\|")
				text.WriteString(fmt.Sprintf("| %s | %s | %s |\n", result.ModuleName, result.FileName, firstLine))
			}
			text.WriteString("\n**Tip:** Use `show_full_blocks: true` to see complete code blocks\n")
		}
	}

	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": text.String(),
			},
		},
	}
}

func (s *Server) handleListModuleExamples(args any) map[string]any {
	argsBytes, _ := json.Marshal(args)
	var moduleArgs struct {
		ModuleName string `json:"module_name"`
	}
	if err := json.Unmarshal(argsBytes, &moduleArgs); err != nil {
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": "Error: Invalid parameters",
				},
			},
		}
	}

	module, err := s.db.GetModule(moduleArgs.ModuleName)
	if err != nil {
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": fmt.Sprintf("Module '%s' not found", moduleArgs.ModuleName),
				},
			},
		}
	}

	// Get all files in examples/ directory
	files, err := s.db.GetModuleFiles(module.ID)
	if err != nil {
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": fmt.Sprintf("Error getting files: %v", err),
				},
			},
		}
	}

	// Extract unique example names from examples/ paths
	exampleMap := make(map[string][]string)
	for _, file := range files {
		if strings.HasPrefix(file.FilePath, "examples/") {
			parts := strings.Split(file.FilePath, "/")
			if len(parts) >= 3 {
				exampleName := parts[1]
				exampleMap[exampleName] = append(exampleMap[exampleName], file.FileName)
			}
		}
	}

	var text strings.Builder
	text.WriteString(fmt.Sprintf("# Examples for %s\n\n", moduleArgs.ModuleName))

	if len(exampleMap) == 0 {
		text.WriteString("No examples found for this module.\n")
	} else {
		text.WriteString(fmt.Sprintf("Found %d example(s):\n\n", len(exampleMap)))
		for exampleName, fileList := range exampleMap {
			text.WriteString(fmt.Sprintf("## %s\n", exampleName))
			text.WriteString("Files:\n")
			for _, fileName := range fileList {
				text.WriteString(fmt.Sprintf("- %s\n", fileName))
			}
			text.WriteString("\n")
		}
	}

	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": text.String(),
			},
		},
	}
}

func (s *Server) handleGetExampleContent(args any) map[string]any {
	argsBytes, _ := json.Marshal(args)
	var exampleArgs struct {
		ModuleName  string `json:"module_name"`
		ExampleName string `json:"example_name"`
	}
	if err := json.Unmarshal(argsBytes, &exampleArgs); err != nil {
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": "Error: Invalid parameters",
				},
			},
		}
	}

	module, err := s.db.GetModule(exampleArgs.ModuleName)
	if err != nil {
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": fmt.Sprintf("Module '%s' not found", exampleArgs.ModuleName),
				},
			},
		}
	}

	// Get all files for this module
	files, err := s.db.GetModuleFiles(module.ID)
	if err != nil {
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": fmt.Sprintf("Error getting files: %v", err),
				},
			},
		}
	}

	// Filter files that belong to this example
	examplePrefix := fmt.Sprintf("examples/%s/", exampleArgs.ExampleName)
	var exampleFiles []database.ModuleFile
	for _, file := range files {
		if strings.HasPrefix(file.FilePath, examplePrefix) {
			exampleFiles = append(exampleFiles, file)
		}
	}

	if len(exampleFiles) == 0 {
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": fmt.Sprintf("Example '%s' not found in module '%s'", exampleArgs.ExampleName, exampleArgs.ModuleName),
				},
			},
		}
	}

	var text strings.Builder
	text.WriteString(fmt.Sprintf("# %s / examples/%s\n\n", exampleArgs.ModuleName, exampleArgs.ExampleName))
	text.WriteString(fmt.Sprintf("Contains %d file(s)\n\n", len(exampleFiles)))

	// Sort files: main.tf first, then others
	sortedFiles := make([]database.ModuleFile, 0, len(exampleFiles))
	var mainFile *database.ModuleFile
	for i := range exampleFiles {
		if exampleFiles[i].FileName == "main.tf" {
			mainFile = &exampleFiles[i]
		} else {
			sortedFiles = append(sortedFiles, exampleFiles[i])
		}
	}
	if mainFile != nil {
		sortedFiles = append([]database.ModuleFile{*mainFile}, sortedFiles...)
	}

	for _, file := range sortedFiles {
		text.WriteString(fmt.Sprintf("## %s\n\n", file.FileName))
		text.WriteString("```hcl\n")
		text.WriteString(file.Content)
		text.WriteString("\n```\n\n")
	}

	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": text.String(),
			},
		},
	}
}

func (s *Server) sendResponse(response Message) {
	data, err := json.Marshal(response)
	if err != nil {
		log.Printf("Failed to marshal response: %v", err)
		return
	}

	if s.writer == nil {
		log.Printf("No writer configured, dropping response: %s", string(data))
		return
	}

	if _, err := fmt.Fprintln(s.writer, string(data)); err != nil {
		log.Printf("Failed to write response: %v", err)
		return
	}
	log.Printf("Sent: %s", string(data))
}

func (s *Server) sendError(code int, message string, id any) {
	response := Message{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
	}
	s.sendResponse(response)
}
