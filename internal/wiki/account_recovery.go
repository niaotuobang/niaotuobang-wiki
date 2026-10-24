package wiki

import (
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

// RecoverSystemAdmin is intentionally CLI-only. A service-user-readable password
// file is required so credentials never appear in process arguments or logs.
func (a *App) RecoverSystemAdmin(id, passwordFile string) error {
	info, e := os.Stat(passwordFile)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("recovery requires an owner-only password file")
	}
	value, e := os.ReadFile(passwordFile)
	if e != nil || len(value) < 10 || len(value) > 72 {
		return fmt.Errorf("recovery password must contain 10 to 72 bytes")
	}
	hash, e := bcrypt.GenerateFromPassword(value, bcrypt.DefaultCost)
	if e != nil {
		return e
	}
	tx, e := a.store.DB.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var role string
	if e = tx.QueryRow(`SELECT access_role FROM users WHERE id=? AND disabled=0`, id).Scan(&role); e != nil || role != "system_admin" {
		return fmt.Errorf("recovery target must be an enabled system administrator")
	}
	if _, e = tx.Exec(`UPDATE users SET password_hash=? WHERE id=?`, hash, id); e != nil {
		return e
	}
	if e = revokeCredentials(tx, id); e != nil {
		return e
	}
	if e = audit(tx, id, "system.local_recovery", id); e != nil {
		return e
	}
	return tx.Commit()
}
