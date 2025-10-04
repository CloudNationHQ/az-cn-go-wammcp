package indexer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/cloudnationhq/az-cn-wam-mcp/internal/database"
	"github.com/cloudnationhq/az-cn-wam-mcp/internal/util"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

type Syncer struct {
	db           *database.DB
	githubClient *GitHubClient
	org          string
}

type GitHubRepo struct {
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	UpdatedAt   string `json:"updated_at"`
	HTMLURL     string `json:"html_url"`
	Private     bool   `json:"private"`
	Archived    bool   `json:"archived"`
	Size        int    `json:"size"`
}

type GitHubContent struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	DownloadURL string `json:"download_url"`
	Content     string `json:"content"`
	Size        int64  `json:"size"`
}

type GitHubClient struct {
	httpClient *http.Client
	cache      map[string]CacheEntry
	cacheMutex sync.RWMutex
	rateLimit  *RateLimiter
	token      string
}

type paginatedResponse struct {
	data    []byte
	nextURL string
}

type CacheEntry struct {
	Data      any
	ExpiresAt time.Time
}

type RateLimiter struct {
	tokens    int
	maxTokens int
	refillAt  time.Time
	mutex     sync.Mutex
}

type SyncProgress struct {
	TotalRepos     int
	ProcessedRepos int
	SkippedRepos   int
	CurrentRepo    string
	Errors         []string
	UpdatedRepos   []string
}

var ErrRepoContentUnavailable = errors.New("repository content unavailable")

func NewSyncer(db *database.DB, token string, org string) *Syncer {
	client := &GitHubClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		cache:      make(map[string]CacheEntry),
		rateLimit:  &RateLimiter{tokens: 60, maxTokens: 60, refillAt: time.Now().Add(time.Hour)},
		token:      token,
	}

	if token != "" {
		client.rateLimit.maxTokens = 5000
		client.rateLimit.tokens = 5000
	}

	return &Syncer{
		db:           db,
		githubClient: client,
		org:          org,
	}
}

func (s *Syncer) SyncAll() (*SyncProgress, error) {
	progress := &SyncProgress{}

	log.Println("Fetching repositories from GitHub...")
	repos, err := s.fetchRepositories()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch repositories: %w", err)
	}

	progress.TotalRepos = len(repos)
	log.Printf("Found %d repositories", len(repos))

	for _, repo := range repos {
		progress.CurrentRepo = repo.Name
		log.Printf("Syncing repository: %s (%d/%d)", repo.Name, progress.ProcessedRepos+1, progress.TotalRepos)

		if err := s.syncRepository(repo); err != nil {
			errMsg := fmt.Sprintf("Failed to sync %s: %v", repo.Name, err)
			log.Println(errMsg)
			progress.Errors = append(progress.Errors, errMsg)
		}

		progress.ProcessedRepos++
	}

	log.Printf("Sync completed: %d/%d repositories synced successfully",
		progress.ProcessedRepos-len(progress.Errors), progress.TotalRepos)

	return progress, nil
}

func (s *Syncer) SyncUpdates() (*SyncProgress, error) {
	progress := &SyncProgress{}

	s.githubClient.clearCache()
	log.Println("Fetching repositories from GitHub (cache cleared)...")
	repos, err := s.fetchRepositories()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch repositories: %w", err)
	}

	progress.TotalRepos = len(repos)
	log.Printf("Found %d repositories", len(repos))

	for _, repo := range repos {
		progress.CurrentRepo = repo.Name

		existingModule, err := s.db.GetModule(repo.Name)

		if err != nil {
			log.Printf("Module %s not found in DB (error: %v), will sync", repo.Name, err)
		} else if existingModule == nil {
			log.Printf("Module %s not found in DB (nil), will sync", repo.Name)
		} else if existingModule.LastUpdated == repo.UpdatedAt {
			log.Printf("Skipping %s (already up-to-date)", repo.Name)
			progress.SkippedRepos++
			progress.ProcessedRepos++
			continue
		} else {
			log.Printf("Module %s needs update: DB='%s' vs GitHub='%s'", repo.Name, existingModule.LastUpdated, repo.UpdatedAt)
		}

		log.Printf("Syncing repository: %s (%d/%d)", repo.Name, progress.ProcessedRepos+1, progress.TotalRepos)

		syncErr := s.syncRepository(repo)
		if syncErr != nil {
			errMsg := fmt.Sprintf("Failed to sync %s: %v", repo.Name, syncErr)
			log.Println(errMsg)
			progress.Errors = append(progress.Errors, errMsg)
		} else {
			progress.UpdatedRepos = append(progress.UpdatedRepos, repo.Name)
		}

		progress.ProcessedRepos++
	}

	syncedCount := len(progress.UpdatedRepos)

	log.Printf("Sync completed: %d/%d repositories synced, %d skipped (up-to-date), %d errors",
		syncedCount, progress.TotalRepos, progress.SkippedRepos, len(progress.Errors))

	return progress, nil
}

