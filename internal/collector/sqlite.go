package collector

import (
	"database/sql"
	"strings"

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

// --- Shared SQLite column helpers ---
//
// One defensive column-probing implementation shared by every SQLite adapter
// (goose/hermes/octofriend/kiro/kilo): PRAGMA the actual columns, bind whichever
// documented spelling is present, and select NULL for absent columns so the
// positional Scan layout stays fixed when a schema drifts.

// sqliteTableColumns returns the lower-cased column names of a table via PRAGMA
// table_info. A missing table yields an empty set (no error from the PRAGMA
// itself), which callers treat as "no usable table".
func sqliteTableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query("PRAGMA table_info(" + sqliteQuoteIdent(table) + ")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   sql.NullString
			notnull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols[strings.ToLower(name)] = true
	}
	return cols, rows.Err()
}

// sqliteFirstPresent returns the first candidate column present in cols, or "".
func sqliteFirstPresent(cols map[string]bool, candidates []string) string {
	for _, c := range candidates {
		if cols[strings.ToLower(c)] {
			return c
		}
	}
	return ""
}

// sqliteSelectExpr returns a quoted column reference, or NULL when the column is
// absent so the positional Scan layout stays fixed.
func sqliteSelectExpr(col string) string {
	if col == "" {
		return "NULL"
	}
	return sqliteQuoteIdent(col)
}

// sqliteQuoteIdent double-quotes a SQLite identifier, escaping embedded quotes.
func sqliteQuoteIdent(ident string) string {
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}

func sqliteInt(v sql.NullInt64) int {
	if !v.Valid {
		return 0
	}
	return int(v.Int64)
}

func sqliteStr(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return v.String
}

// sqliteFirstPtr returns the first non-nil pointer's value, or 0.
func sqliteFirstPtr(ptrs ...*int) int {
	for _, p := range ptrs {
		if p != nil {
			return *p
		}
	}
	return 0
}
