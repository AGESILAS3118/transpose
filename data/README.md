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

## Production data pipeline

This API-backed workflow is only a Phase 0 bootstrap and quality-measurement
tool. Production database releases will be generated in CI from Repology data
dumps, validated and versioned there; users will consume the local database
without querying the public API.