func (s *Syncer) fetchRepositories() ([]GitHubRepo, error) {
	url := fmt.Sprintf("https://api.github.com/orgs/%s/repos?per_page=100", s.org)

	var allRepos []GitHubRepo
	for url != "" {
		data, nextURL, err := s.githubClient.getWithPagination(url)
		if err != nil {
			return nil, err
		}

		var pageRepos []GitHubRepo
		if err := json.Unmarshal(data, &pageRepos); err != nil {
			return nil, err
		}

		allRepos = append(allRepos, pageRepos...)
		url = nextURL
	}

	var terraformRepos []GitHubRepo
	for _, repo := range allRepos {
		if !strings.HasPrefix(repo.Name, "terraform-azure-") {
			continue
		}

		if repo.Private {
			log.Printf("Skipping %s (private repository)", repo.Name)
			continue
		}

		if repo.Archived {
			log.Printf("Skipping %s (archived repository)", repo.Name)
			continue
		}

		if repo.Size <= 0 {
			log.Printf("Skipping %s (empty repository)", repo.Name)
			continue
		}

		terraformRepos = append(terraformRepos, repo)
	}

	return terraformRepos, nil
}

func (s *Syncer) syncRepository(repo GitHubRepo) error {
	moduleID, err := s.insertModuleMetadata(repo)
	if err != nil {
		return err
	}

	if err := s.clearExistingModuleData(moduleID, repo.Name); err != nil {
		log.Printf("Warning: failed to clear old data for %s: %v", repo.Name, err)
	}

	if err := s.syncReadme(moduleID, repo); err != nil {
		log.Printf("Warning: failed to fetch README for %s: %v", repo.Name, err)
	}

	hasExamples, submoduleIDs, err := s.syncRepositoryContent(moduleID, repo)
	if err != nil {
		if errors.Is(err, ErrRepoContentUnavailable) {
			return s.handleUnavailableRepo(moduleID, repo.Name)
		}
		return fmt.Errorf("failed to sync files: %w", err)
	}

	if err := s.parseModulesAndSubmodules(moduleID, submoduleIDs, repo.Name); err != nil {
		log.Printf("Warning: failed to parse terraform files: %v", err)
	}

	if hasExamples {
		s.markModuleHasExamples(moduleID)
	}

	return nil
}

func (s *Syncer) insertModuleMetadata(repo GitHubRepo) (int64, error) {
	module := &database.Module{
		Name:        repo.Name,
		FullName:    repo.FullName,
		Description: repo.Description,
		RepoURL:     repo.HTMLURL,
		LastUpdated: repo.UpdatedAt,
	}

	moduleID, err := s.db.InsertModule(module)
	if err != nil {
		return 0, fmt.Errorf("failed to insert module: %w", err)
	}

	return moduleID, nil
}

func (s *Syncer) clearExistingModuleData(moduleID int64, repoName string) error {
	existingModule, _ := s.db.GetModuleByID(moduleID)
	if existingModule != nil && existingModule.ID != 0 {
		if err := s.db.ClearModuleData(moduleID); err != nil {
			return err
		}
	}

	return s.db.DeleteChildModules(repoName)
}

func (s *Syncer) syncReadme(moduleID int64, repo GitHubRepo) error {
	readme, err := s.fetchReadme(repo.FullName)
	if err != nil {
		return err
	}

	module := &database.Module{
		ID:            moduleID,
		Name:          repo.Name,
		FullName:      repo.FullName,
		Description:   repo.Description,
		RepoURL:       repo.HTMLURL,
		LastUpdated:   repo.UpdatedAt,
		ReadmeContent: readme,
	}

	_, err = s.db.InsertModule(module)
	return err
}

