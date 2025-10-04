// Package mcp provides the JSON-RPC server for module coordination protocol.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cloudnationhq/az-cn-wam-mcp/internal/database"
	"github.com/cloudnationhq/az-cn-wam-mcp/internal/formatter"
	"github.com/cloudnationhq/az-cn-wam-mcp/internal/indexer"
)

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

type Server struct {
	db        *database.DB
	syncer    *indexer.Syncer
	writer    io.Writer
	jobs      map[string]*SyncJob
	jobsMutex sync.RWMutex
}

func NewServer(db *database.DB, syncer *indexer.Syncer) *Server {
	return &Server{db: db, syncer: syncer, jobs: make(map[string]*SyncJob)}
}

type SyncJob struct {
	ID          string
	Type        string
	Status      string
	StartedAt   time.Time
	CompletedAt *time.Time
	Progress    *indexer.SyncProgress
	Error       string
}

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
					"limit": map[string]any{
						"type":        "number",
						"description": "Optional: maximum number of results to return (default: unlimited for table view, 20 for full blocks)",
					},
					"offset": map[string]any{
						"type":        "number",
						"description": "Optional: number of results to skip for pagination (default: 0)",
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
		{
			"name":        "sync_status",
			"description": "Get status of ongoing or previous sync jobs",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"job_id": map[string]any{
						"type":        "string",
						"description": "Optional job identifier returned by sync commands",
					},
				},
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
	case "sync_status":
		result = s.handleSyncStatus(params.Arguments)
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
	job := s.startSyncJob("full_sync", func() (*indexer.SyncProgress, error) {
		log.Println("Starting full repository sync (async job)...")
		return s.syncer.SyncAll()
	})

	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": fmt.Sprintf("Full sync started.\nJob ID: %s\nUse `sync_status` with this job ID to monitor progress.", job.ID),
			},
		},
	}
}

func (s *Server) handleSyncUpdatesModules() map[string]any {
	log.Println("Starting incremental repository sync (updates only)...")

	progress, err := s.syncer.SyncUpdates()
	if err != nil {
		return ErrorResponse(fmt.Sprintf("Sync failed: %v", err))
	}

	text := formatter.IncrementalSyncProgress(
		progress.TotalRepos,
		len(progress.UpdatedRepos),
		progress.SkippedRepos,
		progress.UpdatedRepos,
		progress.Errors,
	)

	return SuccessResponse(text)
}

func (s *Server) handleSyncStatus(args any) map[string]any {
	statusArgs, err := UnmarshalArgs[struct {
		JobID string `json:"job_id"`
	}](args)
	if err != nil {
		return ErrorResponse("Error: Invalid parameters")
	}

	if statusArgs.JobID != "" {
		job, ok := s.getJob(statusArgs.JobID)
		if !ok {
			return ErrorResponse(fmt.Sprintf("Job '%s' not found", statusArgs.JobID))
		}

		text := s.formatJobDetails(job)
		return SuccessResponse(text)
	}

	jobs := s.listJobs()
	text := s.formatJobList(jobs)
	return SuccessResponse(text)
}

func (s *Server) handleListModules() map[string]any {
	modules, err := s.db.ListModules()
	if err != nil {
		return ErrorResponse(fmt.Sprintf("Error loading modules: %v", err))
	}

	if len(modules) == 0 {
		return SuccessResponse("No modules found. Run sync_modules tool to fetch modules from GitHub.")
	}

	text := formatter.ModuleList(modules)
	return SuccessResponse(text)
}

func (s *Server) handleSearchModules(args any) map[string]any {
	searchArgs, err := UnmarshalArgs[struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}](args)
	if err != nil {
		return ErrorResponse("Error: Invalid search query")
	}

	if searchArgs.Limit == 0 {
		searchArgs.Limit = 10
	}

	modules, err := s.db.SearchModules(searchArgs.Query, searchArgs.Limit)
	if err != nil {
		return ErrorResponse(fmt.Sprintf("Error searching modules: %v", err))
	}

	text := formatter.SearchResults(searchArgs.Query, modules)
	return SuccessResponse(text)
}

