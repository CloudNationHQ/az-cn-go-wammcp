// Package database provides persistence for indexed Terraform module metadata.
package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

type Module struct {
	ID            int64
	Name          string
	FullName      string
	Description   string
	RepoURL       string
	LastUpdated   string
	SyncedAt      time.Time
	ReadmeContent string
	HasExamples   bool
}

type ModuleFile struct {
	ID        int64
	ModuleID  int64
	FileName  string
	FilePath  string
	FileType  string
	Content   string
	SizeBytes int64
}

type ModuleVariable struct {
	ID           int64
	ModuleID     int64
	Name         string
	Type         string
	Description  string
	DefaultValue string
	Required     bool
	Sensitive    bool
}

type ModuleOutput struct {
	ID          int64
	ModuleID    int64
	Name        string
	Description string
	Value       string
	Sensitive   bool
}

type ModuleResource struct {
	ID           int64
	ModuleID     int64
	ResourceType string
	ResourceName string
	Provider     string
	SourceFile   string
}

type ModuleDataSource struct {
	ID         int64
	ModuleID   int64
	DataType   string
	DataName   string
	Provider   string
	SourceFile string
}

type ModuleExample struct {
	ID       int64
	ModuleID int64
	Name     string
	Path     string
	Content  string
}

func New(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	if _, err := conn.Exec(Schema); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &DB{conn: conn}, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func escapeFTS5(query string) string {
	query = strings.ReplaceAll(query, `"`, `""`)
	return `"` + query + `"`
}

func (db *DB) InsertModule(m *Module) (int64, error) {
	_, err := db.conn.Exec(`
		INSERT INTO modules (name, full_name, description, repo_url, last_updated, readme_content, has_examples)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			full_name = excluded.full_name,
			description = excluded.description,
			repo_url = excluded.repo_url,
			last_updated = excluded.last_updated,
			readme_content = excluded.readme_content,
			has_examples = excluded.has_examples,
			synced_at = CURRENT_TIMESTAMP
	`, m.Name, m.FullName, m.Description, m.RepoURL, m.LastUpdated, m.ReadmeContent, m.HasExamples)
	if err != nil {
		return 0, err
	}

	var id int64
	if err := db.conn.QueryRow(`SELECT id FROM modules WHERE name = ?`, m.Name).Scan(&id); err != nil {
		return 0, err
	}

	return id, nil
}

func (db *DB) GetModule(name string) (*Module, error) {
	var m Module
	err := db.conn.QueryRow(`
		SELECT id, name, full_name, description, repo_url, last_updated, synced_at, readme_content, has_examples
		FROM modules WHERE name = ?
	`, name).Scan(&m.ID, &m.Name, &m.FullName, &m.Description, &m.RepoURL, &m.LastUpdated, &m.SyncedAt, &m.ReadmeContent, &m.HasExamples)

	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (db *DB) GetModuleByID(id int64) (*Module, error) {
	var m Module
	err := db.conn.QueryRow(`
		SELECT id, name, full_name, description, repo_url, last_updated, synced_at, readme_content, has_examples
		FROM modules WHERE id = ?
	`, id).Scan(&m.ID, &m.Name, &m.FullName, &m.Description, &m.RepoURL, &m.LastUpdated, &m.SyncedAt, &m.ReadmeContent, &m.HasExamples)

	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (db *DB) ListModules() ([]Module, error) {
	rows, err := db.conn.Query(`
		SELECT id, name, full_name, description, repo_url, last_updated, synced_at, readme_content, has_examples
		FROM modules ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var modules []Module
	for rows.Next() {
		var m Module
		if err := rows.Scan(&m.ID, &m.Name, &m.FullName, &m.Description, &m.RepoURL, &m.LastUpdated, &m.SyncedAt, &m.ReadmeContent, &m.HasExamples); err != nil {
			return nil, err
		}
		modules = append(modules, m)
	}

	return modules, rows.Err()
}

func (db *DB) SearchModules(query string, limit int) ([]Module, error) {
	rows, err := db.conn.Query(`
		SELECT m.id, m.name, m.full_name, m.description, m.repo_url, m.last_updated, m.synced_at, m.readme_content, m.has_examples
		FROM modules m
		JOIN modules_fts ON modules_fts.rowid = m.id
		WHERE modules_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, escapeFTS5(query), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var modules []Module
	for rows.Next() {
		var m Module
		if err := rows.Scan(&m.ID, &m.Name, &m.FullName, &m.Description, &m.RepoURL, &m.LastUpdated, &m.SyncedAt, &m.ReadmeContent, &m.HasExamples); err != nil {
			return nil, err
		}
		modules = append(modules, m)
	}

	return modules, rows.Err()
}

func (db *DB) InsertFile(f *ModuleFile) error {
	_, err := db.conn.Exec(`
		INSERT INTO module_files (module_id, file_name, file_path, file_type, content, size_bytes)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(module_id, file_path) DO UPDATE SET
			file_name = excluded.file_name,
			file_type = excluded.file_type,
			content = excluded.content,
			size_bytes = excluded.size_bytes
	`, f.ModuleID, f.FileName, f.FilePath, f.FileType, f.Content, f.SizeBytes)

	return err
}

func (db *DB) GetModuleFiles(moduleID int64) ([]ModuleFile, error) {
	rows, err := db.conn.Query(`
		SELECT id, module_id, file_name, file_path, file_type, content, size_bytes
		FROM module_files WHERE module_id = ?
	`, moduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []ModuleFile
	for rows.Next() {
		var f ModuleFile
		if err := rows.Scan(&f.ID, &f.ModuleID, &f.FileName, &f.FilePath, &f.FileType, &f.Content, &f.SizeBytes); err != nil {
			return nil, err
		}
		files = append(files, f)
	}

	return files, rows.Err()
}

func (db *DB) SearchFiles(query string, limit int) ([]ModuleFile, error) {
	rows, err := db.conn.Query(`
		SELECT mf.id, mf.module_id, mf.file_name, mf.file_path, mf.file_type, mf.content, mf.size_bytes
		FROM module_files mf
		JOIN files_fts ON files_fts.rowid = mf.id
		WHERE files_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, escapeFTS5(query), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []ModuleFile
	for rows.Next() {
		var f ModuleFile
		if err := rows.Scan(&f.ID, &f.ModuleID, &f.FileName, &f.FilePath, &f.FileType, &f.Content, &f.SizeBytes); err != nil {
			return nil, err
		}
		files = append(files, f)
	}

	return files, rows.Err()
}

func (db *DB) GetFile(moduleName string, filePath string) (*ModuleFile, error) {
	var f ModuleFile
	err := db.conn.QueryRow(`
		SELECT mf.id, mf.module_id, mf.file_name, mf.file_path, mf.file_type, mf.content, mf.size_bytes
		FROM module_files mf
		JOIN modules m ON m.id = mf.module_id
		WHERE m.name = ? AND mf.file_path = ?
	`, moduleName, filePath).Scan(&f.ID, &f.ModuleID, &f.FileName, &f.FilePath, &f.FileType, &f.Content, &f.SizeBytes)

	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (db *DB) InsertVariable(v *ModuleVariable) error {
	_, err := db.conn.Exec(`
		INSERT INTO module_variables (module_id, name, type, description, default_value, required, sensitive)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, v.ModuleID, v.Name, v.Type, v.Description, v.DefaultValue, v.Required, v.Sensitive)
	return err
}

func (db *DB) GetModuleVariables(moduleID int64) ([]ModuleVariable, error) {
	rows, err := db.conn.Query(`
		SELECT id, module_id, name, type, description, default_value, required, sensitive
		FROM module_variables WHERE module_id = ?
	`, moduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var vars []ModuleVariable
	for rows.Next() {
		var v ModuleVariable
		if err := rows.Scan(&v.ID, &v.ModuleID, &v.Name, &v.Type, &v.Description, &v.DefaultValue, &v.Required, &v.Sensitive); err != nil {
			return nil, err
		}
		vars = append(vars, v)
	}

	return vars, rows.Err()
}

func (db *DB) InsertOutput(o *ModuleOutput) error {
	_, err := db.conn.Exec(`
		INSERT INTO module_outputs (module_id, name, description, value, sensitive)
		VALUES (?, ?, ?, ?, ?)
	`, o.ModuleID, o.Name, o.Description, o.Value, o.Sensitive)
	return err
}

func (db *DB) GetModuleOutputs(moduleID int64) ([]ModuleOutput, error) {
	rows, err := db.conn.Query(`
		SELECT id, module_id, name, description, value, sensitive
		FROM module_outputs WHERE module_id = ?
	`, moduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var outputs []ModuleOutput
	for rows.Next() {
		var o ModuleOutput
		if err := rows.Scan(&o.ID, &o.ModuleID, &o.Name, &o.Description, &o.Value, &o.Sensitive); err != nil {
			return nil, err
		}
		outputs = append(outputs, o)
	}

	return outputs, rows.Err()
}

func (db *DB) InsertResource(r *ModuleResource) error {
	_, err := db.conn.Exec(`
		INSERT INTO module_resources (module_id, resource_type, resource_name, provider, source_file)
		VALUES (?, ?, ?, ?, ?)
	`, r.ModuleID, r.ResourceType, r.ResourceName, r.Provider, r.SourceFile)
	return err
}

func (db *DB) GetModuleResources(moduleID int64) ([]ModuleResource, error) {
	rows, err := db.conn.Query(`
		SELECT id, module_id, resource_type, resource_name, provider, source_file
		FROM module_resources WHERE module_id = ?
	`, moduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var resources []ModuleResource
	for rows.Next() {
		var r ModuleResource
		if err := rows.Scan(&r.ID, &r.ModuleID, &r.ResourceType, &r.ResourceName, &r.Provider, &r.SourceFile); err != nil {
			return nil, err
		}
		resources = append(resources, r)
	}

	return resources, rows.Err()
}

func (db *DB) InsertDataSource(d *ModuleDataSource) error {
	_, err := db.conn.Exec(`
		INSERT INTO module_data_sources (module_id, data_type, data_name, provider, source_file)
		VALUES (?, ?, ?, ?, ?)
	`, d.ModuleID, d.DataType, d.DataName, d.Provider, d.SourceFile)
	return err
}

func (db *DB) GetModuleDataSources(moduleID int64) ([]ModuleDataSource, error) {
	rows, err := db.conn.Query(`
		SELECT id, module_id, data_type, data_name, provider, source_file
		FROM module_data_sources WHERE module_id = ?
	`, moduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dataSources []ModuleDataSource
	for rows.Next() {
		var d ModuleDataSource
		if err := rows.Scan(&d.ID, &d.ModuleID, &d.DataType, &d.DataName, &d.Provider, &d.SourceFile); err != nil {
			return nil, err
		}
		dataSources = append(dataSources, d)
	}

	return dataSources, rows.Err()
}

func (db *DB) InsertExample(e *ModuleExample) error {
	_, err := db.conn.Exec(`
		INSERT INTO module_examples (module_id, name, path, content)
		VALUES (?, ?, ?, ?)
	`, e.ModuleID, e.Name, e.Path, e.Content)
	return err
}

func (db *DB) GetModuleExamples(moduleID int64) ([]ModuleExample, error) {
	rows, err := db.conn.Query(`
		SELECT id, module_id, name, path, content
		FROM module_examples WHERE module_id = ?
	`, moduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var examples []ModuleExample
	for rows.Next() {
		var e ModuleExample
		if err := rows.Scan(&e.ID, &e.ModuleID, &e.Name, &e.Path, &e.Content); err != nil {
			return nil, err
		}
		examples = append(examples, e)
	}

	return examples, rows.Err()
}

func (db *DB) ClearModuleData(moduleID int64) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tables := []string{
		"module_files",
		"module_variables",
		"module_outputs",
		"module_resources",
		"module_data_sources",
		"module_examples",
	}

	for _, table := range tables {
		if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE module_id = ?", table), moduleID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) DeleteModuleByID(moduleID int64) error {
	_, err := db.conn.Exec(`DELETE FROM modules WHERE id = ?`, moduleID)
	return err
}

func (db *DB) DeleteChildModules(parentName string) error {
	pattern := parentName + "//%"
	_, err := db.conn.Exec(`DELETE FROM modules WHERE name LIKE ? ESCAPE '\\'`, pattern)
	return err
}