func (s *Syncer) syncRepositoryContent(moduleID int64, repo GitHubRepo) (bool, []int64, error) {
	return s.syncRepositoryFromArchive(moduleID, repo)
}

func (s *Syncer) handleUnavailableRepo(moduleID int64, repoName string) error {
	log.Printf("Skipping %s: repository content unavailable", repoName)
	if delErr := s.db.DeleteModuleByID(moduleID); delErr != nil {
		log.Printf("Warning: failed to delete module record for %s: %v", repoName, delErr)
	}
	return nil
}

func (s *Syncer) parseModulesAndSubmodules(moduleID int64, submoduleIDs []int64, repoName string) error {
	if err := s.parseAndIndexTerraformFiles(moduleID); err != nil {
		log.Printf("Warning: failed to parse terraform files for %s: %v", repoName, err)
	}

	for _, childID := range submoduleIDs {
		if err := s.parseAndIndexTerraformFiles(childID); err != nil {
			log.Printf("Warning: failed to parse terraform files for submodule %d of %s: %v", childID, repoName, err)
		}
	}

	return nil
}

func (s *Syncer) markModuleHasExamples(moduleID int64) {
	module := &database.Module{
		ID:          moduleID,
		HasExamples: true,
	}
	s.db.InsertModule(module)
}

func (s *Syncer) syncRepositoryFromArchive(moduleID int64, repo GitHubRepo) (bool, []int64, error) {
	archiveURL := fmt.Sprintf("https://api.github.com/repos/%s/tarball", repo.FullName)
	data, err := s.githubClient.getArchive(archiveURL)
	if err != nil {
		if errors.Is(err, ErrRepoContentUnavailable) {
			return false, nil, ErrRepoContentUnavailable
		}
		return false, nil, err
	}

	tarReader, err := openTarArchive(data)
	if err != nil {
		return false, nil, err
	}

	return s.processArchiveEntries(tarReader, moduleID, repo)
}

func openTarArchive(data []byte) (*tar.Reader, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to open archive: %w", err)
	}
	return tar.NewReader(gzipReader), nil
}

func (s *Syncer) processArchiveEntries(tarReader *tar.Reader, moduleID int64, repo GitHubRepo) (bool, []int64, error) {
	examplesFound := false
	submoduleIDs := make(map[string]int64)
	var submoduleOrder []int64

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, nil, fmt.Errorf("failed to read archive: %w", err)
		}

		if !isRegularFile(header.Typeflag) {
			continue
		}

		relativePath := normalizeArchivePath(header.Name)
		if relativePath == "" || shouldSkipPath(relativePath) {
			continue
		}

		contentBytes, err := io.ReadAll(tarReader)
		if err != nil {
			return false, nil, fmt.Errorf("failed to read file %s: %w", relativePath, err)
		}

		targetModuleID, updated := s.resolveTargetModule(moduleID, relativePath, repo, submoduleIDs, &submoduleOrder)
		if updated {
			// Module ID was updated
		}

		if err := s.insertModuleFile(targetModuleID, relativePath, header.Size, contentBytes); err != nil {
			log.Printf("Warning: failed to insert file %s: %v", relativePath, err)
		}

		if strings.HasPrefix(relativePath, "examples/") {
			examplesFound = true
		}
	}

	return examplesFound, submoduleOrder, nil
}

func (s *Syncer) resolveTargetModule(moduleID int64, relativePath string, repo GitHubRepo, submoduleIDs map[string]int64, submoduleOrder *[]int64) (int64, bool) {
	if !strings.HasPrefix(relativePath, "modules/") {
		return moduleID, false
	}

	parts := strings.Split(relativePath, "/")
	if len(parts) < 2 {
		return moduleID, false
	}

	subKey := parts[1]
	if subID, ok := submoduleIDs[subKey]; ok {
		return subID, false
	}

	childID, err := s.ensureSubmoduleModule(repo, subKey)
	if err != nil {
		log.Printf("Warning: failed to ensure submodule %s for %s: %v", subKey, repo.Name, err)
		return moduleID, false
	}

	submoduleIDs[subKey] = childID
	*submoduleOrder = append(*submoduleOrder, childID)
	return childID, true
}

