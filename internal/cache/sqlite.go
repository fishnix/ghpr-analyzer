package cache

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/go-github/v62/github"
	"go.uber.org/zap"
	_ "modernc.org/sqlite"
)

const (
	// Current schema version
	schemaVersion = 2
)

// SQLiteCache implements cache using SQLite
type SQLiteCache struct {
	db        *sql.DB
	logger    *zap.Logger
	ttl       time.Duration
	ignoreTTL bool
}

// NewSQLiteCache creates a new SQLite cache
func NewSQLiteCache(dbPath string, ttl time.Duration, ignoreTTL bool, logger *zap.Logger) (*SQLiteCache, error) {
	// Set SQLite connection parameters to handle busy database
	db, err := sql.Open("sqlite", dbPath+"?_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(1) // SQLite doesn't handle concurrent writes well
	db.SetMaxIdleConns(1)

	cache := &SQLiteCache{
		db:        db,
		logger:    logger,
		ttl:       ttl,
		ignoreTTL: ignoreTTL,
	}

	// Initialize schema
	if err := cache.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Run migration if needed
	if err := cache.migrateSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate schema: %w", err)
	}

	return cache, nil
}

// initSchema initializes the database schema
func (c *SQLiteCache) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS cache_schema (
		version INTEGER NOT NULL,
		created_at DATETIME NOT NULL,
		PRIMARY KEY (version)
	);
	
	CREATE TABLE IF NOT EXISTS repos (
		org TEXT NOT NULL,
		data BLOB NOT NULL,
		timestamp DATETIME NOT NULL,
		PRIMARY KEY (org)
	);
	
	CREATE TABLE IF NOT EXISTS codeowners (
		owner TEXT NOT NULL,
		repo TEXT NOT NULL,
		data BLOB NOT NULL,
		timestamp DATETIME NOT NULL,
		PRIMARY KEY (owner, repo)
	);
	
	CREATE TABLE IF NOT EXISTS prs (
		owner TEXT NOT NULL,
		repo TEXT NOT NULL,
		pr_number INTEGER NOT NULL,
		data BLOB NOT NULL,
		created_at DATETIME,
		closed_at DATETIME,
		timestamp DATETIME NOT NULL,
		PRIMARY KEY (owner, repo, pr_number)
	);
	
	CREATE TABLE IF NOT EXISTS pr_files (
		owner TEXT NOT NULL,
		repo TEXT NOT NULL,
		pr_number INTEGER NOT NULL,
		data BLOB NOT NULL,
		timestamp DATETIME NOT NULL,
		PRIMARY KEY (owner, repo, pr_number)
	);
	`

	_, err := c.db.Exec(schema)
	if err != nil {
		return err
	}

	// Set schema version if not already set
	var existingVersion int
	err = c.db.QueryRow(`SELECT version FROM cache_schema LIMIT 1`).Scan(&existingVersion)
	if err == sql.ErrNoRows {
		// No schema version set - check if this is an old database
		var prsTableExists bool
		err = c.db.QueryRow(`
			SELECT COUNT(*) > 0 
			FROM sqlite_master 
			WHERE type='table' 
			AND name='prs'
		`).Scan(&prsTableExists)
		if err != nil {
			return fmt.Errorf("failed to check prs table: %w", err)
		}

		var initialVersion int
		if prsTableExists {
			// Old database exists - check if it has old schema
			rows, err := c.db.Query(`SELECT since, until FROM prs LIMIT 1`)
			if err == nil {
				// Old schema detected (has since/until columns)
				rows.Close() // Close the query immediately
				initialVersion = 1
				c.logger.Info("Detected old cache schema, will migrate", zap.Int("version", initialVersion))
			} else {
				// New schema already exists or table is empty
				initialVersion = schemaVersion
				c.logger.Info("Detected new cache schema", zap.Int("version", initialVersion))
			}
		} else {
			// New database, use current version
			initialVersion = schemaVersion
			c.logger.Info("Initializing new cache schema", zap.Int("version", initialVersion))
		}

		// Insert initial version
		_, err = c.db.Exec(`
			INSERT INTO cache_schema (version, created_at) 
			VALUES (?, ?)
		`, initialVersion, time.Now())
		if err != nil {
			return fmt.Errorf("failed to set schema version: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to check schema version: %w", err)
	}

	return nil
}

// migrateSchema migrates the database schema from old format to new format
func (c *SQLiteCache) migrateSchema() error {
	c.logger.Debug("Starting schema migration check")

	// Get current schema version (should be set by initSchema)
	var currentVersion int
	err := c.db.QueryRow(`SELECT version FROM cache_schema LIMIT 1`).Scan(&currentVersion)
	if err == sql.ErrNoRows {
		// This shouldn't happen if initSchema ran correctly, but handle it
		c.logger.Warn("No schema version found, assuming version 1")
		currentVersion = 1
		_, err = c.db.Exec(`
			INSERT INTO cache_schema (version, created_at) VALUES (1, ?)
		`, time.Now())
		if err != nil {
			return fmt.Errorf("failed to set schema version: %w", err)
		}
		c.logger.Debug("Inserted schema version 1", zap.Int("version", currentVersion))
	} else if err != nil {
		return fmt.Errorf("failed to get schema version: %w", err)
	}

	c.logger.Debug("Current cache schema version",
		zap.Int("version", currentVersion),
		zap.Int("target_version", schemaVersion))

	// If already at current version, no migration needed
	if currentVersion >= schemaVersion {
		c.logger.Debug("Schema is already at current version, no migration needed")
		return nil
	}

	c.logger.Debug("Migration needed",
		zap.Int("from_version", currentVersion),
		zap.Int("to_version", schemaVersion))

	// Migrate from version 1 to version 2
	if currentVersion == 1 {
		c.logger.Debug("Running migration from version 1 to version 2")
		return c.migrateFromV1ToV2()
	}

	// Future migrations can be added here
	c.logger.Debug("No migration path found for version", zap.Int("version", currentVersion))
	return nil
}

// migrateFromV1ToV2 migrates from schema version 1 to version 2
func (c *SQLiteCache) migrateFromV1ToV2() error {
	c.logger.Debug("Checking if prs table exists")

	// Check if prs table exists with old schema
	var tableExists bool
	err := c.db.QueryRow(`
		SELECT COUNT(*) > 0 
		FROM sqlite_master 
		WHERE type='table' 
		AND name='prs'
	`).Scan(&tableExists)
	if err != nil {
		c.logger.Debug("Error checking for prs table", zap.Error(err))
		return fmt.Errorf("failed to check prs table: %w", err)
	}

	c.logger.Debug("PRs table check result", zap.Bool("exists", tableExists))

	if !tableExists {
		// Table doesn't exist yet, just update schema version
		c.logger.Debug("PRs table doesn't exist, updating schema version only")
		_, err = c.db.Exec(`
			UPDATE cache_schema 
			SET version = ?, created_at = ?
			WHERE version = 1
		`, schemaVersion, time.Now())
		if err != nil {
			return fmt.Errorf("failed to update schema version: %w", err)
		}
		c.logger.Info("Updated cache schema version", zap.Int("version", schemaVersion))
		return nil
	}

	// Check if old schema exists by trying to query the old columns
	c.logger.Debug("Checking for old schema columns (since, until)")
	checkRows, err := c.db.Query(`SELECT since, until FROM prs LIMIT 1`)
	if err != nil {
		// Old columns don't exist, already migrated, just update version
		c.logger.Debug("Old schema columns not found, table already migrated")
		checkRows.Close()
		_, err = c.db.Exec(`
			UPDATE cache_schema 
			SET version = ?, created_at = ?
			WHERE version = 1
		`, schemaVersion, time.Now())
		if err != nil {
			return fmt.Errorf("failed to update schema version: %w", err)
		}
		c.logger.Info("Updated cache schema version", zap.Int("version", schemaVersion))
		return nil
	}
	checkRows.Close()
	c.logger.Debug("Old schema columns found, proceeding with migration")

	c.logger.Info("Migrating PR cache schema from old format to new format")
	c.logger.Debug("Creating new prs_new table with updated schema")

	// Create new table with new schema
	_, err = c.db.Exec(`
		CREATE TABLE IF NOT EXISTS prs_new (
			owner TEXT NOT NULL,
			repo TEXT NOT NULL,
			pr_number INTEGER NOT NULL,
			data BLOB NOT NULL,
			created_at DATETIME,
			closed_at DATETIME,
			timestamp DATETIME NOT NULL,
			PRIMARY KEY (owner, repo, pr_number)
		)
	`)
	if err != nil {
		c.logger.Debug("Failed to create prs_new table", zap.Error(err))
		return fmt.Errorf("failed to create new prs table: %w", err)
	}
	c.logger.Debug("Successfully created prs_new table")

	// Count total rows to migrate for progress tracking
	c.logger.Debug("Counting total rows to migrate")
	var totalRows int
	err = c.db.QueryRow(`SELECT COUNT(*) FROM prs`).Scan(&totalRows)
	if err != nil {
		c.logger.Warn("Failed to count rows for migration", zap.Error(err))
		totalRows = 0
	}
	c.logger.Info("Starting migration", zap.Int("total_rows", totalRows))
	c.logger.Debug("Querying old prs table for migration")

	// Migrate data from old format to new format
	// Use a separate connection/transaction to read from old table
	// to avoid locking issues
	c.logger.Debug("Querying old prs table for migration")
	rows, err := c.db.Query(`
		SELECT owner, repo, since, until, data, timestamp 
		FROM prs
	`)
	if err != nil {
		// No old data to migrate
		c.logger.Debug("Query failed, no old data to migrate", zap.Error(err))
		c.logger.Info("No old PR data to migrate")
		return nil
	}
	defer rows.Close()
	c.logger.Debug("Successfully queried old prs table, starting row processing")

	// Read all rows into memory first to avoid holding a read lock
	// while writing to the new table
	type oldPRRow struct {
		owner     string
		repo      string
		since     string
		until     string
		data      []byte
		timestamp time.Time
	}

	var allRows []oldPRRow
	c.logger.Debug("Reading all rows into memory")
	for rows.Next() {
		var row oldPRRow
		if err := rows.Scan(&row.owner, &row.repo, &row.since, &row.until, &row.data, &row.timestamp); err != nil {
			c.logger.Debug("Failed to scan row", zap.Error(err))
			c.logger.Warn("Failed to scan old PR data", zap.Error(err))
			continue
		}
		allRows = append(allRows, row)
	}
	rows.Close() // Close immediately after reading
	c.logger.Debug("Finished reading rows into memory", zap.Int("total_rows", len(allRows)))

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error reading rows: %w", err)
	}

	migratedCount := 0
	processedRows := 0
	batchSize := 1000 // Commit every 1000 PRs
	batchCount := 0

	var tx *sql.Tx
	ctx := context.Background()

	for _, row := range allRows {
		processedRows++
		c.logger.Debug("Processing row", zap.Int("row_number", processedRows))

		owner := row.owner
		repo := row.repo
		sinceStr := row.since
		untilStr := row.until
		data := row.data
		timestamp := row.timestamp

		c.logger.Debug("Processing row data",
			zap.String("owner", owner),
			zap.String("repo", repo),
			zap.String("since", sinceStr),
			zap.String("until", untilStr),
			zap.Int("data_size", len(data)))

		// Parse time window
		since, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			c.logger.Debug("Failed to parse since time", zap.String("since", sinceStr), zap.Error(err))
			c.logger.Warn("Failed to parse since time", zap.Error(err))
			continue
		}
		until, err := time.Parse(time.RFC3339, untilStr)
		if err != nil {
			c.logger.Debug("Failed to parse until time", zap.String("until", untilStr), zap.Error(err))
			c.logger.Warn("Failed to parse until time", zap.Error(err))
			continue
		}
		c.logger.Debug("Parsed time window",
			zap.Time("since", since),
			zap.Time("until", until))

		// Unmarshal PRs
		var prs []*github.PullRequest
		if err := json.Unmarshal(data, &prs); err != nil {
			c.logger.Debug("Failed to unmarshal PR data", zap.Int("data_size", len(data)), zap.Error(err))
			c.logger.Warn("Failed to unmarshal PR data", zap.Error(err))
			continue
		}
		c.logger.Debug("Unmarshaled PRs",
			zap.Int("pr_count", len(prs)),
			zap.String("owner", owner),
			zap.String("repo", repo))

		// Start a new transaction if needed
		if tx == nil {
			c.logger.Debug("Starting new transaction")
			startTime := time.Now()
			// Use BeginTx with context and immediate transaction mode
			// Immediate mode acquires a write lock immediately, avoiding deadlocks
			tx, err = c.db.BeginTx(ctx, &sql.TxOptions{
				Isolation: sql.LevelDefault,
			})
			if err != nil {
				c.logger.Debug("Failed to begin transaction", zap.Error(err))
				return fmt.Errorf("failed to begin transaction: %w", err)
			}
			elapsed := time.Since(startTime)
			c.logger.Debug("Transaction started",
				zap.Duration("elapsed", elapsed),
				zap.Int("prs_to_process", len(prs)))
		}

		// Insert each PR individually
		prsInWindow := 0
		prsSkipped := 0
		c.logger.Debug("Starting to process PRs from row",
			zap.Int("total_prs", len(prs)),
			zap.String("owner", owner),
			zap.String("repo", repo))

		for i, pr := range prs {
			if pr.Number == nil {
				c.logger.Debug("Skipping PR with nil number")
				prsSkipped++
				continue
			}

			prData, err := json.Marshal(pr)
			if err != nil {
				c.logger.Debug("Failed to marshal PR", zap.Int("pr_number", *pr.Number), zap.Error(err))
				c.logger.Warn("Failed to marshal PR", zap.Error(err))
				prsSkipped++
				continue
			}

			var createdAt, closedAt *time.Time
			if pr.CreatedAt != nil {
				createdAt = &pr.CreatedAt.Time
			}
			if pr.ClosedAt != nil {
				closedAt = &pr.ClosedAt.Time
			}

			// Only migrate PRs that are within the original time window
			if closedAt != nil && !closedAt.Before(since) && !closedAt.After(until) {
				// Ensure we have a transaction
				if tx == nil {
					c.logger.Debug("Starting new transaction for PR")
					tx, err = c.db.BeginTx(ctx, &sql.TxOptions{
						Isolation: sql.LevelDefault,
					})
					if err != nil {
						c.logger.Debug("Failed to begin transaction", zap.Error(err))
						return fmt.Errorf("failed to begin transaction: %w", err)
					}
					c.logger.Debug("Transaction started for PR")
				}

				c.logger.Debug("Inserting PR into new table",
					zap.Int("pr_index", i),
					zap.Int("total_prs", len(prs)),
					zap.String("owner", owner),
					zap.String("repo", repo),
					zap.Int("pr_number", *pr.Number),
					zap.Time("closed_at", *closedAt),
					zap.Int("data_size", len(prData)))

				insertStart := time.Now()
				_, err = tx.ExecContext(ctx, `
					INSERT OR REPLACE INTO prs_new (owner, repo, pr_number, data, created_at, closed_at, timestamp)
					VALUES (?, ?, ?, ?, ?, ?, ?)
				`, owner, repo, *pr.Number, prData, createdAt, closedAt, timestamp)
				insertElapsed := time.Since(insertStart)

				if err != nil {
					c.logger.Debug("Failed to insert PR",
						zap.Int("pr_number", *pr.Number),
						zap.Duration("elapsed", insertElapsed),
						zap.Error(err))
					c.logger.Warn("Failed to insert migrated PR", zap.Error(err))
					prsSkipped++
					continue
				}

				c.logger.Debug("PR inserted successfully",
					zap.Int("pr_number", *pr.Number),
					zap.Duration("elapsed", insertElapsed))

				migratedCount++
				batchCount++
				prsInWindow++

				// Commit batch periodically to avoid huge transactions
				if batchCount >= batchSize {
					c.logger.Debug("Committing batch",
						zap.Int("batch_size", batchCount),
						zap.Int("migrated_count", migratedCount))
					commitStart := time.Now()
					if err := tx.Commit(); err != nil {
						c.logger.Debug("Failed to commit batch",
							zap.Duration("elapsed", time.Since(commitStart)),
							zap.Error(err))
						return fmt.Errorf("failed to commit migration batch: %w", err)
					}
					commitElapsed := time.Since(commitStart)
					c.logger.Info("Migration progress",
						zap.Int("processed_rows", processedRows),
						zap.Int("total_rows", totalRows),
						zap.Int("migrated_prs", migratedCount))
					c.logger.Debug("Batch committed successfully",
						zap.Int("batch_size", batchCount),
						zap.Duration("commit_elapsed", commitElapsed))
					tx = nil // Reset for next batch
					batchCount = 0

					// If there are more PRs in this row, start a new transaction
					if i < len(prs)-1 {
						c.logger.Debug("Starting new transaction for remaining PRs in row")
						tx, err = c.db.BeginTx(ctx, &sql.TxOptions{
							Isolation: sql.LevelDefault,
						})
						if err != nil {
							c.logger.Debug("Failed to begin new transaction", zap.Error(err))
							return fmt.Errorf("failed to begin new transaction: %w", err)
						}
						c.logger.Debug("New transaction started for remaining PRs")
					}
				}
			} else {
				c.logger.Debug("Skipping PR outside time window",
					zap.Int("pr_number", *pr.Number),
					zap.Time("closed_at", func() time.Time {
						if closedAt != nil {
							return *closedAt
						}
						return time.Time{}
					}()))
				prsSkipped++
			}
		}
		c.logger.Debug("Processed PRs from row",
			zap.Int("total_prs", len(prs)),
			zap.Int("prs_in_window", prsInWindow),
			zap.Int("prs_skipped", prsSkipped))

		// Log progress every 10 rows
		if processedRows%10 == 0 && totalRows > 0 {
			c.logger.Info("Migration progress",
				zap.Int("processed_rows", processedRows),
				zap.Int("total_rows", totalRows),
				zap.Int("migrated_prs", migratedCount))
		}
	}

	// Commit final batch if any
	if tx != nil {
		c.logger.Debug("Committing final batch", zap.Int("batch_size", batchCount))
		if err := tx.Commit(); err != nil {
			c.logger.Debug("Failed to commit final batch", zap.Error(err))
			return fmt.Errorf("failed to commit final migration batch: %w", err)
		}
		c.logger.Debug("Final batch committed successfully")
	}

	c.logger.Debug("All rows processed",
		zap.Int("processed_rows", processedRows),
		zap.Int("migrated_prs", migratedCount))

	// Drop old table and rename new table
	c.logger.Debug("Dropping old prs table")
	_, err = c.db.Exec(`DROP TABLE prs`)
	if err != nil {
		c.logger.Debug("Failed to drop old prs table", zap.Error(err))
		return fmt.Errorf("failed to drop old prs table: %w", err)
	}
	c.logger.Debug("Old prs table dropped successfully")

	c.logger.Debug("Renaming prs_new to prs")
	_, err = c.db.Exec(`ALTER TABLE prs_new RENAME TO prs`)
	if err != nil {
		c.logger.Debug("Failed to rename prs_new table", zap.Error(err))
		return fmt.Errorf("failed to rename new prs table: %w", err)
	}
	c.logger.Debug("Table renamed successfully")

	// Update schema version
	c.logger.Debug("Updating schema version", zap.Int("new_version", schemaVersion))
	_, err = c.db.Exec(`
		UPDATE cache_schema 
		SET version = ?, created_at = ?
		WHERE version = 1
	`, schemaVersion, time.Now())
	if err != nil {
		c.logger.Debug("Failed to update schema version", zap.Error(err))
		return fmt.Errorf("failed to update schema version: %w", err)
	}
	c.logger.Debug("Schema version updated successfully")

	c.logger.Info("PR cache migration complete",
		zap.Int("migrated_prs", migratedCount),
		zap.Int("schema_version", schemaVersion))
	return nil
}

// GetRepos retrieves cached repositories
func (c *SQLiteCache) GetRepos(ctx context.Context, org string) ([]*github.Repository, error) {
	var data []byte
	var timestamp time.Time

	c.logger.Debug("Getting cached repositories", zap.String("org", org))

	err := c.db.QueryRowContext(ctx,
		"SELECT data, timestamp FROM repos WHERE org = ?",
		org,
	).Scan(&data, &timestamp)

	if err == sql.ErrNoRows {
		c.logger.Debug("Cache entry not found", zap.String("org", org))
		return nil, fmt.Errorf("cache entry not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query cache: %w", err)
	}

	// Check expiration (unless ignoreTTL is set)
	if !c.ignoreTTL {
		entry := CacheEntry{Timestamp: timestamp}
		if entry.IsExpired(c.ttl) {
			c.logger.Debug("Cache entry expired", zap.String("org", org))
			return nil, fmt.Errorf("cache entry expired")
		}
	}

	// Unmarshal
	var repos []*github.Repository
	if err := json.Unmarshal(data, &repos); err != nil {
		c.logger.Debug("Failed to unmarshal data", zap.String("org", org), zap.Error(err))
		return nil, fmt.Errorf("failed to unmarshal data: %w", err)
	}

	return repos, nil
}

// SetRepos caches repositories
func (c *SQLiteCache) SetRepos(ctx context.Context, org string, repos []*github.Repository) error {
	data, err := json.Marshal(repos)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	_, err = c.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO repos (org, data, timestamp) VALUES (?, ?, ?)`,
		org, data, time.Now(),
	)

	return err
}

