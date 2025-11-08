# SPEC.md — GitHub PR Analysis Tool

**Goal:** a local, idiomatic Go application that programmatically analyzes **closed pull requests** across a GitHub *organization*, grouping counts by CODEOWNERS teams (and individuals) and by repository, with configurable filters, local caching (no external DB), and respectful rate-limiting.

---

# 1. Overview / requirements

## 1.1 Functional
- Count total number of PRs **closed** in a configurable time window across an org (all repos).
- Count PRs closed in the same window **grouped by CODEOWNERS teams** (a team or individual listed in a repo’s `CODEOWNERS` file).
  - If a CODEOWNERS file lists multiple owners for a path, a PR matching that path will be attributed per the selected attribution mode (see config).
- Count PRs closed in the same window **grouped by repository**.
- Include PRs for repos **without a CODEOWNERS file** under a grouped label: `"no_codeowners"` or `"unknown_codeowners"`.
- Support filters:
  - Exclude PRs opened by specific user(s) (support multiple).
  - Exclude PRs whose title **starts with** one or more configured strings (support multiple).
- Support rolling up multiple Github teams up under a named team defined in the config file
- CLI + configuration file for parameters.
- Output results in machine-friendly formats (JSON, CSV) and human summary.

## 1.2 Non-functional
- Respect GitHub API rate limits; rate-limit behavior and timing must be configurable.
- Implement in idiomatic Go.
- Use GitHub API via official Go SDK(s) (e.g., `github.com/google/go-github`) and `golang.org/x/oauth2` for auth.
- Cache GitHub organization metadata locally — no separate DB server. The cache must be readable/editable, refreshable and support manual invalidation.
- The command must allow only using the cache for analysis and skipping the API calls.
- Concurrency: parallelize work (repo-level) while respecting rate limits.
- Provide unit and integration tests where possible (integration tests must be opt-in and require a token).
- Clear logging and error handling.

---

# 2. High-level architecture

Components:
- CLI (defined in the `cmd` package with the analyze command defined in `cmd/analyze.go`)
- Config loader (YAML/JSON)
- GitHub client wrapper (auth, retries, rate-limit handling)
- Fetcher:
  - Repo enumerator (list org repos)
  - PR fetcher (per-repo, time window, closed PRs)
  - CODEOWNERS fetcher & parser (per-repo)
- Cacher:
  - Local persistent cache (SQLite by default, with optional JSON files)
  - Cache layer for repo metadata, CODEOWNERS contents, PR lists (etag/last-modified optionally)
- Processor/mapper:
  - Map PRs to owners (teams/people) using CODEOWNERS rules
  - Apply filters (author exclusions, title prefix exclusions)
  - Attribution model (configurable)
- Aggregator:
  - Metrics engine (total, by-team, by-repo)
- Exporter:
  - JSON, CSV, human summary, optional CSV per-repo
- Tests and tooling

Execution flow:
1. Load config and CLI flags.
2. Initialize cache and GitHub client (token via env var or config).
3. Enumerate repositories in the organization — load from cache if valid, else from API.
4. For each repo (concurrently, limited by worker pool), ensure CODEOWNERS cached; fetch PRs (closed, date window) with pagination, respecting rate limiting.
5. Parse CODEOWNERS, map PR changed files to owners (pattern matching).
6. Apply exclusions and attribution rules; increment counters.
7. Write aggregated outputs.

---

# 3. Configuration

Support CLI flags and a config file (YAML). CLI flags override config.