func (s *Server) handleGetModuleInfo(args any) map[string]any {
	moduleArgs, err := UnmarshalArgs[struct {
		ModuleName string `json:"module_name"`
	}](args)
	if err != nil {
		return ErrorResponse("Error: Invalid module name")
	}

	module, err := s.db.GetModule(moduleArgs.ModuleName)
	if err != nil {
		return ErrorResponse(fmt.Sprintf("Module '%s' not found", moduleArgs.ModuleName))
	}

	variables, _ := s.db.GetModuleVariables(module.ID)
	outputs, _ := s.db.GetModuleOutputs(module.ID)
	resources, _ := s.db.GetModuleResources(module.ID)
	files, _ := s.db.GetModuleFiles(module.ID)

	text := formatter.ModuleInfo(module, variables, outputs, resources, files)
	return SuccessResponse(text)
}

func (s *Server) handleSearchCode(args any) map[string]any {
	searchArgs, err := UnmarshalArgs[struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}](args)
	if err != nil {
		return ErrorResponse("Error: Invalid search query")
	}

	if searchArgs.Limit == 0 {
		searchArgs.Limit = 20
	}

	files, err := s.db.SearchFiles(searchArgs.Query, searchArgs.Limit)
	if err != nil {
		return ErrorResponse(fmt.Sprintf("Error searching code: %v", err))
	}

	getModuleName := func(moduleID int64) string {
		module, err := s.db.GetModuleByID(moduleID)
		if err == nil {
			return module.Name
		}
		return "unknown"
	}

	text := formatter.CodeSearchResults(searchArgs.Query, files, getModuleName)
	return SuccessResponse(text)
}

func (s *Server) handleGetFileContent(args any) map[string]any {
	fileArgs, err := UnmarshalArgs[struct {
		ModuleName string `json:"module_name"`
		FilePath   string `json:"file_path"`
	}](args)
	if err != nil {
		return ErrorResponse("Error: Invalid parameters")
	}

	file, err := s.db.GetFile(fileArgs.ModuleName, fileArgs.FilePath)
	if err != nil {
		return ErrorResponse(fmt.Sprintf("File '%s' not found in module '%s'", fileArgs.FilePath, fileArgs.ModuleName))
	}

	text := formatter.FileContent(fileArgs.ModuleName, file.FilePath, file.FileType, file.SizeBytes, file.Content)
	return SuccessResponse(text)
}

func (s *Server) handleExtractVariableDefinition(args any) map[string]any {
	varArgs, err := UnmarshalArgs[struct {
		ModuleName   string `json:"module_name"`
		VariableName string `json:"variable_name"`
	}](args)
	if err != nil {
		return ErrorResponse("Error: Invalid parameters")
	}

	file, err := s.db.GetFile(varArgs.ModuleName, "variables.tf")
	if err != nil {
		return ErrorResponse(fmt.Sprintf("variables.tf not found in module '%s'", varArgs.ModuleName))
	}

	variableBlock := extractVariableBlock(file.Content, varArgs.VariableName)
	if variableBlock == "" {
		return ErrorResponse(fmt.Sprintf("Variable '%s' not found in %s", varArgs.VariableName, varArgs.ModuleName))
	}

	text := formatter.VariableDefinition(varArgs.ModuleName, varArgs.VariableName, variableBlock)
	return SuccessResponse(text)
}

func extractVariableBlock(content, variableName string) string {
	variablePattern := fmt.Sprintf(`variable "%s"`, variableName)
	startIdx := strings.Index(content, variablePattern)
	if startIdx == -1 {
		return ""
	}

	braceCount := 0
	inBlock := false
	endIdx := startIdx

Loop:
	for i := startIdx; i < len(content); i++ {
		char := content[i]
		switch char {
		case '{':
			braceCount++
			inBlock = true
		case '}':
			braceCount--
			if inBlock && braceCount == 0 {
				endIdx = i + 1
				break Loop
			}
		}
	}

	return content[startIdx:endIdx]
}

func (s *Server) handleComparePatternAcrossModules(args any) map[string]any {
	patternArgs, err := UnmarshalArgs[struct {
		Pattern        string `json:"pattern"`
		FileType       string `json:"file_type"`
		ShowFullBlocks bool   `json:"show_full_blocks"`
		Limit          int    `json:"limit"`
		Offset         int    `json:"offset"`
	}](args)
	if err != nil {
		return ErrorResponse("Error: Invalid parameters")
	}

	if patternArgs.Limit == 0 && patternArgs.ShowFullBlocks {
		patternArgs.Limit = 20
	}

	modules, err := s.db.ListModules()
	if err != nil {
		return ErrorResponse(fmt.Sprintf("Error loading modules: %v", err))
	}

	results := s.findPatternMatches(modules, patternArgs.Pattern, patternArgs.FileType)
	paginatedResults := paginateResults(results, patternArgs.Offset, patternArgs.Limit)

	text := formatter.PatternComparison(
		patternArgs.Pattern,
		paginatedResults,
		patternArgs.ShowFullBlocks,
		patternArgs.Offset,
		patternArgs.Limit,
		len(results),
	)

	return SuccessResponse(text)
}

