package wiki

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

func (a *App) CleanupUploads() error {
	a.uploadMu.Lock()
	defer a.uploadMu.Unlock()
	cutoff := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	rows, e := a.store.DB.Query(`SELECT id,object_key FROM media WHERE state IN ('pending','uploaded','cancelled') AND created_at<? LIMIT 100`, cutoff)
	if e != nil {
		return e
	}
	type item struct{ id, key string }
	items := []item{}
	for rows.Next() {
		var m item
		if e = rows.Scan(&m.id, &m.key); e != nil {
			rows.Close()
			return e
		}
		items = append(items, m)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, m := range items {
		if a.oss != nil {
			e = a.oss.bucket.DeleteObject(m.key)
		} else {
			e = os.Remove(filepath.Join(a.store.DataDir, m.key))
			if os.IsNotExist(e) {
				e = nil
			}
		}
		if e != nil {
			return e
		}
		if _, e = a.store.DB.Exec(`UPDATE media SET state='expired' WHERE id=?`, m.id); e != nil {
			return e
		}
	}
	_, e = a.store.DB.Exec(`DELETE FROM sessions WHERE expires_at<?`, now())
	if e != nil {
		return e
	}
	cutoffAuth := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339Nano)
	for _, q := range []string{`DELETE FROM auth_challenges WHERE id IN (SELECT id FROM auth_challenges WHERE expires_at<? LIMIT 500)`, `DELETE FROM auth_rate_limits WHERE key IN (SELECT key FROM auth_rate_limits WHERE expires_at<? LIMIT 500)`} {
		if _, e = a.store.DB.Exec(q, cutoffAuth); e != nil {
			return e
		}
	}
	_, e = a.store.DB.Exec(`DELETE FROM auth_audit_events WHERE id IN (SELECT id FROM auth_audit_events WHERE created_at<? LIMIT 500)`, time.Now().UTC().Add(-90*24*time.Hour).Format(time.RFC3339Nano))
	return e
}
func (a *App) Maintain(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.CleanupUploads()
		}
	}
}
