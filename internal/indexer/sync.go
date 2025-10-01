package indexer

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cloudnationhq/az-cn-wam-mcp/internal/database"
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
	data, err := s.githubClient.get(url)
	if err != nil {
		return nil, err
	}

	var repos []GitHubRepo
	if err := json.Unmarshal(data, &repos); err != nil {
		return nil, err
	}

	var terraformRepos []GitHubRepo
	for _, repo := range repos {
		if strings.HasPrefix(repo.Name, "terraform-") {
			terraformRepos = append(terraformRepos, repo)
		}
	}

	return terraformRepos, nil
}

func (s *Syncer) syncRepository(repo GitHubRepo) error {
	module := &database.Module{
		Name:        repo.Name,
		FullName:    repo.FullName,
		Description: repo.Description,
		RepoURL:     repo.HTMLURL,
		LastUpdated: repo.UpdatedAt,
	}

	moduleID, err := s.db.InsertModule(module)
	if err != nil {
		return fmt.Errorf("failed to insert module: %w", err)
	}

	existingModule, _ := s.db.GetModuleByID(moduleID)
	if existingModule != nil && existingModule.ID != 0 {
		if err := s.db.ClearModuleData(moduleID); err != nil {
			log.Printf("Warning: failed to clear old data for %s: %v", repo.Name, err)
		}
	}

	readme, err := s.fetchReadme(repo.FullName)
	if err != nil {
		log.Printf("Warning: failed to fetch README for %s: %v", repo.Name, err)
	} else {
		module.ReadmeContent = readme
		module.ID = moduleID
		s.db.InsertModule(module) // Update with README
	}

	if err := s.syncRepositoryFiles(moduleID, repo.FullName, ""); err != nil {
		return fmt.Errorf("failed to sync files: %w", err)
	}

	if err := s.parseAndIndexTerraformFiles(moduleID); err != nil {
		log.Printf("Warning: failed to parse terraform files for %s: %v", repo.Name, err)
	}

	hasExamples := s.hasExamplesDirectory(repo.FullName)
	if hasExamples {
		module.HasExamples = true
		module.ID = moduleID
		s.db.InsertModule(module)

		if err := s.syncExamples(moduleID, repo.FullName); err != nil {
			log.Printf("Warning: failed to sync examples for %s: %v", repo.Name, err)
		}
	}

	return nil
}

func (s *Syncer) syncRepositoryFiles(moduleID int64, repoFullName, path string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s", repoFullName, path)
	data, err := s.githubClient.get(url)
	if err != nil {
		return err
	}

	var contents []GitHubContent
	if err := json.Unmarshal(data, &contents); err != nil {
		return err
	}

	for _, content := range contents {
		if content.Type == "dir" {
			skipDirs := []string{".github", ".git", "node_modules", ".terraform"}
			if slices.Contains(skipDirs, content.Name) {
				continue
			}

			if err := s.syncRepositoryFiles(moduleID, repoFullName, content.Path); err != nil {
				log.Printf("Warning: failed to sync directory %s: %v", content.Path, err)
			}
			continue
		}

		if content.Type == "file" {
			fileContent, err := s.fetchFileContent(content)
			if err != nil {
				log.Printf("Warning: failed to fetch file %s: %v", content.Path, err)
				continue
			}

			file := &database.ModuleFile{
				ModuleID:  moduleID,
				FileName:  content.Name,
				FilePath:  content.Path,
				FileType:  getFileType(content.Name),
				Content:   fileContent,
				SizeBytes: content.Size,
			}

			if err := s.db.InsertFile(file); err != nil {
				log.Printf("Warning: failed to insert file %s: %v", content.Path, err)
			}
		}
	}

	return nil
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

func (s *Syncer) hasExamplesDirectory(repoFullName string) bool {
	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/examples", repoFullName)
	_, err := s.githubClient.get(url)
	return err == nil
}

func (s *Syncer) syncExamples(moduleID int64, repoFullName string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/examples", repoFullName)
	data, err := s.githubClient.get(url)
	if err != nil {
		return err
	}

	var contents []GitHubContent
	if err := json.Unmarshal(data, &contents); err != nil {
		return err
	}

	for _, content := range contents {
		if content.Type == "dir" {
			exampleFiles, err := s.fetchExampleFiles(repoFullName, content.Path)
			if err != nil {
				log.Printf("Warning: failed to fetch example %s: %v", content.Name, err)
				continue
			}

			example := &database.ModuleExample{
				ModuleID: moduleID,
				Name:     content.Name,
				Path:     content.Path,
				Content:  exampleFiles,
			}

			if err := s.db.InsertExample(example); err != nil {
				log.Printf("Warning: failed to insert example %s: %v", content.Name, err)
			}
		}
	}

	return nil
}

func (s *Syncer) fetchExampleFiles(repoFullName, path string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s", repoFullName, path)
	data, err := s.githubClient.get(url)
	if err != nil {
		return "", err
	}

	var contents []GitHubContent
	if err := json.Unmarshal(data, &contents); err != nil {
		return "", err
	}

	var result strings.Builder
	for _, content := range contents {
		if content.Type == "file" && strings.HasSuffix(content.Name, ".tf") {
			fileContent, err := s.fetchFileContent(content)
			if err != nil {
				continue
			}
			result.WriteString(fmt.Sprintf("# %s\n", content.Name))
			result.WriteString(fileContent)
			result.WriteString("\n\n")
		}
	}

	return result.String(), nil
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

		variables := parseVariables(file.Content)
		for _, v := range variables {
			v.ModuleID = moduleID
			if err := s.db.InsertVariable(&v); err != nil {
				log.Printf("Warning: failed to insert variable: %v", err)
			}
		}

		outputs := parseOutputs(file.Content)
		for _, o := range outputs {
			o.ModuleID = moduleID
			if err := s.db.InsertOutput(&o); err != nil {
				log.Printf("Warning: failed to insert output: %v", err)
			}
		}

		resources := parseResources(file.Content, file.FileName)
		for _, r := range resources {
			r.ModuleID = moduleID
			if err := s.db.InsertResource(&r); err != nil {
				log.Printf("Warning: failed to insert resource: %v", err)
			}
		}

		dataSources := parseDataSources(file.Content, file.FileName)
		for _, d := range dataSources {
			d.ModuleID = moduleID
			if err := s.db.InsertDataSource(&d); err != nil {
				log.Printf("Warning: failed to insert data source: %v", err)
			}
		}
	}

	return nil
}