// GetCODEOWNERS retrieves cached CODEOWNERS file
func (c *SQLiteCache) GetCODEOWNERS(ctx context.Context, owner, repo string) ([]byte, error) {
	var data []byte
	var timestamp time.Time

	err := c.db.QueryRowContext(ctx,
		"SELECT data, timestamp FROM codeowners WHERE owner = ? AND repo = ?",
		owner, repo,
	).Scan(&data, &timestamp)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("cache entry not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query cache: %w", err)
	}

	// Check expiration (unless ignoreTTL is set)
	if !c.ignoreTTL {
		entry := CacheEntry{Timestamp: timestamp}
		if entry.IsExpired(c.ttl) {
			return nil, fmt.Errorf("cache entry expired")
		}
	}

	return data, nil
}

// SetCODEOWNERS caches CODEOWNERS file
func (c *SQLiteCache) SetCODEOWNERS(ctx context.Context, owner, repo string, content []byte) error {
	_, err := c.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO codeowners (owner, repo, data, timestamp) VALUES (?, ?, ?, ?)`,
		owner, repo, content, time.Now(),
	)

	return err
}

// GetPRs retrieves cached PRs for a repository, filtered by time window
func (c *SQLiteCache) GetPRs(ctx context.Context, owner, repo string, since, until time.Time) ([]*github.PullRequest, error) {
	rows, err := c.db.QueryContext(ctx,
		`SELECT data, closed_at, timestamp 
		 FROM prs 
		 WHERE owner = ? AND repo = ? 
		 AND closed_at IS NOT NULL 
		 AND closed_at >= ? AND closed_at <= ?`,
		owner, repo, since, until,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query cache: %w", err)
	}
	defer rows.Close()

	var prs []*github.PullRequest
	var hasExpiredEntries bool
	oldestTimestamp := time.Now()

	for rows.Next() {
		var data []byte
		var closedAt sql.NullTime
		var timestamp time.Time

		if err := rows.Scan(&data, &closedAt, &timestamp); err != nil {
			c.logger.Warn("Failed to scan PR data", zap.Error(err))
			continue
		}

		// Check expiration (unless ignoreTTL is set)
		if !c.ignoreTTL {
			entry := CacheEntry{Timestamp: timestamp}
			if entry.IsExpired(c.ttl) {
				hasExpiredEntries = true
				if timestamp.Before(oldestTimestamp) {
					oldestTimestamp = timestamp
				}
				continue
			}
		}

		// Unmarshal PR
		var pr github.PullRequest
		if err := json.Unmarshal(data, &pr); err != nil {
			c.logger.Warn("Failed to unmarshal PR data", zap.Error(err))
			continue
		}

		// Double-check the PR is within the time window
		if pr.ClosedAt != nil {
			closedAtTime := pr.ClosedAt.Time
			if !closedAtTime.Before(since) && !closedAtTime.After(until) {
				prs = append(prs, &pr)
			}
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating PR rows: %w", err)
	}

	// If we have no PRs but found expired entries, return an error
	if len(prs) == 0 && hasExpiredEntries {
		return nil, fmt.Errorf("cache entry expired")
	}

	// If we have no PRs at all, return not found
	if len(prs) == 0 {
		return nil, fmt.Errorf("cache entry not found")
	}

	return prs, nil
}

// SetPRs caches PRs for a repository (stores individual PRs by ID)
func (c *SQLiteCache) SetPRs(ctx context.Context, owner, repo string, prs []*github.PullRequest) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now()
	for _, pr := range prs {
		if pr.Number == nil {
			continue
		}

		prData, err := json.Marshal(pr)
		if err != nil {
			c.logger.Warn("Failed to marshal PR", zap.Error(err))
			continue
		}

		var createdAt, closedAt *time.Time
		if pr.CreatedAt != nil {
			createdAt = &pr.CreatedAt.Time
		}
		if pr.ClosedAt != nil {
			closedAt = &pr.ClosedAt.Time
		}

		_, err = tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO prs (owner, repo, pr_number, data, created_at, closed_at, timestamp) 
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			owner, repo, *pr.Number, prData, createdAt, closedAt, now,
		)
		if err != nil {
			return fmt.Errorf("failed to insert PR: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetPRFiles retrieves cached PR files
func (c *SQLiteCache) GetPRFiles(ctx context.Context, owner, repo string, prNumber int) ([]*github.CommitFile, error) {
	var data []byte
	var timestamp time.Time

	err := c.db.QueryRowContext(ctx,
		"SELECT data, timestamp FROM pr_files WHERE owner = ? AND repo = ? AND pr_number = ?",
		owner, repo, prNumber,
	).Scan(&data, &timestamp)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("cache entry not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query cache: %w", err)
	}

	// Check expiration (unless ignoreTTL is set)
	if !c.ignoreTTL {
		entry := CacheEntry{Timestamp: timestamp}
		if entry.IsExpired(c.ttl) {
			return nil, fmt.Errorf("cache entry expired")
		}
	}

	// Unmarshal
	var files []*github.CommitFile
	if err := json.Unmarshal(data, &files); err != nil {
		return nil, fmt.Errorf("failed to unmarshal data: %w", err)
	}

	return files, nil
}

// SetPRFiles caches PR files
func (c *SQLiteCache) SetPRFiles(ctx context.Context, owner, repo string, prNumber int, files []*github.CommitFile) error {
	data, err := json.Marshal(files)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	_, err = c.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO pr_files (owner, repo, pr_number, data, timestamp) VALUES (?, ?, ?, ?, ?)`,
		owner, repo, prNumber, data, time.Now(),
	)

	return err
}

