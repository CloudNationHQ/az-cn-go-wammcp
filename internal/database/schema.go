package database

const Schema = `
CREATE TABLE IF NOT EXISTS modules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    full_name TEXT NOT NULL,
    description TEXT,
    repo_url TEXT NOT NULL,
    last_updated TEXT,
    synced_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    readme_content TEXT,
    has_examples BOOLEAN DEFAULT 0
);

CREATE TABLE IF NOT EXISTS module_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    module_id INTEGER NOT NULL,
    file_name TEXT NOT NULL,
    file_path TEXT NOT NULL,
    file_type TEXT,
    content TEXT NOT NULL,
    size_bytes INTEGER,
    FOREIGN KEY (module_id) REFERENCES modules(id) ON DELETE CASCADE,
    UNIQUE(module_id, file_path)
);

CREATE TABLE IF NOT EXISTS module_variables (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    module_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    type TEXT,
    description TEXT,
    default_value TEXT,
    required BOOLEAN DEFAULT 1,
    sensitive BOOLEAN DEFAULT 0,
    FOREIGN KEY (module_id) REFERENCES modules(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS module_outputs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    module_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    value TEXT,
    sensitive BOOLEAN DEFAULT 0,
    FOREIGN KEY (module_id) REFERENCES modules(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS module_resources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    module_id INTEGER NOT NULL,
    resource_type TEXT NOT NULL,
    resource_name TEXT NOT NULL,
    provider TEXT,
    source_file TEXT,
    FOREIGN KEY (module_id) REFERENCES modules(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS module_data_sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    module_id INTEGER NOT NULL,
    data_type TEXT NOT NULL,
    data_name TEXT NOT NULL,
    provider TEXT,
    source_file TEXT,
    FOREIGN KEY (module_id) REFERENCES modules(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS module_examples (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    module_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    path TEXT,
    content TEXT,
    FOREIGN KEY (module_id) REFERENCES modules(id) ON DELETE CASCADE
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_modules_name ON modules(name);
CREATE INDEX IF NOT EXISTS idx_modules_full_name ON modules(full_name);
CREATE INDEX IF NOT EXISTS idx_module_files_module_id ON module_files(module_id);
CREATE INDEX IF NOT EXISTS idx_module_files_type ON module_files(file_type);
CREATE INDEX IF NOT EXISTS idx_module_variables_module_id ON module_variables(module_id);
CREATE INDEX IF NOT EXISTS idx_module_outputs_module_id ON module_outputs(module_id);
CREATE INDEX IF NOT EXISTS idx_module_resources_module_id ON module_resources(module_id);
CREATE INDEX IF NOT EXISTS idx_module_resources_type ON module_resources(resource_type);
CREATE INDEX IF NOT EXISTS idx_module_data_sources_module_id ON module_data_sources(module_id);
CREATE INDEX IF NOT EXISTS idx_module_examples_module_id ON module_examples(module_id);

-- HCL block index for fast AST-based queries
CREATE TABLE IF NOT EXISTS hcl_blocks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    module_id INTEGER NOT NULL,
    file_path TEXT NOT NULL,
    block_type TEXT NOT NULL, -- resource|dynamic|lifecycle
    type_label TEXT,          -- e.g., azurerm_storage_account for resource, or label for dynamic
    start_byte INTEGER NOT NULL,
    end_byte INTEGER NOT NULL,
    attr_paths TEXT,          -- newline-separated flattened attribute paths within this block (e.g., "for_each\nlifecycle.ignore_changes")
    FOREIGN KEY (module_id) REFERENCES modules(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_hcl_blocks_module ON hcl_blocks(module_id);
CREATE INDEX IF NOT EXISTS idx_hcl_blocks_type ON hcl_blocks(block_type);
CREATE INDEX IF NOT EXISTS idx_hcl_blocks_label ON hcl_blocks(type_label);

CREATE TABLE IF NOT EXISTS hcl_relationships (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    module_id INTEGER NOT NULL,
    file_path TEXT NOT NULL,
    block_type TEXT NOT NULL,
    block_labels TEXT,
    attribute_path TEXT NOT NULL,
    reference_type TEXT NOT NULL,
    reference_name TEXT NOT NULL,
    start_byte INTEGER NOT NULL,
    end_byte INTEGER NOT NULL,
    FOREIGN KEY (module_id) REFERENCES modules(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_hcl_relationships_module ON hcl_relationships(module_id);
CREATE INDEX IF NOT EXISTS idx_hcl_relationships_ref ON hcl_relationships(module_id, reference_name);
CREATE INDEX IF NOT EXISTS idx_hcl_relationships_attr ON hcl_relationships(module_id, attribute_path);
CREATE INDEX IF NOT EXISTS idx_hcl_relationships_ref_only ON hcl_relationships(reference_name);
CREATE INDEX IF NOT EXISTS idx_hcl_relationships_attr_only ON hcl_relationships(attribute_path);

CREATE VIRTUAL TABLE IF NOT EXISTS modules_fts USING fts5(
    name,
    description,
    readme_content,
    content='modules',
    content_rowid='id'
);

CREATE VIRTUAL TABLE IF NOT EXISTS files_fts USING fts5(
    file_name,
    file_path,
    content,
    content='module_files',
    content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS modules_fts_insert AFTER INSERT ON modules BEGIN
    INSERT INTO modules_fts(rowid, name, description, readme_content)
    VALUES (new.id, new.name, new.description, new.readme_content);
END;

CREATE TRIGGER IF NOT EXISTS modules_fts_update AFTER UPDATE ON modules BEGIN
    UPDATE modules_fts
    SET name = new.name,
        description = new.description,
        readme_content = new.readme_content
    WHERE rowid = new.id;
END;

CREATE TRIGGER IF NOT EXISTS modules_fts_delete AFTER DELETE ON modules BEGIN
    DELETE FROM modules_fts WHERE rowid = old.id;
END;

-- Triggers to keep files FTS in sync
CREATE TRIGGER IF NOT EXISTS files_fts_insert AFTER INSERT ON module_files BEGIN
    INSERT INTO files_fts(rowid, file_name, file_path, content)
    VALUES (new.id, new.file_name, new.file_path, new.content);
END;

CREATE TRIGGER IF NOT EXISTS files_fts_update AFTER UPDATE ON module_files BEGIN
    UPDATE files_fts
    SET file_name = new.file_name,
        file_path = new.file_path,
        content = new.content
    WHERE rowid = new.id;
END;

CREATE TRIGGER IF NOT EXISTS files_fts_delete AFTER DELETE ON module_files BEGIN
    DELETE FROM files_fts WHERE rowid = old.id;
END;

-- Auto-generated and user-defined aliases for modules
CREATE TABLE IF NOT EXISTS module_aliases (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    module_id INTEGER NOT NULL,
    alias TEXT NOT NULL,
    weight INTEGER DEFAULT 1,
    source TEXT,
    FOREIGN KEY (module_id) REFERENCES modules(id) ON DELETE CASCADE,
    UNIQUE(module_id, alias)
);

CREATE INDEX IF NOT EXISTS idx_alias_alias ON module_aliases(alias);
CREATE INDEX IF NOT EXISTS idx_alias_module_id ON module_aliases(module_id);

CREATE TABLE IF NOT EXISTS module_tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    module_id INTEGER NOT NULL,
    tag TEXT NOT NULL,
    weight INTEGER DEFAULT 1,
    source TEXT,
    FOREIGN KEY (module_id) REFERENCES modules(id) ON DELETE CASCADE,
    UNIQUE(module_id, tag)
);

CREATE INDEX IF NOT EXISTS idx_module_tags_module_id ON module_tags(module_id);
CREATE INDEX IF NOT EXISTS idx_module_tags_tag ON module_tags(tag);

CREATE TABLE IF NOT EXISTS module_releases (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    module_id INTEGER NOT NULL,
    version TEXT NOT NULL,
    tag TEXT NOT NULL,
    previous_version TEXT,
    previous_tag TEXT,
    commit_sha TEXT,
    previous_commit_sha TEXT,
    release_date TEXT,
    comparison_url TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (module_id) REFERENCES modules(id) ON DELETE CASCADE,
    UNIQUE(module_id, version)
);

CREATE INDEX IF NOT EXISTS idx_module_releases_module ON module_releases(module_id);
CREATE INDEX IF NOT EXISTS idx_module_releases_tag ON module_releases(tag);

CREATE TABLE IF NOT EXISTS module_release_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    release_id INTEGER NOT NULL,
    section TEXT NOT NULL,
    entry_key TEXT NOT NULL,
    title TEXT NOT NULL,
    details TEXT,
    identifier TEXT,
    change_type TEXT,
    order_index INTEGER DEFAULT 0,
    FOREIGN KEY (release_id) REFERENCES module_releases(id) ON DELETE CASCADE,
    UNIQUE(release_id, entry_key)
);

CREATE INDEX IF NOT EXISTS idx_module_release_entries_release ON module_release_entries(release_id);
CREATE INDEX IF NOT EXISTS idx_module_release_entries_identifier ON module_release_entries(identifier);
`
