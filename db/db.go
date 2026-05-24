package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

// InitDB initializes the SQLite database, runs migrations, and configures connection pooling
func InitDB(dbPath string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create db directory: %w", err)
	}

	// Open SQLite database using the pure Go driver "sqlite"
	var err error
	DB, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Configure database connection pool to avoid "database is locked" errors.
	// SQLite supports multiple readers but only one writer. Using WAL mode
	// we can safely set max open connections to a small pool (e.g. 10) to support concurrency.
	DB.SetMaxOpenConns(10)
	DB.SetMaxIdleConns(5)
	DB.SetConnMaxLifetime(time.Hour)

	// Enable WAL (Write-Ahead Logging) and set other performance pragmas
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA foreign_keys=ON;",
		"PRAGMA busy_timeout=5000;", // wait up to 5 seconds if locked
		"PRAGMA auto_vacuum=INCREMENTAL;",
		"PRAGMA cache_size=-1000;", // limit page cache to ~1MB RAM
	}
	for _, pragma := range pragmas {
		if _, err := DB.Exec(pragma); err != nil {
			return fmt.Errorf("failed to execute pragma (%s): %w", pragma, err)
		}
	}

	if err := createTables(); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	runMigrations()

	// Start asynchronous background log cleanup daemon (keeps DB size optimized)
	go startLogCleanupDaemon()

	return nil
}

func createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL CHECK(role IN ('super_admin', 'admin', 'viewer')),
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,

		`CREATE TABLE IF NOT EXISTS monitors (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			type TEXT NOT NULL CHECK(type IN ('http', 'ping')),
			target TEXT NOT NULL,
			interval INTEGER NOT NULL,
			timeout INTEGER NOT NULL,
			retries INTEGER NOT NULL DEFAULT 3,
			active INTEGER NOT NULL DEFAULT 1 CHECK(active IN (0, 1)),
			keyword TEXT,
			ssl_expiry_warning INTEGER DEFAULT 0 CHECK(ssl_expiry_warning IN (0, 1)),
			public INTEGER NOT NULL DEFAULT 0 CHECK(public IN (0, 1)),
			status TEXT NOT NULL DEFAULT 'unknown' CHECK(status IN ('up', 'down', 'unknown')),
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,

		`CREATE TABLE IF NOT EXISTS monitor_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			monitor_id INTEGER NOT NULL,
			status TEXT NOT NULL CHECK(status IN ('up', 'down')),
			response_time INTEGER,
			status_code INTEGER,
			message TEXT,
			ssl_days_remaining INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(monitor_id) REFERENCES monitors(id) ON DELETE CASCADE
		);`,

		`CREATE INDEX IF NOT EXISTS idx_logs_monitor_time ON monitor_logs(monitor_id, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_logs_created_at ON monitor_logs(created_at);`,

		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
		
		`CREATE TABLE IF NOT EXISTS sessions (
			token TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			expires_at DATETIME NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,
	}

	for _, query := range queries {
		if _, err := DB.Exec(query); err != nil {
			return err
		}
	}
	return nil
}

// startLogCleanupDaemon deletes logs older than 30 days and runs Incremental Vacuum to reclaim space
func startLogCleanupDaemon() {
	ticker := time.NewTicker(6 * time.Hour) // Run every 6 hours
	defer ticker.Stop()

	// Run initially on startup
	pruneLogs()

	for range ticker.C {
		pruneLogs()
	}
}

func pruneLogs() {
	// Delete detailed logs older than 30 days
	rows, err := DB.Exec(`DELETE FROM monitor_logs WHERE created_at < datetime('now', '-30 days')`)
	if err != nil {
		log.Printf("[DB Cleanup] Error pruning logs: %v", err)
		return
	}
	
	rowsAff, err := rows.RowsAffected()
	if err == nil && rowsAff > 0 {
		log.Printf("[DB Cleanup] Pruned %d database logs older than 30 days.", rowsAff)
		// Run incremental vacuum to reclaim deleted space back to OS
		if _, err := DB.Exec(`PRAGMA incremental_vacuum(50);`); err != nil {
			log.Printf("[DB Cleanup] Error running vacuum: %v", err)
		}
	}
	
	// Force SQLite to release as much memory as possible back to the OS
	_, _ = DB.Exec("PRAGMA shrink_memory;")
}

func runMigrations() {
	// Add alert_interval to monitors
	_, _ = DB.Exec("ALTER TABLE monitors ADD COLUMN alert_interval INTEGER DEFAULT 0")

	// Add api_key to users
	_, _ = DB.Exec("ALTER TABLE users ADD COLUMN api_key TEXT")

	// Generate api keys for users who don't have one
	rows, err := DB.Query("SELECT id FROM users WHERE api_key IS NULL OR api_key = ''")
	if err == nil {
		type userToUpdate struct {
			id int64
		}
		var users []userToUpdate
		for rows.Next() {
			var u userToUpdate
			if err := rows.Scan(&u.id); err == nil {
				users = append(users, u)
			}
		}
		rows.Close()

		for _, u := range users {
			b := make([]byte, 16)
			_, _ = rand.Read(b)
			apiKey := hex.EncodeToString(b)
			_, _ = DB.Exec("UPDATE users SET api_key = ? WHERE id = ?", apiKey, u.id)
		}
	}

	// Clean up and update default alert templates if they contain "////" or represent the old English defaults
	_, _ = DB.Exec(`UPDATE settings SET value = '🔴 Servis Çevrimdışı: {name}' WHERE key = 'alert_subject_down' AND (value LIKE '%////%' OR value = '🔴 Monitor Down: {name}' OR value = '')`)
	_, _ = DB.Exec(`UPDATE settings SET value = '⚠️ **{name}** ({target}) servisi ÇEVRİMDışı durumuna geçti.
⏱️ Gecikme: {latency}ms
🔍 Hata Detayı: {message}' WHERE key = 'alert_body_down' AND (value LIKE '%////%' OR value LIKE '%changed state to%' OR value = '')`)
	_, _ = DB.Exec(`UPDATE settings SET value = '🟢 Servis Çevrimiçi: {name}' WHERE key = 'alert_subject_up' AND (value LIKE '%////%' OR value = '🟢 Monitor Up: {name}' OR value = '')`)
	_, _ = DB.Exec(`UPDATE settings SET value = '✅ **{name}** ({target}) servisi tekrar çevrimiçi.
⏱️ Gecikme: {latency}ms' WHERE key = 'alert_body_up' AND (value LIKE '%////%' OR value LIKE '%back online%' OR value = '')`)

	// Unquote any settings that were double-quoted by previous auto-save bugs
	srows, serr := DB.Query("SELECT key, value FROM settings")
	if serr == nil {
		type settingUpdate struct {
			key string
			val string
		}
		var updates []settingUpdate
		for srows.Next() {
			var k, v string
			if err := srows.Scan(&k, &v); err == nil {
				// Try unquoting
				if unquoted, err := strconv.Unquote(v); err == nil {
					// We might have double unquoting needed if it was saved multiple times
					for {
						if u, err := strconv.Unquote(unquoted); err == nil {
							unquoted = u
						} else {
							break
						}
					}
					updates = append(updates, settingUpdate{key: k, val: unquoted})
				}
			}
		}
		srows.Close()
		for _, upd := range updates {
			_, _ = DB.Exec("UPDATE settings SET value = ? WHERE key = ?", upd.val, upd.key)
		}
	}
}

