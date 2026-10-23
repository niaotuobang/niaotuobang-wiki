package wiki

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// Stage immutable media before opening the import's database transaction.
func (a *App) stageImportedMedia(media map[string]importedMedia) (staged []importedMedia, cleanup func(), err error) {
	keys := []string{}
	cleanup = func() {
		for _, key := range keys {
			if a.oss != nil {
				a.oss.bucket.DeleteObject(key)
			} else {
				os.Remove(filepath.Join(a.store.DataDir, key))
			}
		}
	}
	for _, m := range media {
		var exists int
		if err = a.store.DB.QueryRow(`SELECT count(*) FROM media WHERE id=?`, m.ID).Scan(&exists); err != nil {
			return
		}
		if exists > 0 {
			continue
		}
		keys = append(keys, m.Key)
		m, err = a.stageOneImportedMedia(m)
		if err != nil {
			return
		}
		staged = append(staged, m)
	}
	return
}
func (a *App) stageOneImportedMedia(m importedMedia) (importedMedia, error) {
	source, e := os.Open(m.Path)
	if e != nil {
		return m, e
	}
	defer source.Close()
	temp, e := os.CreateTemp(a.store.DataDir, ".import-media-")
	if e != nil {
		return m, e
	}
	defer os.Remove(temp.Name())
	defer temp.Close()
	h := sha256.New()
	n, e := io.Copy(io.MultiWriter(temp, h), io.LimitReader(source, m.Size+1))
	if e != nil {
		return m, e
	}
	if n != m.Size {
		return m, fmt.Errorf("import media changed size during preparation")
	}
	m.Checksum = hex.EncodeToString(h.Sum(nil))
	if _, e = temp.Seek(0, 0); e != nil {
		return m, e
	}
	prefix, e := io.ReadAll(io.LimitReader(temp, 65536))
	if e != nil {
		return m, e
	}
	if e = inspectMedia(prefix, m.ContentType); e != nil {
		return m, e
	}
	if a.oss != nil {
		if _, e = temp.Seek(0, 0); e == nil {
			e = a.oss.bucket.PutObject(m.Key, temp, oss.ContentType(m.ContentType), oss.ObjectACL(oss.ACLPrivate))
		}
		return m, e
	}
	if e = temp.Close(); e != nil {
		return m, e
	}
	if e = os.MkdirAll(filepath.Join(a.store.DataDir, "media"), 0700); e != nil {
		return m, e
	}
	e = os.Rename(temp.Name(), filepath.Join(a.store.DataDir, m.Key))
	return m, e
}