func (s *Server) findPatternMatches(modules []database.Module, pattern, fileType string) []formatter.PatternMatch {
	var results []formatter.PatternMatch

	for _, module := range modules {
		files, err := s.db.GetModuleFiles(module.ID)
		if err != nil {
			continue
		}

		for _, file := range files {
			if fileType != "" && file.FileName != fileType {
				continue
			}

			if !strings.HasSuffix(file.FileName, ".tf") {
				continue
			}

			matches := extractPatternMatches(file.Content, pattern)
			for i, match := range matches {
				displayName := module.Name
				if len(matches) > 1 {
					displayName = fmt.Sprintf("%s #%d", module.Name, i+1)
				}
				results = append(results, formatter.PatternMatch{
					ModuleName: displayName,
					FileName:   file.FileName,
					Match:      match,
				})
			}
		}
	}

	return results
}

func extractPatternMatches(content, pattern string) []string {
	var matches []string
	searchContent := content
	offset := 0

	for {
		idx := strings.Index(searchContent, pattern)
		if idx == -1 {
			break
		}

		actualIdx := offset + idx
		startIdx := actualIdx
		for startIdx > 0 && content[startIdx] != '\n' {
			startIdx--
		}

		endIdx := findBlockEnd(content, actualIdx)
		if endIdx > startIdx {
			matches = append(matches, strings.TrimSpace(content[startIdx:endIdx]))
		}

		offset = actualIdx + len(pattern)
		if offset >= len(content) {
			break
		}
		searchContent = content[offset:]
	}

	return matches
}

func findBlockEnd(content string, startPos int) int {
	braceCount := 0
	inBlock := false

	for i := startPos; i < len(content); i++ {
		char := content[i]
		switch char {
		case '{':
			braceCount++
			inBlock = true
		case '}':
			braceCount--
			if inBlock && braceCount == 0 {
				endIdx := i + 1
				for endIdx < len(content) && content[endIdx] != '\n' {
					endIdx++
				}
				return endIdx
			}
		}
	}
	return startPos
}

func paginateResults(results []formatter.PatternMatch, offset, limit int) []formatter.PatternMatch {
	total := len(results)
	startIdx := min(max(0, offset), total)
	endIdx := total
	if limit > 0 {
		endIdx = min(startIdx+limit, total)
	}
	return results[startIdx:endIdx]
}

func (s *Server) handleListModuleExamples(args any) map[string]any {
	moduleArgs, err := UnmarshalArgs[struct {
		ModuleName string `json:"module_name"`
	}](args)
	if err != nil {
		return ErrorResponse("Error: Invalid parameters")
	}

	module, err := s.db.GetModule(moduleArgs.ModuleName)
	if err != nil {
		return ErrorResponse(fmt.Sprintf("Module '%s' not found", moduleArgs.ModuleName))
	}

	files, err := s.db.GetModuleFiles(module.ID)
	if err != nil {
		return ErrorResponse(fmt.Sprintf("Error getting files: %v", err))
	}

	exampleMap := buildExampleMap(files)
	text := formatter.ExampleList(moduleArgs.ModuleName, exampleMap)
	return SuccessResponse(text)
}

func buildExampleMap(files []database.ModuleFile) map[string][]string {
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
	return exampleMap
}

func (s *Server) handleGetExampleContent(args any) map[string]any {
	exampleArgs, err := UnmarshalArgs[struct {
		ModuleName  string `json:"module_name"`
		ExampleName string `json:"example_name"`
	}](args)
	if err != nil {
		return ErrorResponse("Error: Invalid parameters")
	}

	module, err := s.db.GetModule(exampleArgs.ModuleName)
	if err != nil {
		return ErrorResponse(fmt.Sprintf("Module '%s' not found", exampleArgs.ModuleName))
	}

	files, err := s.db.GetModuleFiles(module.ID)
	if err != nil {
		return ErrorResponse(fmt.Sprintf("Error getting files: %v", err))
	}

	exampleFiles := filterExampleFiles(files, exampleArgs.ExampleName)
	if len(exampleFiles) == 0 {
		return ErrorResponse(fmt.Sprintf("Example '%s' not found in module '%s'", exampleArgs.ExampleName, exampleArgs.ModuleName))
	}

	sortedFiles := sortExampleFiles(exampleFiles)
	text := formatter.ExampleContent(exampleArgs.ModuleName, exampleArgs.ExampleName, sortedFiles)
	return SuccessResponse(text)
}