### Example `config.yaml`
```yaml
github:
  org: "my-org"
  token_env_var: "GITHUB_TOKEN" 
time_window:
  since: "2025-10-01T00:00:00Z"
  until: "2025-10-31T23:59:59Z"
filters:
  exclude_authors:
    - "bot-user"
    - "do-not-count"
  exclude_title_prefixes:
    - "WIP:"
    - "DO NOT MERGE"
attribution:
  mode: "multi"   # "multi" | "primary" | "first-owner-only"
cache:
  backend: "sqlite"
  sqlite_path: "./cache.db"
  json_dir: "./cache"
  ttl_minutes: 1440
rate_limiter:
  type: "token-bucket"
  qps: 2
  burst: 20
  retry:
    max_attempts: 5
    base_delay_ms: 500
output:
  format: "json"
  output_dir: "./out"
logging:
  level: "info"
concurrency:
  repo_workers: 8
team_rollup:
  - name: my rollup team
    teams:
      - team_1
      - team_2
      - team_3
  - name: my other rollup
    teams:
      - team_3
      - team_4
      - team_5
```

### CLI flags (examples)
- `--config` (path)
- `--org`
- `--since`, `--until`
- `--exclude-author`
- `--exclude-title-prefix`
- `--cache-backend`
- `--output-format`
- `--invalidate-cache`
- `--dry-run`
- `--skip-api-calls`

---

# 4. Cache design (local, no DB server)

Two backends supported (pluggable):
1. **SQLite (recommended)** — single-file durable store, transactional, good for moderate scale.
   - Schema (simple):
     - `repos`, `codeowners`, `prs`, `meta`
2. **Flat JSON per repo**
   - Directory layout under `cache/`
   - Easy to inspect/edit

Cache policies:
- simple, refreshable architecture
- keep track of the freshness, but don't overengineer with TTLs, ETags, etc, a flag to refresh the cache/call the API is good enough
- Manual invalidation flag
- Per-repo granularity

---

# 5. GitHub API usage & rate limiting

- Auth via `GITHUB_TOKEN`
- Official Go client: `github.com/google/go-github`
- Pagination via `ListOptions`
- Rate limiting:
  - Token bucket limiter
  - Backoff on 5xx/429 with jitter
  - Watch headers (`X-RateLimit-Remaining` etc.)
- support configurable backoffs and timers for calling the Github API

---

# 6. CODEOWNERS parsing & mapping

- Parse CODEOWNERS files from repo root, `.github/`
- Match file patterns (gitignore-like)
- Determine specificity (longest match)
- Attribution modes:
  - `multi`
  - `primary`
  - `first-owner-only`
- Handle team (`@org/team`) vs individual (`@user`)
- Label PRs with no owners as `"no_codeowners"`

---

# 7. PR fetching & filters

- Fetch closed PRs by repo within date window
- Filter:
  - Exclude authors
  - Exclude title prefixes
- Fetch changed files (`ListFiles`) to determine owners
- Cache PRs & file lists per repo

---

# 8. Aggregation & output

- Metrics:
  - `total_prs_closed`
  - `prs_by_team`
  - `prs_by_repo`
  - `prs_by_user`
- Formats:
  - JSON
  - CSV
- Example JSON output shown in spec

---

# 9. Concurrency & performance

- Worker pool for repo processing
- Centralized rate limiter
- TTL-based cache reuse
- Optional dry-run

---

# 10. Error handling & retries

- Retries on network & 5xx
- Fail-fast on auth/org errors
- Continue but log on repo-level errors

---

# 11. Testing

- Unit tests for parser, filters, aggregation
- Integration tests (optional, token-required)

---

# 12. Security & credentials

- Use `GITHUB_TOKEN` via env var
- Minimal scopes
- Secure cache file permissions

---

# 13. Example commands

```bash
ghpr-analyzer analyze --config ./config.yaml
ghpr-analyzer analyze  --org my-org --since 2025-10-01T00:00:00Z --until 2025-10-31T23:59:59Z --exclude-author bot --output-format csv
ghpr-analyzer analyze  --config ./config.yaml --invalidate-cache
```

---

# 14. Deliverables

- MVP: repo enumeration, PR fetching, JSON output
- CODEOWNERS parsing
- Filters, attribution modes
- CSV output
- Rate limiting
- Caching
- Testing, docs

---

# 15. Notes

- Default attribution: `multi`
- SQLite default cache backend
- Per-repo PR listing preferred over search API
