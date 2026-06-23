package collector

import (
	"database/sql"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (registers driver "sqlite")
)

// sqliteDriver is the driver name registered by modernc.org/sqlite.
const sqliteDriver = "sqlite"

// openReadOnlySQLite opens a SQLite database for read-only access. The DSN uses
// mode=ro and immutable=1 so a live agent process holding the DB is never
// blocked or disturbed (immutable promises the file will not change under us,
// which suits a point-in-time scan), plus a busy_timeout as a belt-and-braces
// guard against transient lock contention. The agent DBs scanned here
// (cursor/zed/opencode/goose) are only ever read, never written.
func openReadOnlySQLite(path string) (*sql.DB, error) {
	dsn := "file:" + path + "?mode=ro&immutable=1&_pragma=busy_timeout(5000)"
	return sql.Open(sqliteDriver, dsn)
}
