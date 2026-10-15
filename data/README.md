# Repology bootstrap database

Phase 0.3 bootstraps a local SQLite database from a captured manifest and the
public Repology API. Requests are paced at one per second and cached for seven
days under `~/.cache/xpose/repology/`.

```sh
go build -o transpose .
./transpose update-db --from-manifest /tmp/manifest-fedora.yaml --target debian_13
./transpose resolve --from-manifest /tmp/manifest-fedora.yaml \
  --target debian_13 --sample 200 --output /tmp/sample-200.csv
```

For a smaller initial run use `--limit 50`; cached responses can be reused
without network access with `update-db --offline`. The database is stored at
`~/.local/share/xpose/repology.db`.

## Local database schema

- `projects(name PRIMARY KEY, upstream_url, last_seen)` stores the Repology
  project and its upstream URL.
- `packages(id, project_name, repo, name, version, category, maintainer)`
  stores target repository candidates. `(project_name, repo, name, version)`
  is unique.
- Indexes cover `(repo, name)` and `(project_name, repo)`.

Repology data is licensed under **CC-BY-SA 4.0**. Attribution is required for
redistribution of this database or a derived database.

## Repology dumps

The official `dumps.repology.org` index currently publishes complete PostgreSQL
SQL dumps, not a filtered `debian_13` export. The latest dump observed for this
prototype was about 2.46 GB compressed; Repology documents more than 10 GB of
database space after restoration and requires the matching PostgreSQL major
version plus Repology-specific extensions. It is not a directly importable
filtered input for this local SQLite prototype. `update-db --dump` and
`--from-url` accept filtered JSON Lines records with `project_name`, `repo`,
`name`, `version`, and optional upstream metadata; `--from-url` resumes with
`curl -C -`.

## Production data pipeline

The API-backed workflow is only a Phase 0 bootstrap and quality-measurement
tool. In production, CI should provision the PostgreSQL version and extensions
documented by Repology, import the full upstream dump, export the target
repository joined to its Repology `project_name`, then build, validate,
attribute (CC-BY-SA 4.0), and publish the versioned local SQLite database.
Users will consume that database without querying the public API.

## P0.3 limitations observed

Fedora package names are not consistently Repology project names. Subpackages
such as `NetworkManager-adsl`, `foo-devel`, and `python3-foo` need
normalization before lookup. Fedora-specific firmware, release/configuration
packages, and versioned desktop components may have no Debian 13 project. API
response records also use `binname` / `visiblename`, not always `name`;
importing only the latter caused false negatives.

For Phase 1, enable exact project lookup followed by tested subpackage
normalization aliases, and only then a low-confidence fuzzy search that yields
ambiguous suggestions rather than silently claiming an equivalent package.
Graphical applications should later gain AppStream project/upstream mappings.