func parseVariables(content string) []database.ModuleVariable {
	var variables []database.ModuleVariable

	varRegex := regexp.MustCompile(`(?s)variable\s+"([^"]+)"\s*\{([^}]*)\}`)
	matches := varRegex.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		name := match[1]
		block := match[2]

		variable := database.ModuleVariable{
			Name:     name,
			Required: true, // default
		}

		if typeMatch := regexp.MustCompile(`type\s*=\s*([^\n]+)`).FindStringSubmatch(block); len(typeMatch) > 1 {
			variable.Type = strings.TrimSpace(typeMatch[1])
		}

		if descMatch := regexp.MustCompile(`description\s*=\s*"([^"]+)"`).FindStringSubmatch(block); len(descMatch) > 1 {
			variable.Description = descMatch[1]
		}

		if defaultMatch := regexp.MustCompile(`(?s)default\s*=\s*(.+?)(?:\n\s*\w+\s*=|\n\s*\}|$)`).FindStringSubmatch(block); len(defaultMatch) > 1 {
			variable.Required = false
			variable.DefaultValue = strings.TrimSpace(defaultMatch[1])
		}

		if sensitiveMatch := regexp.MustCompile(`sensitive\s*=\s*true`).FindString(block); sensitiveMatch != "" {
			variable.Sensitive = true
		}

		variables = append(variables, variable)
	}

	return variables
}

func parseOutputs(content string) []database.ModuleOutput {
	var outputs []database.ModuleOutput

	outputRegex := regexp.MustCompile(`(?s)output\s+"([^"]+)"\s*\{([^}]*)\}`)
	matches := outputRegex.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		name := match[1]
		block := match[2]

		output := database.ModuleOutput{
			Name: name,
		}

		if descMatch := regexp.MustCompile(`description\s*=\s*"([^"]+)"`).FindStringSubmatch(block); len(descMatch) > 1 {
			output.Description = descMatch[1]
		}

		if sensitiveMatch := regexp.MustCompile(`sensitive\s*=\s*true`).FindString(block); sensitiveMatch != "" {
			output.Sensitive = true
		}

		outputs = append(outputs, output)
	}

	return outputs
}

func parseResources(content, fileName string) []database.ModuleResource {
	var resources []database.ModuleResource

	resourceRegex := regexp.MustCompile(`resource\s+"([^"]+)"\s+"([^"]+)"`)
	matches := resourceRegex.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		resource := database.ModuleResource{
			ResourceType: match[1],
			ResourceName: match[2],
			SourceFile:   fileName,
		}

		parts := strings.SplitN(match[1], "_", 2)
		if len(parts) > 0 {
			resource.Provider = parts[0]
		}

		resources = append(resources, resource)
	}

	return resources
}

func parseDataSources(content, fileName string) []database.ModuleDataSource {
	var dataSources []database.ModuleDataSource

	dataRegex := regexp.MustCompile(`data\s+"([^"]+)"\s+"([^"]+)"`)
	matches := dataRegex.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		dataSource := database.ModuleDataSource{
			DataType:   match[1],
			DataName:   match[2],
			SourceFile: fileName,
		}

		parts := strings.SplitN(match[1], "_", 2)
		if len(parts) > 0 {
			dataSource.Provider = parts[0]
		}

		dataSources = append(dataSources, dataSource)
	}

	return dataSources
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
