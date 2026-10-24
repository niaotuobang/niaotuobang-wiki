package wiki

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/draw"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	xdraw "golang.org/x/image/draw"
)

func (a *App) avatarRoutes() {
	a.route("POST /api/v1/account/avatar", "read", func(w http.ResponseWriter, r *http.Request, u *User) error {
		// Serialize image decoding and backup together to bound memory and keep
		// the database reference and media object consistent during a backup.
		a.uploadMu.Lock()
		defer a.uploadMu.Unlock()
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			return fail(413, "AVATAR_TOO_LARGE", "头像不能超过 2 MiB")
		}
		cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
		if err != nil || (format != "jpeg" && format != "png" && format != "webp") || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 4_000_000 {
			return fail(400, "VALIDATION_ERROR", "请选择 PNG、JPEG 或 WebP 图片，最多 400 万像素")
		}
		src, _, err := image.Decode(bytes.NewReader(raw))
		if err != nil {
			return fail(400, "VALIDATION_ERROR", "头像图片无法读取")
		}
		bounds := src.Bounds()
		side := min(bounds.Dx(), bounds.Dy())
		x, y := bounds.Min.X+(bounds.Dx()-side)/2, bounds.Min.Y+(bounds.Dy()-side)/2
		dst := image.NewRGBA(image.Rect(0, 0, 256, 256))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, image.Rect(x, y, x+side, y+side), draw.Src, nil)
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, dst); err != nil {
			return err
		}
		id, key := newID(), "media/"+newID()
		if a.oss != nil {
			err = a.oss.bucket.PutObject(key, bytes.NewReader(encoded.Bytes()), oss.ContentType("image/png"))
		} else {
			path := filepath.Join(a.store.DataDir, key)
			err = os.MkdirAll(filepath.Dir(path), 0700)
			if err == nil {
				err = os.WriteFile(path, encoded.Bytes(), 0600)
			}
		}
		if err != nil {
			return err
		}
		committed := false
		defer func() {
			if !committed {
				if a.oss != nil {
					a.oss.bucket.DeleteObject(key)
				} else {
					os.Remove(filepath.Join(a.store.DataDir, key))
				}
			}
		}()
		tx, err := a.store.DB.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		hash := sha256.Sum256(encoded.Bytes())
		if _, err = tx.Exec(`INSERT INTO media(id,owner_id,name,content_type,size,state,object_key,created_at,checksum) VALUES(?,?,?,'image/png',?,'ready',?,?,?)`, id, u.ID, "avatar.png", encoded.Len(), key, now(), hex.EncodeToString(hash[:])); err != nil {
			return err
		}
		result, err := tx.Exec(`UPDATE users SET avatar_media_id=? WHERE id=? AND disabled=0`, id, u.ID)
		if err != nil {
			return err
		}
		if n, _ := result.RowsAffected(); n != 1 {
			return fail(403, "FORBIDDEN", "账号已停用")
		}
		if err = audit(tx, u.ID, "account.avatar.update", u.ID); err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		committed = true
		writeJSON(w, 200, map[string]string{"avatar_media_id": id})
		return nil
	})
	a.route("DELETE /api/v1/account/avatar", "read", func(w http.ResponseWriter, r *http.Request, u *User) error {
		tx, err := a.store.DB.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err = tx.Exec(`UPDATE users SET avatar_media_id=NULL WHERE id=?`, u.ID); err != nil {
			return err
		}
		if err = audit(tx, u.ID, "account.avatar.remove", u.ID); err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		return nil
	})
}