// Invalidate invalidates all cache entries
func (c *SQLiteCache) Invalidate(ctx context.Context) error {
	tables := []string{"repos", "codeowners", "prs", "pr_files"}
	for _, table := range tables {
		if _, err := c.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			return fmt.Errorf("failed to invalidate %s: %w", table, err)
		}
	}
	return nil
}

// InvalidateRepo invalidates cache for a specific repository
func (c *SQLiteCache) InvalidateRepo(ctx context.Context, owner, repo string) error {
	_, err := c.db.ExecContext(ctx,
		"DELETE FROM codeowners WHERE owner = ? AND repo = ?",
		owner, repo,
	)
	if err != nil {
		return fmt.Errorf("failed to invalidate codeowners: %w", err)
	}

	_, err = c.db.ExecContext(ctx,
		"DELETE FROM prs WHERE owner = ? AND repo = ?",
		owner, repo,
	)
	if err != nil {
		return fmt.Errorf("failed to invalidate prs: %w", err)
	}

	_, err = c.db.ExecContext(ctx,
		"DELETE FROM pr_files WHERE owner = ? AND repo = ?",
		owner, repo,
	)
	if err != nil {
		return fmt.Errorf("failed to invalidate pr_files: %w", err)
	}

	return nil
}

// Close closes the cache
func (c *SQLiteCache) Close() error {
	return c.db.Close()
}