func (s *Syncer) insertModuleFile(moduleID int64, relativePath string, size int64, content []byte) error {
	fileName := path.Base(relativePath)
	file := &database.ModuleFile{
		ModuleID:  moduleID,
		FileName:  fileName,
		FilePath:  relativePath,
		FileType:  getFileType(fileName),
		Content:   string(content),
		SizeBytes: size,
	}

	return s.db.InsertFile(file)
}

func normalizeArchivePath(name string) string {
	parts := strings.SplitN(name, "/", 2)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

func shouldSkipPath(relativePath string) bool {
	skipDirs := map[string]struct{}{
		".git":         {},
		".github":      {},
		"node_modules": {},
		".terraform":   {},
	}

	segments := strings.Split(relativePath, "/")
	for _, segment := range segments {
		if _, skip := skipDirs[segment]; skip {
			return true
		}
	}

	return false
}

func (s *Syncer) ensureSubmoduleModule(repo GitHubRepo, subKey string) (int64, error) {
	submoduleName := fmt.Sprintf("%s//modules/%s", repo.Name, subKey)
	module := &database.Module{
		Name:        submoduleName,
		FullName:    repo.FullName,
		Description: fmt.Sprintf("Submodule %s of %s", subKey, repo.Name),
		RepoURL:     repo.HTMLURL,
		LastUpdated: repo.UpdatedAt,
	}

	moduleID, err := s.db.InsertModule(module)
	if err != nil {
		return 0, fmt.Errorf("failed to insert submodule %s: %w", submoduleName, err)
	}

	if err := s.db.ClearModuleData(moduleID); err != nil {
		log.Printf("Warning: failed to clear old data for submodule %s: %v", submoduleName, err)
	}

	return moduleID, nil
}

func isRegularFile(typeFlag byte) bool {
	return typeFlag == tar.TypeReg
}

func (s *Syncer) fetchReadme(repoFullName string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/readme", repoFullName)
	data, err := s.githubClient.get(url)
	if err != nil {
		return "", err
	}

	var content GitHubContent
	if err := json.Unmarshal(data, &content); err != nil {
		return "", err
	}

	return s.fetchFileContent(content)
}

func (s *Syncer) fetchFileContent(content GitHubContent) (string, error) {
	if content.DownloadURL != "" {
		data, err := s.githubClient.get(content.DownloadURL)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	if content.Content != "" {
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(content.Content, "\n", ""))
		if err != nil {
			return "", err
		}
		return string(decoded), nil
	}

	return "", fmt.Errorf("no content available")
}

func (s *Syncer) parseAndIndexTerraformFiles(moduleID int64) error {
	files, err := s.db.GetModuleFiles(moduleID)
	if err != nil {
		return err
	}

	for _, file := range files {
		if file.FileType != "terraform" {
			continue
		}

		if err := s.parseAndIndexTerraformFile(moduleID, file); err != nil {
			log.Printf("Warning: failed to parse %s: %v", file.FilePath, err)
		}
	}

	return nil
}

func (s *Syncer) parseAndIndexTerraformFile(moduleID int64, file database.ModuleFile) error {
	body, err := parseHCLBody(file.Content, file.FilePath)
	if err != nil {
		return err
	}

	s.indexVariables(moduleID, body, file.Content)
	s.indexOutputs(moduleID, body, file.Content)
	s.indexResources(moduleID, body, file.FileName)
	s.indexDataSources(moduleID, body, file.FileName)

	return nil
}

func (s *Syncer) indexVariables(moduleID int64, body *hclsyntax.Body, content string) {
	variables := extractVariables(body, content)
	for _, v := range variables {
		v.ModuleID = moduleID
		if err := s.db.InsertVariable(&v); err != nil {
			log.Printf("Warning: failed to insert variable: %v", err)
		}
	}
}

func (s *Syncer) indexOutputs(moduleID int64, body *hclsyntax.Body, content string) {
	outputs := extractOutputs(body, content)
	for _, o := range outputs {
		o.ModuleID = moduleID
		if err := s.db.InsertOutput(&o); err != nil {
			log.Printf("Warning: failed to insert output: %v", err)
		}
	}
}

func (s *Syncer) indexResources(moduleID int64, body *hclsyntax.Body, fileName string) {
	resources := extractResources(body, fileName)
	for _, r := range resources {
		r.ModuleID = moduleID
		if err := s.db.InsertResource(&r); err != nil {
			log.Printf("Warning: failed to insert resource: %v", err)
		}
	}
}

func (s *Syncer) indexDataSources(moduleID int64, body *hclsyntax.Body, fileName string) {
	dataSources := extractDataSources(body, fileName)
	for _, d := range dataSources {
		d.ModuleID = moduleID
		if err := s.db.InsertDataSource(&d); err != nil {
			log.Printf("Warning: failed to insert data source: %v", err)
		}
	}
}

func parseHCLBody(content string, filename string) (*hclsyntax.Body, error) {
	parser := hclparse.NewParser()
	file, diags := parser.ParseHCL([]byte(content), filename)
	if diags.HasErrors() {
		return nil, fmt.Errorf(diags.Error())
	}

	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, fmt.Errorf("unexpected HCL body type for %s", filename)
	}

	return body, nil
}

