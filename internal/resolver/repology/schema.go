package repology

const schemaSQL = `
CREATE TABLE IF NOT EXISTS projects (
    name TEXT PRIMARY KEY,
    upstream_url TEXT,
    last_seen TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS packages (
    id INTEGER PRIMARY KEY,
    project_name TEXT NOT NULL REFERENCES projects(name) ON DELETE CASCADE,
    repo TEXT NOT NULL,
    name TEXT NOT NULL CHECK(name <> ''),
    version TEXT NOT NULL,
    category TEXT NOT NULL,
    maintainer TEXT NOT NULL,
    UNIQUE(project_name, repo, name, version)
);
CREATE INDEX IF NOT EXISTS idx_packages_repo_name ON packages(repo, name);
CREATE INDEX IF NOT EXISTS idx_packages_project_repo ON packages(project_name, repo);
DELETE FROM packages WHERE name = '';
`
