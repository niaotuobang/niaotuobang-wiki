package wiki

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

type AccountImportReport struct {
	Users              int  `json:"users"`
	Identities         int  `json:"identities"`
	SharedEmailAliases int  `json:"shared_email_aliases"`
	Applied            bool `json:"applied"`
}

// ImportMatterwikiAccounts is an explicit, server-local migration operation.
// Passwords are never read. Shared mailboxes have one login identity while all
// original user IDs, names and biographies remain separate historical records.
func (a *App) ImportMatterwikiAccounts(input, primaryID string, apply bool) (report AccountImportReport, err error) {
	if a.auth == nil {
		return report, fmt.Errorf("account import requires the target authentication configuration")
	}
	raw, err := os.ReadFile(input)
	if err != nil {
		return report, err
	}
	if len(raw) > 64<<20 {
		return report, fmt.Errorf("import bundle exceeds 64 MiB")
	}
	var bundle legacyBundle
	if err = json.Unmarshal(raw, &bundle); err != nil {
		return report, err
	}
	sort.SliceStable(bundle.Users, func(i, j int) bool {
		x, e1 := strconv.Atoi(string(bundle.Users[i].ID))
		y, e2 := strconv.Atoi(string(bundle.Users[j].ID))
		if e1 == nil && e2 == nil {
			return x < y
		}
		return bundle.Users[i].ID < bundle.Users[j].ID
	})
	known := map[string]bool{}
	emails := map[string]string{}
	canonical := map[string]string{}
	cipher := map[string][]byte{}
	primaryFound := false
	for _, u := range bundle.Users {
		id := "legacy-user-" + string(u.ID)
		if u.ID == "" || u.Name == "" || known[id] {
			return report, fmt.Errorf("missing or duplicate legacy account identity")
		}
		known[id] = true
		address, e := normalizeIdentity("email", u.Email)
		if e != nil {
			return report, fmt.Errorf("legacy account %s has an invalid mailbox", u.ID)
		}
		emails[id] = address
		if _, ok := canonical[address]; !ok {
			canonical[address] = id
		}
		if string(u.ID) == primaryID {
			primaryFound = true
			canonical[address] = id
		}
		cipher[id], err = a.auth.encrypt("email", address)
		if err != nil {
			return report, err
		}
	}
	if !primaryFound {
		return report, fmt.Errorf("select an existing legacy account as initial system administrator")
	}
	allowedTarget := map[string]bool{}
	for id := range known {
		allowedTarget[id] = true
	}
	for _, article := range bundle.Articles {
		for _, revision := range append(article.History, article.legacyRevision) {
			if revision.UserID != "" {
				allowedTarget["legacy-user-"+string(revision.UserID)] = true
			}
		}
	}
	rows, err := a.store.DB.Query(`SELECT id FROM users`)
	if err != nil {
		return report, err
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return report, err
		}
		if !allowedTarget[id] {
			rows.Close()
			return report, fmt.Errorf("account import requires a clean target or the same previously imported accounts")
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return report, err
	}
	report.Users = len(bundle.Users)
	report.Identities = len(canonical)
	report.SharedEmailAliases = report.Users - report.Identities
	if !apply {
		return report, nil
	}
	tx, err := a.store.DB.Begin()
	if err != nil {
		return report, err
	}
	defer tx.Rollback()
	for _, old := range bundle.Users {
		id := "legacy-user-" + string(old.ID)
		var existing int
		if err = tx.QueryRow(`SELECT count(*) FROM users WHERE id=?`, id).Scan(&existing); err != nil {
			return report, err
		}
		if existing == 0 {
			u := User{ID: id, Login: id, Name: old.Name, Role: "user", CanEdit: true}
			if string(old.ID) == primaryID {
				u.Role = "system_admin"
			}
			if err = insertUser(tx, u, []byte{}); err != nil {
				return report, err
			}
		}
	}
	for _, old := range bundle.Users {
		id := "legacy-user-" + string(old.ID)
		address := emails[id]
		owner := canonical[address]
		if _, err = tx.Exec(`INSERT INTO legacy_user_profiles(user_id,about,created_at,updated_at,email_cipher,canonical_user_id) VALUES(?,?,?,?,?,?) ON CONFLICT(user_id) DO NOTHING`, id, old.About, old.CreatedAt, old.UpdatedAt, cipher[id], owner); err != nil {
			return report, err
		}
		if _, err = tx.Exec(`UPDATE users SET created_at=? WHERE id=?`, old.CreatedAt, id); err != nil {
			return report, err
		}
		if id != owner {
			continue
		}
		lookup := a.auth.digest("identity-index", "email", address)
		var bound string
		err = tx.QueryRow(`SELECT user_id FROM user_identities WHERE kind='email' AND lookup_hash=?`, lookup).Scan(&bound)
		if err == nil {
			if bound != id {
				return report, fmt.Errorf("mailbox is already bound to a different target account")
			}
			continue
		}
		if err != sql.ErrNoRows {
			return report, err
		}
		if _, err = tx.Exec(`INSERT INTO user_identities(id,user_id,kind,address_cipher,lookup_hash,verified_at) VALUES(?,?,'email',?,?,'')`, newID(), id, cipher[id], lookup); err != nil {
			return report, err
		}
	}
	if err = audit(tx, "", "migration.accounts", primaryID); err != nil {
		return report, err
	}
	if err = tx.Commit(); err != nil {
		return report, err
	}
	a.authMu.Lock()
	a.setupToken = ""
	a.authMu.Unlock()
	os.Remove(filepath.Join(a.store.DataDir, "setup-token"))
	report.Applied = true
	return report, nil
}
