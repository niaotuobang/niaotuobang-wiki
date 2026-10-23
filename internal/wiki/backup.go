package wiki

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type BackupManifest struct {
	Version   int               `json:"version"`
	CreatedAt string            `json:"created_at"`
	Files     map[string]string `json:"sha256"`
}

func (a *App) Backup(output string) (string, error) {
	a.uploadMu.Lock()
	defer a.uploadMu.Unlock()
	if e := os.MkdirAll(output, 0700); e != nil {
		return "", e
	}
	work, e := os.MkdirTemp(a.store.DataDir, ".backup-work-")
	if e != nil {
		return "", e
	}
	defer os.RemoveAll(work)
	snapshot := filepath.Join(work, "wiki.db")
	if _, e = a.store.DB.Exec(`VACUUM INTO ?`, snapshot); e != nil {
		return "", e
	}
	db, e := sql.Open("sqlite", snapshot)
	if e != nil {
		return "", e
	}
	defer db.Close()
	if _, e = db.Exec(`DELETE FROM sessions; DELETE FROM invites;`); e != nil {
		return "", e
	}
	rows, e := db.Query(`SELECT object_key,size FROM media WHERE state='ready'`)
	if e != nil {
		return "", e
	}
	type item struct {
		key  string
		size int64
	}
	media := []item{}
	for rows.Next() {
		var m item
		if e = rows.Scan(&m.key, &m.size); e != nil {
			rows.Close()
			return "", e
		}
		media = append(media, m)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return "", e
	}
	db.Close()
	f, e := os.CreateTemp(output, ".backup-")
	if e != nil {
		return "", e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	manifest := BackupManifest{1, now(), map[string]string{}}
	add := func(name string, size int64, src io.Reader) error {
		if e := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: size}); e != nil {
			return e
		}
		h := sha256.New()
		n, e := io.Copy(io.MultiWriter(tw, h), io.LimitReader(src, size+1))
		if e != nil {
			return e
		}
		if n != size {
			return fmt.Errorf("backup media size mismatch")
		}
		manifest.Files[name] = hex.EncodeToString(h.Sum(nil))
		return nil
	}
	sf, e := os.Open(snapshot)
	if e != nil {
		return "", e
	}
	info, e := sf.Stat()
	if e != nil {
		sf.Close()
		return "", e
	}
	e = add("wiki.db", info.Size(), sf)
	sf.Close()
	if e != nil {
		return "", e
	}
	for _, m := range media {
		var reader io.ReadCloser
		if a.oss != nil {
			reader, e = a.oss.bucket.GetObject(m.key)
		} else {
			reader, e = os.Open(filepath.Join(a.store.DataDir, m.key))
		}
		if e != nil {
			return "", e
		}
		e = add(m.key, m.size, reader)
		reader.Close()
		if e != nil {
			return "", e
		}
	}
	b, e := json.MarshalIndent(manifest, "", "  ")
	if e != nil {
		return "", e
	}
	if e = tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Size: int64(len(b))}); e != nil {
		return "", e
	}
	if _, e = tw.Write(b); e != nil {
		return "", e
	}
	if e = tw.Close(); e != nil {
		return "", e
	}
	if e = gz.Close(); e != nil {
		return "", e
	}
	if e = f.Sync(); e != nil {
		return "", e
	}
	if e = f.Close(); e != nil {
		return "", e
	}
	target := filepath.Join(output, "wiki-"+newID()+".tar.gz")
	if e = os.Rename(tmp, target); e != nil {
		return "", e
	}
	return target, nil
}
func RestoreBackup(input, target string) error {
	if entries, e := os.ReadDir(target); e == nil && len(entries) > 0 {
		return fmt.Errorf("restore target must be empty")
	}
	parent := filepath.Dir(target)
	if e := os.MkdirAll(parent, 0700); e != nil {
		return e
	}
	work, e := os.MkdirTemp(parent, ".wiki-restore-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(work)
	f, e := os.Open(input)
	if e != nil {
		return e
	}
	defer f.Close()
	gz, e := gzip.NewReader(f)
	if e != nil {
		return e
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	hashes := map[string]string{}
	var manifest BackupManifest
	var total int64
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
		name := h.Name
		if h.Typeflag != tar.TypeReg || name != "wiki.db" && name != "manifest.json" && !(strings.HasPrefix(name, "media/") && safeID.MatchString(strings.TrimPrefix(name, "media/"))) {
			return fmt.Errorf("invalid backup entry")
		}
		if _, ok := hashes[name]; ok {
			return fmt.Errorf("duplicate backup entry")
		}
		total += h.Size
		if h.Size < 0 || h.Size > 1<<30 || total > 20<<30 {
			return fmt.Errorf("backup exceeds restore limit")
		}
		if name == "manifest.json" {
			if h.Size > 4<<20 {
				return fmt.Errorf("manifest too large")
			}
			if e = json.NewDecoder(tr).Decode(&manifest); e != nil {
				return e
			}
			hashes[name] = "manifest"
			continue
		}
		path := filepath.Join(work, name)
		if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return e
		}
		out, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		hash := sha256.New()
		_, e = io.Copy(io.MultiWriter(out, hash), tr)
		closeErr := out.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
		hashes[name] = hex.EncodeToString(hash.Sum(nil))
	}
	if manifest.Version != 1 || manifest.Files["wiki.db"] == "" || len(manifest.Files) != len(hashes)-1 {
		return fmt.Errorf("backup manifest is incomplete")
	}
	for name, checksum := range manifest.Files {
		if hashes[name] != checksum {
			return fmt.Errorf("backup checksum mismatch")
		}
	}
	db, e := sql.Open("sqlite", filepath.Join(work, "wiki.db"))
	if e != nil {
		return e
	}
	var check string
	e = db.QueryRow(`PRAGMA integrity_check`).Scan(&check)
	db.Close()
	if e != nil {
		return e
	}
	if check != "ok" {
		return fmt.Errorf("backup database integrity check failed")
	}
	if entries, e := os.ReadDir(target); e == nil {
		if len(entries) != 0 {
			return fmt.Errorf("restore target became non-empty")
		}
		if e = os.Remove(target); e != nil {
			return e
		}
	}
	return os.Rename(work, target)
}
func (a *App) downloadBackup(w http.ResponseWriter, r *http.Request, _ *User) error {
	dir, e := os.MkdirTemp(a.store.DataDir, ".backup-download-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(dir)
	path, e := a.Backup(dir)
	if e != nil {
		return e
	}
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil {
		return e
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", `attachment; filename="wiki-backup.tar.gz"`)
	http.ServeContent(w, r, "wiki-backup.tar.gz", info.ModTime(), f)
	return nil
}
