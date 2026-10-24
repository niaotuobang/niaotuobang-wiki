package wiki

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

func migrate(db *sql.DB, version int, dir string) error {
	const current = 10
	if version > current {
		return fmt.Errorf("database schema %d is newer than supported version %d", version, current)
	}
	if version == current {
		return nil
	}
	if version > 0 {
		backupDir := filepath.Join(dir, "migration-backups")
		if e := os.MkdirAll(backupDir, 0700); e != nil {
			return e
		}
		if _, e := db.Exec(`VACUUM INTO ?`, filepath.Join(backupDir, fmt.Sprintf("schema-%d-%s.db", version, newID()))); e != nil {
			return e
		}
	}
	tx, e := db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for _, m := range []struct {
		version int
		file    string
	}{{1, "001_initial.sql"}, {2, "002_link_context.sql"}, {3, "003_member_access.sql"}, {4, "004_private_accounts.sql"}, {5, "005_invite_history.sql"}, {6, "006_invite_notes.sql"}, {7, "007_email_only.sql"}, {8, "008_user_avatars.sql"}, {9, "009_legacy_provenance.sql"}, {10, "010_user_creation_time.sql"}} {
		if m.version <= version {
			continue
		}
		b, e := migrationFiles.ReadFile("migrations/" + m.file)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(string(b)); e != nil {
			return fmt.Errorf("migration %d: %w", m.version, e)
		}
	}
	if _, e = tx.Exec(`PRAGMA user_version=10`); e != nil {
		return e
	}
	return tx.Commit()
}
