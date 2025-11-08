# GitHub PR Analysis Tool

A local, idiomatic Go application that programmatically analyzes **closed pull requests** across a GitHub organization, grouping counts by CODEOWNERS teams (and individuals) and by repository, with configurable filters, local caching, and respectful rate-limiting.

## Features

- ✅ **Repository Enumeration**: Lists all repositories in a GitHub organization
- ✅ **PR Analysis**: Analyzes closed PRs within a configurable time window
- ✅ **CODEOWNERS Support**: Parses CODEOWNERS files and attributes PRs to teams
- ✅ **Team Rollup**: Roll up multiple GitHub teams under named rollup teams
- ✅ **Filtering**: Exclude PRs by author or title prefix
- ✅ **Rate Limiting**: Respectful GitHub API rate limiting with token bucket algorithm
- ✅ **Concurrent Processing**: Parallel repository processing with configurable worker pool
- ✅ **JSON Output**: Machine-friendly JSON output with aggregated metrics
- ✅ **Logging**: Configurable logging levels (debug, info, warn, error)

## Installation

### Prerequisites

- Go 1.25.3 or later
- A GitHub personal access token (PAT) with appropriate scopes

### Build from Source

```bash
git clone <repository-url>
cd <repository-directory>
go build -o analyzer ./main.go
```

## GitHub Token Setup

### Creating a Personal Access Token (PAT)

