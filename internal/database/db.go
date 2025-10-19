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

type ModuleAlias struct {
	ID       int64
	ModuleID int64
	Alias    string
	Weight   int
	Source   sql.NullString
}

type ModuleTag struct {
	ID       int64
	ModuleID int64
	Tag      string
	Weight   int
	Source   sql.NullString
}

type HCLBlock struct {
	ID        int64
	ModuleID  int64
	FilePath  string
	BlockType string
	TypeLabel sql.NullString
	StartByte int64
	EndByte   int64
	AttrPaths sql.NullString
}

type HCLRelationship struct {
	ID            int64
	ModuleID      int64
	FilePath      string
	BlockType     string
	BlockLabels   string
	AttributePath string
	ReferenceType string
	ReferenceName string
	StartByte     int64
	EndByte       int64
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

func (db *DB) SearchFilesFTS(match string, limit int) ([]ModuleFile, error) {
	rows, err := db.conn.Query(`
        SELECT mf.id, mf.module_id, mf.file_name, mf.file_path, mf.file_type, mf.content, mf.size_bytes
        FROM module_files mf
        JOIN files_fts ON files_fts.rowid = mf.id
        WHERE files_fts MATCH ?
        ORDER BY rank
        LIMIT ?
    `, match, limit)
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
		"hcl_blocks",
		"hcl_relationships",
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

func (db *DB) SetModuleHasExamples(moduleID int64, hasExamples bool) error {
	_, err := db.conn.Exec(`
        UPDATE modules
        SET has_examples = ?
        WHERE id = ?
    `, hasExamples, moduleID)
	return err
}

func (db *DB) InsertHCLBlock(moduleID int64, filePath, blockType, typeLabel string, startByte, endByte int, attrPaths string) (int64, error) {
	res, err := db.conn.Exec(`
        INSERT INTO hcl_blocks (module_id, file_path, block_type, type_label, start_byte, end_byte, attr_paths)
        VALUES (?, ?, ?, ?, ?, ?, ?)
    `, moduleID, filePath, blockType, nullIfEmpty(typeLabel), startByte, endByte, nullIfEmpty(attrPaths))
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

// QueryHCLBlocks finds blocks by type and optional label match.
// If prefix is true, matches rows where type_label starts with the given value.
func (db *DB) QueryHCLBlocks(blockType, typeLabel string, prefix bool) ([]HCLBlock, error) {
	var rows *sql.Rows
	var err error
	if blockType == "lifecycle" {
		rows, err = db.conn.Query(`
            SELECT id, module_id, file_path, block_type, type_label, start_byte, end_byte, attr_paths
            FROM hcl_blocks
            WHERE block_type = 'lifecycle'
        `)
	} else if prefix {
		like := typeLabel + "%"
		rows, err = db.conn.Query(`
            SELECT id, module_id, file_path, block_type, type_label, start_byte, end_byte, attr_paths
            FROM hcl_blocks
            WHERE block_type = ? AND type_label LIKE ?
        `, blockType, like)
	} else {
		rows, err = db.conn.Query(`
            SELECT id, module_id, file_path, block_type, type_label, start_byte, end_byte, attr_paths
            FROM hcl_blocks
            WHERE block_type = ? AND type_label = ?
        `, blockType, typeLabel)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []HCLBlock
	for rows.Next() {
		var b HCLBlock
		if err := rows.Scan(&b.ID, &b.ModuleID, &b.FilePath, &b.BlockType, &b.TypeLabel, &b.StartByte, &b.EndByte, &b.AttrPaths); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (db *DB) InsertRelationship(r *HCLRelationship) error {
	_, err := db.conn.Exec(`
        INSERT INTO hcl_relationships (
            module_id,
            file_path,
            block_type,
            block_labels,
            attribute_path,
            reference_type,
            reference_name,
            start_byte,
            end_byte
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    `, r.ModuleID, r.FilePath, r.BlockType, nullIfEmpty(r.BlockLabels), r.AttributePath, r.ReferenceType, r.ReferenceName, r.StartByte, r.EndByte)
	return err
}

func (db *DB) QueryRelationships(moduleID int64, term string, limit int) ([]HCLRelationship, error) {
	if limit <= 0 {
		limit = 20
	}

	likeTerm := "%" + strings.ToLower(term) + "%"

	rows, err := db.conn.Query(`
        SELECT
            id,
            module_id,
            file_path,
            block_type,
            block_labels,
            attribute_path,
            reference_type,
            reference_name,
            start_byte,
            end_byte
        FROM hcl_relationships
        WHERE module_id = ?
          AND (
                LOWER(attribute_path) LIKE ?
             OR LOWER(reference_name) LIKE ?
             OR LOWER(IFNULL(block_labels, '')) LIKE ?
             OR LOWER(block_type) LIKE ?
          )
        ORDER BY file_path, start_byte
        LIMIT ?
    `, moduleID, likeTerm, likeTerm, likeTerm, likeTerm, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []HCLRelationship
	for rows.Next() {
		var rel HCLRelationship
		var blockLabels sql.NullString
		if err := rows.Scan(
			&rel.ID,
			&rel.ModuleID,
			&rel.FilePath,
			&rel.BlockType,
			&blockLabels,
			&rel.AttributePath,
			&rel.ReferenceType,
			&rel.ReferenceName,
			&rel.StartByte,
			&rel.EndByte,
		); err != nil {
			return nil, err
		}
		if blockLabels.Valid {
			rel.BlockLabels = blockLabels.String
		}
		results = append(results, rel)
	}

	return results, rows.Err()
}

func (db *DB) QueryRelationshipsAny(term string, limit int) ([]HCLRelationship, error) {
	if limit <= 0 {
		limit = 20
	}

	likeTerm := "%" + strings.ToLower(term) + "%"

	rows, err := db.conn.Query(`
	        SELECT
	            id,
	            module_id,
	            file_path,
	            block_type,
	            block_labels,
	            attribute_path,
	            reference_type,
	            reference_name,
	            start_byte,
	            end_byte
	        FROM hcl_relationships
	        WHERE
	              LOWER(attribute_path) LIKE ?
	           OR LOWER(reference_name) LIKE ?
	        ORDER BY module_id, file_path, start_byte
	        LIMIT ?
	    `, likeTerm, likeTerm, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []HCLRelationship
	for rows.Next() {
		var rel HCLRelationship
		var blockLabels sql.NullString
		if err := rows.Scan(
			&rel.ID,
			&rel.ModuleID,
			&rel.FilePath,
			&rel.BlockType,
			&blockLabels,
			&rel.AttributePath,
			&rel.ReferenceType,
			&rel.ReferenceName,
			&rel.StartByte,
			&rel.EndByte,
		); err != nil {
			return nil, err
		}
		if blockLabels.Valid {
			rel.BlockLabels = blockLabels.String
		}
		results = append(results, rel)
	}

	return results, rows.Err()
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (db *DB) HCLBlockExists(moduleID int64, filePath, blockType, typePrefix string, attrFilters []string) (bool, error) {
	base := `SELECT 1 FROM hcl_blocks WHERE module_id = ? AND file_path = ?`
	args := []any{moduleID, filePath}
	if blockType != "" {
		base += ` AND block_type = ?`
		args = append(args, blockType)
	}
	if typePrefix != "" {
		base += ` AND type_label LIKE ?`
		args = append(args, typePrefix+"%")
	}
	for range attrFilters {
		base += ` AND instr(IFNULL(attr_paths, ''), ?) > 0`
	}
	for _, f := range attrFilters {
		args = append(args, f)
	}
	base += ` LIMIT 1`
	var one int
	err := db.conn.QueryRow(base, args...).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

type ModuleStructureSummary struct {
	ResourceCount              int
	LifecycleCount             int
	DynamicLabels              []string
	TopResourceTypes           []string
	ResourcesWithIgnoreChanges int
}

func (db *DB) SummarizeModuleStructure(moduleID int64) (*ModuleStructureSummary, error) {
	sum := &ModuleStructureSummary{}

	_ = db.conn.QueryRow(`SELECT COUNT(*) FROM hcl_blocks WHERE module_id = ? AND block_type = 'resource'`, moduleID).Scan(&sum.ResourceCount)
	_ = db.conn.QueryRow(`SELECT COUNT(*) FROM hcl_blocks WHERE module_id = ? AND block_type = 'lifecycle'`, moduleID).Scan(&sum.LifecycleCount)
	_ = db.conn.QueryRow(`SELECT COUNT(*) FROM hcl_blocks WHERE module_id = ? AND block_type = 'resource' AND instr(IFNULL(attr_paths,''), 'lifecycle.ignore_changes') > 0`, moduleID).Scan(&sum.ResourcesWithIgnoreChanges)

	rows, err := db.conn.Query(`
        SELECT type_label, COUNT(*) AS cnt
        FROM hcl_blocks
        WHERE module_id = ? AND block_type = 'resource' AND type_label IS NOT NULL
        GROUP BY type_label
        ORDER BY cnt DESC, type_label ASC
        LIMIT 5
    `, moduleID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var label sql.NullString
			var cnt int
			if err := rows.Scan(&label, &cnt); err == nil && label.Valid {
				sum.TopResourceTypes = append(sum.TopResourceTypes, label.String)
			}
		}
	}

	rows2, err2 := db.conn.Query(`
        SELECT DISTINCT type_label FROM hcl_blocks
        WHERE module_id = ? AND block_type = 'dynamic' AND type_label IS NOT NULL
        ORDER BY type_label
    `, moduleID)
	if err2 == nil {
		defer rows2.Close()
		for rows2.Next() {
			var label sql.NullString
			if err := rows2.Scan(&label); err == nil && label.Valid {
				sum.DynamicLabels = append(sum.DynamicLabels, label.String)
			}
		}
	}

	return sum, nil
}

func (db *DB) GetModuleDynamicLabels(moduleID int64) ([]string, error) {
	rows, err := db.conn.Query(`
        SELECT DISTINCT type_label FROM hcl_blocks WHERE module_id = ? AND block_type = 'dynamic' AND type_label IS NOT NULL
    `, moduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var labels []string
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return nil, err
		}
		labels = append(labels, label)
	}
	return labels, rows.Err()
}

func (db *DB) CountResourceBlocks(moduleID int64) (int, error) {
	var total int
	err := db.conn.QueryRow(`SELECT COUNT(*) FROM hcl_blocks WHERE module_id = ? AND block_type = 'resource'`, moduleID).Scan(&total)
	return total, err
}

func (db *DB) GetModuleResourceTypes(moduleID int64) ([]string, error) {
	rows, err := db.conn.Query(`
        SELECT resource_type FROM module_resources WHERE module_id = ?
    `, moduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var types []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		types = append(types, t)
	}
	return types, rows.Err()
}

func (db *DB) ClearModuleTags(moduleID int64) error {
	_, err := db.conn.Exec(`DELETE FROM module_tags WHERE module_id = ?`, moduleID)
	return err
}

func (db *DB) InsertModuleTag(moduleID int64, tag string, weight int, source string) error {
	_, err := db.conn.Exec(`
        INSERT INTO module_tags (module_id, tag, weight, source)
        VALUES (?, ?, ?, ?)
        ON CONFLICT(module_id, tag) DO UPDATE SET
            weight = excluded.weight,
            source = COALESCE(excluded.source, source)
    `, moduleID, strings.ToLower(tag), weight, source)
	return err
}

func (db *DB) GetModuleTags(moduleID int64) ([]ModuleTag, error) {
	rows, err := db.conn.Query(`
        SELECT id, module_id, tag, weight, source
        FROM module_tags WHERE module_id = ?
        ORDER BY weight DESC, tag ASC
    `, moduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []ModuleTag
	for rows.Next() {
		var t ModuleTag
		if err := rows.Scan(&t.ID, &t.ModuleID, &t.Tag, &t.Weight, &t.Source); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

func (db *DB) ClearModuleAliases(moduleID int64) error {
	_, err := db.conn.Exec(`DELETE FROM module_aliases WHERE module_id = ?`, moduleID)
	return err
}

func (db *DB) InsertModuleAlias(moduleID int64, alias string, weight int, source string) error {
	_, err := db.conn.Exec(`
        INSERT INTO module_aliases (module_id, alias, weight, source)
        VALUES (?, ?, ?, ?)
        ON CONFLICT(module_id, alias) DO UPDATE SET
            weight = excluded.weight,
            source = COALESCE(excluded.source, source)
    `, moduleID, strings.ToLower(alias), weight, source)
	return err
}

func (db *DB) ResolveModuleByAlias(alias string) (*Module, error) {
	var m Module
	err := db.conn.QueryRow(`
        SELECT m.id, m.name, m.full_name, m.description, m.repo_url, m.last_updated, m.synced_at, m.readme_content, m.has_examples
        FROM module_aliases a
        JOIN modules m ON m.id = a.module_id
        WHERE a.alias = ?
        ORDER BY a.weight DESC,
                 (CASE WHEN instr(m.name, '//') > 0 THEN 1 ELSE 0 END) ASC,
                 m.name ASC
        LIMIT 1
    `, strings.ToLower(alias)).Scan(&m.ID, &m.Name, &m.FullName, &m.Description, &m.RepoURL, &m.LastUpdated, &m.SyncedAt, &m.ReadmeContent, &m.HasExamples)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (db *DB) ResolveModuleByAliasPrefix(prefix string) (*Module, error) {
	like := strings.ToLower(prefix) + "%"
	var m Module
	err := db.conn.QueryRow(`
        SELECT m.id, m.name, m.full_name, m.description, m.repo_url, m.last_updated, m.synced_at, m.readme_content, m.has_examples
        FROM module_aliases a
        JOIN modules m ON m.id = a.module_id
        WHERE a.alias LIKE ?
        GROUP BY m.id
        ORDER BY MAX(a.weight) DESC,
                 (CASE WHEN instr(m.name, '//') > 0 THEN 1 ELSE 0 END) ASC,
                 m.name ASC
        LIMIT 1
    `, like).Scan(&m.ID, &m.Name, &m.FullName, &m.Description, &m.RepoURL, &m.LastUpdated, &m.SyncedAt, &m.ReadmeContent, &m.HasExamples)
	if err != nil {
		return nil, err
	}
	return &m, nil
}