func extractVariables(body *hclsyntax.Body, content string) []database.ModuleVariable {
	var variables []database.ModuleVariable

	for _, block := range body.Blocks {
		if block.Type != "variable" || len(block.Labels) == 0 {
			continue
		}

		variable := database.ModuleVariable{
			Name:     block.Labels[0],
			Required: true,
		}

		if attr, ok := block.Body.Attributes["type"]; ok {
			variable.Type = strings.TrimSpace(expressionText(content, attr.Expr.Range()))
		}

		if attr, ok := block.Body.Attributes["description"]; ok {
			if literal, ok := attr.Expr.(*hclsyntax.LiteralValueExpr); ok && literal.Val.Type() == cty.String {
				variable.Description = literal.Val.AsString()
			}
		}

		if attr, ok := block.Body.Attributes["default"]; ok {
			variable.Required = false
			variable.DefaultValue = strings.TrimSpace(expressionText(content, attr.Expr.Range()))
		}

		if attr, ok := block.Body.Attributes["sensitive"]; ok {
			variable.Sensitive = attributeIsTrue(attr, content)
		}

		variables = append(variables, variable)
	}

	return variables
}

func extractOutputs(body *hclsyntax.Body, content string) []database.ModuleOutput {
	var outputs []database.ModuleOutput

	for _, block := range body.Blocks {
		if block.Type != "output" || len(block.Labels) == 0 {
			continue
		}

		output := database.ModuleOutput{
			Name: block.Labels[0],
		}

		if attr, ok := block.Body.Attributes["description"]; ok {
			if literal, ok := attr.Expr.(*hclsyntax.LiteralValueExpr); ok && literal.Val.Type() == cty.String {
				output.Description = literal.Val.AsString()
			}
		}

		if attr, ok := block.Body.Attributes["sensitive"]; ok {
			output.Sensitive = attributeIsTrue(attr, content)
		}

		outputs = append(outputs, output)
	}

	return outputs
}

func extractResources(body *hclsyntax.Body, fileName string) []database.ModuleResource {
	var resources []database.ModuleResource

	for _, block := range body.Blocks {
		if block.Type != "resource" || len(block.Labels) < 2 {
			continue
		}

		resourceType := block.Labels[0]
		resource := database.ModuleResource{
			ResourceType: resourceType,
			ResourceName: block.Labels[1],
			Provider:     providerFromType(resourceType),
			SourceFile:   fileName,
		}

		resources = append(resources, resource)
	}

	return resources
}

func extractDataSources(body *hclsyntax.Body, fileName string) []database.ModuleDataSource {
	var dataSources []database.ModuleDataSource

	for _, block := range body.Blocks {
		if block.Type != "data" || len(block.Labels) < 2 {
			continue
		}

		dataType := block.Labels[0]
		dataSource := database.ModuleDataSource{
			DataType:   dataType,
			DataName:   block.Labels[1],
			Provider:   providerFromType(dataType),
			SourceFile: fileName,
		}

		dataSources = append(dataSources, dataSource)
	}

	return dataSources
}

func attributeIsTrue(attr *hclsyntax.Attribute, content string) bool {
	if literal, ok := attr.Expr.(*hclsyntax.LiteralValueExpr); ok && literal.Val.Type() == cty.Bool {
		return literal.Val.True()
	}

	text := strings.TrimSpace(expressionText(content, attr.Expr.Range()))
	return strings.EqualFold(text, "true")
}