func filterExampleFiles(files []database.ModuleFile, exampleName string) []database.ModuleFile {
	examplePrefix := fmt.Sprintf("examples/%s/", exampleName)
	var exampleFiles []database.ModuleFile
	for _, file := range files {
		if strings.HasPrefix(file.FilePath, examplePrefix) {
			exampleFiles = append(exampleFiles, file)
		}
	}
	return exampleFiles
}

func sortExampleFiles(files []database.ModuleFile) []database.ModuleFile {
	sortedFiles := make([]database.ModuleFile, 0, len(files))
	var mainFile *database.ModuleFile
	for i := range files {
		if files[i].FileName == "main.tf" {
			mainFile = &files[i]
		} else {
			sortedFiles = append(sortedFiles, files[i])
		}
	}
	if mainFile != nil {
		sortedFiles = append([]database.ModuleFile{*mainFile}, sortedFiles...)
	}
	return sortedFiles
}

func (s *Server) startSyncJob(jobType string, runner func() (*indexer.SyncProgress, error)) *SyncJob {
	jobID := fmt.Sprintf("%s-%d", jobType, time.Now().UnixNano())
	job := &SyncJob{
		ID:        jobID,
		Type:      jobType,
		Status:    "running",
		StartedAt: time.Now(),
	}

	s.jobsMutex.Lock()
	s.jobs[jobID] = job
	s.jobsMutex.Unlock()

	go func() {
		headline := fmt.Sprintf("Sync job %s (%s)", jobID, jobType)
		defer func() {
			if r := recover(); r != nil {
				errMsg := fmt.Sprintf("panic: %v", r)
				log.Printf("%s panicked: %v", headline, r)
				s.completeJobWithError(jobID, errMsg)
			}
		}()

		progress, err := runner()
		if err != nil {
			log.Printf("%s failed: %v", headline, err)
			s.completeJobWithError(jobID, err.Error())
			return
		}

		log.Printf("%s completed", headline)
		s.completeJobWithSuccess(jobID, progress)
	}()

	return job
}

func (s *Server) completeJobWithError(jobID, errMsg string) {
	now := time.Now()
	s.jobsMutex.Lock()
	if job, ok := s.jobs[jobID]; ok {
		job.Status = "failed"
		job.Error = errMsg
		job.CompletedAt = &now
	}
	s.jobsMutex.Unlock()
}

func (s *Server) completeJobWithSuccess(jobID string, progress *indexer.SyncProgress) {
	now := time.Now()
	s.jobsMutex.Lock()
	if job, ok := s.jobs[jobID]; ok {
		job.Status = "completed"
		job.Progress = progress
		job.CompletedAt = &now
	}
	s.jobsMutex.Unlock()
}

func (s *Server) getJob(jobID string) (*SyncJob, bool) {
	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()
	job, ok := s.jobs[jobID]
	return job, ok
}

func (s *Server) listJobs() []*SyncJob {
	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()
	jobs := make([]*SyncJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].StartedAt.After(jobs[j].StartedAt)
	})
	return jobs
}

func (s *Server) formatJobDetails(job *SyncJob) string {
	progressText := ""
	if job.Progress != nil {
		progressText = formatter.SyncProgress(job.Progress)
	}

	return formatter.JobDetails(
		job.ID,
		job.Type,
		job.Status,
		job.StartedAt,
		job.CompletedAt,
		job.Error,
		progressText,
	)
}

func (s *Server) formatJobList(jobs []*SyncJob) string {
	jobInfos := make([]formatter.JobInfo, len(jobs))
	for i, job := range jobs {
		jobInfos[i] = formatter.JobInfo{
			ID:          job.ID,
			Type:        job.Type,
			Status:      job.Status,
			StartedAt:   job.StartedAt,
			CompletedAt: job.CompletedAt,
		}
	}
	return formatter.JobList(jobInfos)
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
