package store

import (
	"fmt"
	"strings"
)

// Backup writes a transactionally consistent snapshot of the database to
// dest. It uses VACUUM INTO, which is safe to run against a live database
// (unlike copying the file out from under WAL mode). Overwrites dest.
func (s *Store) Backup(dest string) error {
	if strings.TrimSpace(dest) == "" {
		return fmt.Errorf("backup destination must not be empty")
	}
	lit := "'" + strings.ReplaceAll(dest, "'", "''") + "'"
	if _, err := s.db.Exec(`VACUUM INTO ` + lit); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	return nil
}