func expressionText(content string, rng hcl.Range) string {
	data := []byte(content)
	start := rng.Start.Byte
	end := rng.End.Byte

	if start < 0 {
		start = 0
	}
	if end > len(data) {
		end = len(data)
	}
	if end < start {
		end = start
	}

	return string(data[start:end])
}

func providerFromType(fullType string) string {
	return util.ExtractProvider(fullType)
}

func getFileType(fileName string) string {
	if strings.HasSuffix(fileName, ".tf") {
		return "terraform"
	} else if strings.HasSuffix(fileName, ".md") {
		return "markdown"
	} else if strings.HasSuffix(fileName, ".yml") || strings.HasSuffix(fileName, ".yaml") {
		return "yaml"
	} else if strings.HasSuffix(fileName, ".json") {
		return "json"
	}
	return "other"
}

func (rl *RateLimiter) acquire() bool {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	if time.Now().After(rl.refillAt) {
		rl.tokens = rl.maxTokens
		rl.refillAt = time.Now().Add(time.Hour)
	}

	if rl.tokens > 0 {
		rl.tokens--
		return true
	}
	return false
}

func (gc *GitHubClient) clearCache() {
	gc.cacheMutex.Lock()
	gc.cache = make(map[string]CacheEntry)
	gc.cacheMutex.Unlock()
}

func (gc *GitHubClient) get(url string) ([]byte, error) {
	gc.cacheMutex.RLock()
	if entry, exists := gc.cache[url]; exists && time.Now().Before(entry.ExpiresAt) {
		gc.cacheMutex.RUnlock()
		if data, ok := entry.Data.([]byte); ok {
			return data, nil
		}
	}
	gc.cacheMutex.RUnlock()

	if !gc.rateLimit.acquire() {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if gc.token != "" {
		req.Header.Set("Authorization", "token "+gc.token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "az-cn-wam-mcp/1.0.0")

	resp, err := gc.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API error: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	gc.cacheMutex.Lock()
	gc.cache[url] = CacheEntry{
		Data:      data,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	gc.cacheMutex.Unlock()

	return data, nil
}

func (gc *GitHubClient) getArchive(url string) ([]byte, error) {
	if !gc.rateLimit.acquire() {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if gc.token != "" {
		req.Header.Set("Authorization", "token "+gc.token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "az-cn-wam-mcp/1.0.0")

	resp, err := gc.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusConflict {
		return nil, fmt.Errorf("%w: status %d", ErrRepoContentUnavailable, resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API error: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (gc *GitHubClient) getWithPagination(url string) ([]byte, string, error) {
	gc.cacheMutex.RLock()
	if entry, exists := gc.cache[url]; exists && time.Now().Before(entry.ExpiresAt) {
		gc.cacheMutex.RUnlock()
		if cached, ok := entry.Data.(paginatedResponse); ok {
			return cached.data, cached.nextURL, nil
		}
	}
	gc.cacheMutex.RUnlock()

	data, headers, err := gc.doRequest(url)
	if err != nil {
		return nil, "", err
	}

	nextURL := parseNextLink(headers.Get("Link"))

	gc.cacheMutex.Lock()
	gc.cache[url] = CacheEntry{
		Data:      paginatedResponse{data: data, nextURL: nextURL},
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	gc.cacheMutex.Unlock()

	return data, nextURL, nil
}

func (gc *GitHubClient) doRequest(url string) ([]byte, http.Header, error) {
	if !gc.rateLimit.acquire() {
		return nil, nil, fmt.Errorf("rate limit exceeded")
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, nil, err
	}

	if gc.token != "" {
		req.Header.Set("Authorization", "token "+gc.token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "az-cn-wam-mcp/1.0.0")

	resp, err := gc.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("GitHub API error: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	return data, resp.Header.Clone(), nil
}

func parseNextLink(linkHeader string) string {
	if linkHeader == "" {
		return ""
	}

	for _, part := range strings.Split(linkHeader, ",") {
		sections := strings.Split(strings.TrimSpace(part), ";")
		if len(sections) < 2 {
			continue
		}

		urlPart := strings.Trim(sections[0], " <>")
		var rel string
		for _, sec := range sections[1:] {
			sec = strings.TrimSpace(sec)
			if trimmed, ok := strings.CutPrefix(sec, "rel="); ok {
				rel = strings.Trim(trimmed, "\"")
			}
		}

		if rel == "next" {
			return urlPart
		}
	}

	return ""
}