1. **Navigate to GitHub Settings**
   - Go to [GitHub Settings](https://github.com/settings/profile)
   - Click on **Developer settings** in the left sidebar
   - Click on **Personal access tokens** → **Tokens (classic)**

2. **Generate New Token**
   - Click **Generate new token** → **Generate new token (classic)**
   - Give your token a descriptive name (e.g., "PR Analyzer")
   - Set an expiration date (recommended: 90 days or custom)

3. **Select Required Scopes**

   The following scopes are required for the application to function:

   - ✅ **`repo`** (Full control of private repositories)
     - Required to read repository metadata, pull requests, and CODEOWNERS files
     - Includes access to:
       - `repo:status` - Read/write repository status
       - `repo_deployment` - Access deployment status
       - `public_repo` - Access public repositories
       - `repo:invite` - Access repository invitations
       - `security_events` - Read and write security events

   **Note**: For public repositories only, you can use a more limited scope:
   - ✅ **`public_repo`** - Access public repositories (read-only access to public repos)

   **For organizations**, you may also need:
   - ✅ **`read:org`** - Read org and team membership, read org projects (if analyzing private org repos)

4. **Generate and Copy Token**
   - Click **Generate token** at the bottom
   - **IMPORTANT**: Copy the token immediately - you won't be able to see it again!
   - Store it securely (use a password manager)

### Setting the Token

Set the token as an environment variable:

```bash
# Linux/macOS
export GITHUB_TOKEN="your_token_here"

# Windows (PowerShell)
$env:GITHUB_TOKEN="your_token_here"

# Windows (CMD)
set GITHUB_TOKEN=your_token_here
```

Alternatively, you can specify a custom environment variable name in your config file:

```yaml
github:
  token_env_var: "MY_CUSTOM_TOKEN_VAR"
```

Then set that variable:
```bash
export MY_CUSTOM_TOKEN_VAR="your_token_here"
```

### Token Security Best Practices

- 🔒 **Never commit tokens to version control**
- 🔒 **Use environment variables or secret management tools**
- 🔒 **Rotate tokens regularly**
- 🔒 **Use the minimum required scopes**
- 🔒 **Set appropriate expiration dates**
- 🔒 **Revoke tokens when no longer needed**

## Configuration

The application supports configuration via YAML file and CLI flags. CLI flags override config file values.

### Configuration File

Create a `config.yaml` file (see `config.yaml.example` for a template):

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
  threshold: 3000      # Sleep when rate limit remaining reaches this threshold (0 = disabled)
  sleep_minutes: 60    # Minutes to sleep when threshold is reached
output:
  format: "json"
  output_dir: "./out"
logging:
  level: "info"
concurrency:
  repo_workers: 8
```

### Configuration Options

| Section | Option | Description | Default |
|---------|--------|-------------|---------|
| `github` | `org` | GitHub organization name | Required |
| `github` | `token_env_var` | Environment variable name for token | `GITHUB_TOKEN` |
| `time_window` | `since` | Start time (RFC3339 format) | Required |
| `time_window` | `until` | End time (RFC3339 format) | Required |
| `filters` | `exclude_authors` | List of author usernames to exclude | `[]` |
| `filters` | `exclude_title_prefixes` | List of title prefixes to exclude | `[]` |
| `attribution` | `mode` | Attribution mode | `multi` |
| `team_rollup` | - | List of team rollup configurations | `[]` |
| `team_rollup[].name` | - | Name of the rollup team | Required |
| `team_rollup[].teams` | - | List of team names to roll up | Required |
| `rate_limiter` | `qps` | Queries per second | `2` |
| `rate_limiter` | `burst` | Burst size | `20` |
| `rate_limiter` | `threshold` | Rate limit threshold to trigger sleep (0 = disabled) | `0` |
| `rate_limiter` | `sleep_minutes` | Minutes to sleep when threshold is reached | `60` |
| `output` | `format` | Output format (`json`, `csv`) | `json` |
| `output` | `output_dir` | Output directory | `./out` |
| `logging` | `level` | Log level (`debug`, `info`, `warn`, `error`) | `info` |
| `concurrency` | `repo_workers` | Number of concurrent workers | `8` |

## Usage

### Basic Usage

```bash
# Using config file
./analyzer analyze --config config.yaml

# Using CLI flags
./analyzer analyze \
  --org my-org \
  --since 2025-10-01T00:00:00Z \
  --until 2025-10-31T23:59:59Z
```

### With Filters

```bash
./analyzer analyze \
  --org my-org \
  --since 2025-10-01T00:00:00Z \
  --until 2025-10-31T23:59:59Z \
  --exclude-author bot \
  --exclude-author automated \
  --exclude-title-prefix "WIP:" \
  --exclude-title-prefix "DO NOT MERGE"
```

### Output Format

```bash
./analyzer analyze \
  --org my-org \
  --output-format json \
  --output-dir ./results
```

### Dry Run

```bash
./analyzer analyze --org my-org --dry-run
```

### CLI Flags

| Flag | Description | Example |
|------|-------------|---------|
| `--config` | Path to config file | `--config config.yaml` |
| `--org` | GitHub organization name | `--org my-org` |
| `--since` | Start time (RFC3339) | `--since 2025-10-01T00:00:00Z` |
| `--until` | End time (RFC3339) | `--until 2025-10-31T23:59:59Z` |
| `--exclude-author` | Exclude PRs by author (repeatable) | `--exclude-author bot` |
| `--exclude-title-prefix` | Exclude PRs by title prefix (repeatable) | `--exclude-title-prefix "WIP:"` |
| `--output-format` | Output format (`json`, `csv`) | `--output-format json` |
| `--output-dir` | Output directory | `--output-dir ./out` |
| `--dry-run` | Dry run mode (no API calls) | `--dry-run` |
| `--skip-api-calls` | Use cache only (future feature) | `--skip-api-calls` |
| `--invalidate-cache` | Invalidate cache (future feature) | `--invalidate-cache` |
| `--log-level` | Log level (`debug`, `info`, `warn`, `error`) | `--log-level debug` |

## Team Rollup

The application supports rolling up multiple GitHub teams under named rollup teams. This is useful for aggregating statistics across related teams.

### Configuration

Configure team rollups in your `config.yaml`:

```yaml
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

### How It Works

- When a PR is attributed to a team that is part of a rollup (e.g., `team_1`), it is counted **only** under the rollup team name (e.g., `my rollup team`)
- Teams that are **not** part of any rollup are counted under their individual team name
- A team can be part of multiple rollups (counted under all rollup teams it belongs to)
- **Each PR is counted only once per rollup team**, even if multiple teams within that rollup are attributed to the PR
- Team names are normalized (the `@` prefix is removed if present)
- Rollup team names appear in the `prs_by_team` output instead of individual team names for teams in rollups

### Example

If a PR is attributed to `team_1`, `team_2`, and `team_3`, and you have a rollup configured as above:
- All three teams are in the rollup, so the PR is counted **once** under `my rollup team` (not three times)
- The PR is **not** counted under `team_1`, `team_2`, or `team_3` individually
- If the PR is also attributed to `team_6` (not in any rollup), it is counted under `team_6`
- This provides clean aggregated statistics without double-counting

## Output

The application generates two JSON files in the output directory:

### `analysis_results.json`

Aggregated metrics:

```json
{
  "total_prs_closed": 150,
  "prs_by_repo": {
    "my-org/repo1": 45,
    "my-org/repo2": 30,
    "my-org/repo3": 75
  },
  "prs_by_team": {
    "no_codeowners": 150
  },
  "prs_by_user": {
    "alice": 50,
    "bob": 40,
    "charlie": 60
  },
  "time_window": {
    "since": "2025-10-01T00:00:00Z",
    "until": "2025-10-31T23:59:59Z"
  },
  "generated_at": "2025-11-01T10:00:00Z"
}
```

### `prs_by_repo.json`

Detailed PR information grouped by repository:

```json
{
  "my-org/repo1": [
    {
      "number": 123,
      "title": "Add feature X",
      "author": "alice",
      "state": "closed",
      "created_at": "2025-10-15T10:00:00Z",
      "closed_at": "2025-10-16T14:30:00Z",
      "url": "https://github.com/my-org/repo1/pull/123"
    }
  ]
}
```

## Examples

### Analyze Last Month's PRs

```bash
./analyzer analyze \
  --org my-org \
  --since $(date -u -d '1 month ago' +%Y-%m-%dT00:00:00Z) \
  --until $(date -u +%Y-%m-%dT23:59:59Z)
```

### Exclude Bot PRs

```bash
./analyzer analyze \
  --org my-org \
  --since 2025-10-01T00:00:00Z \
  --until 2025-10-31T23:59:59Z \
  --exclude-author dependabot \
  --exclude-author renovate \
  --exclude-author github-actions
```

### Debug Mode

```bash
./analyzer analyze \
  --org my-org \
  --since 2025-10-01T00:00:00Z \
  --until 2025-10-31T23:59:59Z \
  --log-level debug
```

## Troubleshooting

### Token Issues

**Error**: `GitHub token not found in environment variable GITHUB_TOKEN`

**Solution**: Ensure the token is set in your environment:
```bash
export GITHUB_TOKEN="your_token_here"
```

**Error**: `401 Unauthorized`

**Solution**: 
- Verify your token is valid and not expired
- Ensure you have the correct scopes (`repo` for private repos, `public_repo` for public repos)
- Check that you have access to the organization

### Rate Limiting

**Error**: `403 rate limit exceeded`

**Solution**: 
- The application automatically handles rate limiting with exponential backoff
- Reduce `qps` (queries per second) in your config
- Configure `threshold` and `sleep_minutes` to proactively sleep when rate limit is low
- Wait for the rate limit to reset (usually 1 hour)

**Proactive Rate Limit Management**:
You can configure the application to automatically sleep when the rate limit remaining reaches a threshold. For example:
```yaml
rate_limiter:
  threshold: 3000      # Sleep when remaining requests <= 3000
  sleep_minutes: 60    # Sleep for 60 minutes
```

This helps prevent hitting the rate limit by pausing operations when the remaining requests are low.

### Organization Access

**Error**: `404 Not Found` when listing repositories

**Solution**:
- Verify you have access to the organization
- Ensure your token has `read:org` scope for private organizations
- Check that the organization name is correct

### Time Format

**Error**: `invalid time_window.since format (must be RFC3339)`

**Solution**: Use RFC3339 format:
```bash
--since 2025-10-01T00:00:00Z
--until 2025-10-31T23:59:59Z
```

## Development

### Running Tests

```bash
go test ./...
```

### Building

```bash
go build -o analyzer ./main.go
```

### Linting

```bash
golangci-lint run
```

## Roadmap

- [ ] CODEOWNERS parsing and mapping
- [ ] Team attribution modes (multi, primary, first-owner-only)
- [ ] CSV output format
- [ ] SQLite/JSON caching layer
- [ ] Cache-only mode
- [ ] Cache invalidation
- [ ] Per-repo CSV export

## License

See [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

