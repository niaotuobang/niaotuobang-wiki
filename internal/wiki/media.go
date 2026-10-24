package wiki

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	_ "golang.org/x/image/webp"
)

type OSSConfig struct{ Endpoint, Region, Bucket, AccessKeyID, AccessKeySecret, SecurityToken string }
type OSSStorage struct{ bucket, uploads *oss.Bucket }

func newOSS(c OSSConfig) (*OSSStorage, error) {
	if c.Region == "" || c.Bucket == "" || c.AccessKeyID == "" || c.AccessKeySecret == "" {
		return nil, fmt.Errorf("OSS region, bucket and server credentials are required")
	}
	endpoint, e := url.Parse(c.Endpoint)
	if e != nil || endpoint.Host == "" || endpoint.Scheme != "https" && endpoint.Scheme != "http" {
		return nil, fmt.Errorf("invalid OSS endpoint")
	}
	// Use Go's standard proxy-aware transport, including HTTPS_PROXY and NO_PROXY.
	// The SDK default transport ignores these and can fail server-side HEAD after
	// the browser has already successfully uploaded the object.
	clientHTTP := &http.Client{Transport: http.DefaultTransport, Timeout: 2 * time.Minute, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	opts := []oss.ClientOption{oss.Timeout(15, 120), oss.Region(c.Region), oss.AuthVersion(oss.AuthV4), oss.HTTPClient(clientHTTP)}
	if c.SecurityToken != "" {
		opts = append(opts, oss.SecurityToken(c.SecurityToken))
	}
	client, e := oss.New(c.Endpoint, c.AccessKeyID, c.AccessKeySecret, opts...)
	if e != nil {
		return nil, e
	}
	b, e := client.Bucket(c.Bucket)
	if e != nil {
		return nil, e
	}
	// Only upload presigning binds Content-Length. GET/HEAD must not sign a
	// synthetic zero-length header that Go's transport removes on the wire.
	uploadClient, e := oss.New(c.Endpoint, c.AccessKeyID, c.AccessKeySecret, append(opts, oss.AdditionalHeaders([]string{"content-length"}))...)
	if e != nil {
		return nil, e
	}
	uploads, e := uploadClient.Bucket(c.Bucket)
	if e != nil {
		return nil, e
	}
	return &OSSStorage{bucket: b, uploads: uploads}, nil
}
func (a *App) storageName() string {
	if a.oss != nil {
		return "oss"
	}
	return "local"
}

type Media struct {
	ID          string `json:"id"`
	Owner       string `json:"-"`
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	State       string `json:"state"`
	Key         string `json:"-"`
	CreatedAt   string `json:"created_at"`
	Checksum    string `json:"checksum"`
}

func (a *App) media(id string) (Media, error) {
	var m Media
	e := a.store.DB.QueryRow(`SELECT id,owner_id,name,content_type,size,state,object_key,created_at,checksum FROM media WHERE id=?`, id).Scan(&m.ID, &m.Owner, &m.Name, &m.ContentType, &m.Size, &m.State, &m.Key, &m.CreatedAt, &m.Checksum)
	if e == sql.ErrNoRows {
		return m, fail(404, "NOT_FOUND", "媒体不存在")
	}
	return m, e
}
func (a *App) mediaReadable(m Media, u *User) bool {
	if u != nil && m.Owner == u.ID {
		return true
	}
	var n int
	e := a.store.DB.QueryRow(`SELECT count(*) FROM article_media am JOIN articles a ON a.id=am.article_id WHERE am.media_id=? AND a.status='published' AND a.deleted_at=''`, m.ID).Scan(&n)
	return e == nil && n > 0
}

var mediaTypes = map[string]int64{"image/jpeg": 20 << 20, "image/png": 20 << 20, "image/webp": 20 << 20, "image/gif": 20 << 20, "video/mp4": 200 << 20, "video/webm": 200 << 20, "video/quicktime": 200 << 20}

func inspectMedia(b []byte, kind string) error {
	if strings.HasPrefix(kind, "image/") {
		cfg, format, e := image.DecodeConfig(bytes.NewReader(b))
		expected := map[string]string{"image/jpeg": "jpeg", "image/png": "png", "image/gif": "gif", "image/webp": "webp"}[kind]
		if e != nil || format != expected || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 24_000_000 {
			return fail(400, "VALIDATION_ERROR", "图片无效或超过 2400 万像素")
		}
		return nil
	}
	if kind == "video/webm" {
		if len(b) < 4 || !bytes.Equal(b[:4], []byte{0x1a, 0x45, 0xdf, 0xa3}) {
			return fail(400, "VALIDATION_ERROR", "视频内容与 WebM 格式不符")
		}
		return nil
	}
	if len(b) < 12 || string(b[4:8]) != "ftyp" {
		return fail(400, "VALIDATION_ERROR", "视频需要有效的 MP4 或 MOV 容器")
	}
	return nil
}
func (a *App) mediaRoutes() {
	a.route("POST /api/v1/media/uploads", "edit", func(w http.ResponseWriter, r *http.Request, u *User) error {
		var b struct {
			Name        string `json:"name"`
			ContentType string `json:"content_type"`
			Size        int64  `json:"size"`
		}
		if e := decode(w, r, &b); e != nil {
			return e
		}
		limit, ok := mediaTypes[b.ContentType]
		if !ok {
			return fail(400, "VALIDATION_ERROR", "支持 JPEG、PNG、WebP、GIF、MP4、WebM 与 MOV")
		}
		if b.Size <= 0 || b.Size > limit {
			return fail(413, "UPLOAD_TOO_LARGE", "图片上限 20 MiB，视频上限 200 MiB")
		}
		if len(b.Name) > 300 {
			return fail(400, "VALIDATION_ERROR", "文件名过长")
		}
		var pending int
		var used int64
		if e := a.store.DB.QueryRow(`SELECT count(*),coalesce(sum(size),0) FROM media WHERE owner_id=? AND state IN ('pending','uploaded')`, u.ID).Scan(&pending, &used); e != nil {
			return e
		}
		if pending >= 20 || used+b.Size > 1<<30 {
			return fail(429, "RATE_LIMITED", "未完成上传过多，请先完成或取消已有上传")
		}
		m := Media{ID: newID(), Owner: u.ID, Name: filepath.Base(b.Name), ContentType: b.ContentType, Size: b.Size, State: "pending", CreatedAt: now()}
		m.Key = "pending/" + m.ID
		uploadURL := "/api/v1/media/uploads/" + m.ID + "/content"
		headers := map[string]string{"Content-Type": m.ContentType}
		if a.oss != nil {
			var e error
			uploadURL, e = a.oss.uploads.SignURL(m.Key, oss.HTTPPut, 900, oss.ContentType(m.ContentType), oss.ContentLength(m.Size))
			if e != nil {
				return e
			}
		}
		if _, e := a.store.DB.Exec(`INSERT INTO media(id,owner_id,name,content_type,size,state,object_key,created_at) VALUES(?,?,?,?,?,?,?,?)`, m.ID, m.Owner, m.Name, m.ContentType, m.Size, m.State, m.Key, m.CreatedAt); e != nil {
			return e
		}
		writeJSON(w, 201, map[string]any{"media": m, "method": "PUT", "url": uploadURL, "headers": headers, "storage": a.storageName(), "expires_in": 900})
		return nil
	})
	a.route("PUT /api/v1/media/uploads/{id}/content", "edit", func(w http.ResponseWriter, r *http.Request, u *User) error {
		if a.oss != nil {
			return fail(400, "VALIDATION_ERROR", "此存储驱动需要直接上传")
		}
		a.uploadMu.Lock()
		defer a.uploadMu.Unlock()
		m, e := a.media(r.PathValue("id"))
		if e != nil {
			return e
		}
		if m.Owner != u.ID {
			return fail(403, "FORBIDDEN", "不能上传到他人的媒体记录")
		}
		if m.State != "pending" {
			return fail(409, "MEDIA_NOT_READY", "上传已完成或已取消")
		}
		if e := checkUploadAge(m); e != nil {
			return e
		}
		dir := filepath.Join(a.store.DataDir, "pending")
		if e = os.MkdirAll(dir, 0700); e != nil {
			return e
		}
		f, e := os.CreateTemp(dir, "upload-")
		if e != nil {
			return e
		}
		defer os.Remove(f.Name())
		defer f.Close()
		hash := sha256.New()
		r.Body = http.MaxBytesReader(w, r.Body, m.Size+1)
		n, e := io.Copy(io.MultiWriter(f, hash), r.Body)
		if e != nil || n != m.Size {
			return fail(400, "VALIDATION_ERROR", "上传大小不符，请重新上传")
		}
		if e = f.Close(); e != nil {
			return e
		}
		target := filepath.Join(a.store.DataDir, m.Key)
		if e = os.Rename(f.Name(), target); e != nil {
			return e
		}
		if _, e = a.store.DB.Exec(`UPDATE media SET state='uploaded',checksum=? WHERE id=?`, hex.EncodeToString(hash.Sum(nil)), m.ID); e != nil {
			return e
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		return nil
	})
	a.route("POST /api/v1/media/uploads/{id}/complete", "edit", func(w http.ResponseWriter, r *http.Request, u *User) error {
		a.uploadMu.Lock()
		defer a.uploadMu.Unlock()
		m, e := a.media(r.PathValue("id"))
		if e != nil {
			return e
		}
		if m.Owner != u.ID {
			return fail(403, "FORBIDDEN", "不能认领他人的上传")
		}
		if m.State == "ready" {
			writeJSON(w, 200, m)
			return nil
		}
		if m.State == "cancelled" {
			return fail(409, "MEDIA_NOT_READY", "上传已取消")
		}
		if e := checkUploadAge(m); e != nil {
			return e
		}
		finalKey := "media/" + newID()
		var reader io.ReadCloser
		etag := ""
		if a.oss != nil {
			meta, e := a.oss.bucket.GetObjectDetailedMeta(m.Key)
			if e != nil {
				return fail(400, "MEDIA_NOT_READY", "文件尚未上传完成")
			}
			size, _ := strconv.ParseInt(meta.Get("Content-Length"), 10, 64)
			if size != m.Size || meta.Get("Content-Type") != m.ContentType {
				return fail(400, "VALIDATION_ERROR", "对象大小或类型不符")
			}
			etag = meta.Get("ETag")
			reader, e = a.oss.bucket.GetObject(m.Key, oss.Range(0, 65535), oss.IfMatch(etag))
			if e != nil {
				return e
			}
		} else {
			if m.State != "uploaded" {
				return fail(400, "MEDIA_NOT_READY", "文件尚未上传完成")
			}
			reader, e = os.Open(filepath.Join(a.store.DataDir, m.Key))
			if e != nil {
				return e
			}
		}
		prefix, e := io.ReadAll(io.LimitReader(reader, 65536))
		reader.Close()
		if e != nil {
			return e
		}
		if e = inspectMedia(prefix, m.ContentType); e != nil {
			return e
		}
		if a.oss != nil {
			_, e = a.oss.bucket.CopyObject(m.Key, finalKey, oss.CopySourceIfMatch(etag), oss.ObjectACL(oss.ACLPrivate))
			m.Checksum = etag
		} else {
			e = os.MkdirAll(filepath.Join(a.store.DataDir, "media"), 0700)
			if e == nil {
				e = os.Rename(filepath.Join(a.store.DataDir, m.Key), filepath.Join(a.store.DataDir, finalKey))
			}
		}
		if e != nil {
			return e
		}
		if _, e = a.store.DB.Exec(`UPDATE media SET state='ready',object_key=?,checksum=? WHERE id=?`, finalKey, m.Checksum, m.ID); e != nil {
			return e
		}
		if a.oss != nil {
			a.oss.bucket.DeleteObject(m.Key)
		}
		m.State = "ready"
		m.Key = finalKey
		writeJSON(w, 200, m)
		return nil
	})
	a.route("DELETE /api/v1/media/uploads/{id}", "edit", func(w http.ResponseWriter, r *http.Request, u *User) error {
		a.uploadMu.Lock()
		defer a.uploadMu.Unlock()
		m, e := a.media(r.PathValue("id"))
		if e != nil {
			return e
		}
		if m.Owner != u.ID {
			return fail(403, "FORBIDDEN", "不能取消他人的上传")
		}
		if m.State == "ready" {
			return fail(409, "MEDIA_NOT_READY", "已完成的媒体不能作为临时上传取消")
		}
		if _, e = a.store.DB.Exec(`UPDATE media SET state='cancelled' WHERE id=?`, m.ID); e != nil {
			return e
		}
		if a.oss != nil {
			a.oss.bucket.DeleteObject(m.Key)
		} else {
			os.Remove(filepath.Join(a.store.DataDir, m.Key))
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		return nil
	})
	a.route("GET /api/v1/media/{id}", "read", func(w http.ResponseWriter, r *http.Request, u *User) error {
		m, e := a.media(r.PathValue("id"))
		if e != nil {
			return e
		}
		if !a.mediaReadable(m, u) {
			return fail(404, "NOT_FOUND", "媒体不存在")
		}
		writeJSON(w, 200, map[string]any{"media": m, "url": "/media/" + m.ID})
		return nil
	})
	a.route("GET /media/{id}", "read", func(w http.ResponseWriter, r *http.Request, u *User) error {
		m, e := a.media(r.PathValue("id"))
		if e != nil {
			return e
		}
		if m.State != "ready" || !a.mediaReadable(m, u) {
			return fail(404, "NOT_FOUND", "媒体不存在")
		}
		w.Header().Set("Cache-Control", "private, no-store")
		if a.oss != nil {
			return a.proxyOSSMedia(w, r, m)
		}

		f, e := os.Open(filepath.Join(a.store.DataDir, m.Key))
		if e != nil {
			return e
		}
		defer f.Close()
		info, e := f.Stat()
		if e != nil {
			return e
		}
		w.Header().Set("Content-Type", m.ContentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", m.ID))
		http.ServeContent(w, r, m.ID, info.ModTime(), f)
		return nil
	})
}
func checkUploadAge(m Media) error {
	t, e := time.Parse(time.RFC3339Nano, m.CreatedAt)
	if e != nil || time.Since(t) > time.Hour {
		return fail(410, "UPLOAD_EXPIRED", "上传已过期，请重新选择文件")
	}
	return nil
}

// proxyOSSMedia keeps authorization on every GET/HEAD/Range request. Never
// redirect a reader to a reusable signed object URL.
func (a *App) proxyOSSMedia(w http.ResponseWriter, r *http.Request, m Media) error {
	signed, e := a.oss.bucket.SignURL(m.Key, oss.HTTPGet, 60)
	if e != nil {
		return e
	}
	req, e := http.NewRequestWithContext(r.Context(), r.Method, signed, nil)
	if e != nil {
		return e
	}
	if r.Method == http.MethodHead {
		signed, e = a.oss.bucket.SignURL(m.Key, oss.HTTPHead, 60)
		if e != nil {
			return e
		}
		req.URL, e = url.Parse(signed)
		if e != nil {
			return e
		}
	}
	for _, h := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"} {
		if v := r.Header.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}
	client := &http.Client{Timeout: 5 * time.Minute, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	response, e := client.Do(req)
	if e != nil {
		return fail(502, "MEDIA_UNAVAILABLE", "媒体暂时不可用")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 && response.StatusCode != 206 && response.StatusCode != 304 && response.StatusCode != 416 {
		return fail(502, "MEDIA_UNAVAILABLE", "媒体暂时不可用")
	}
	for _, h := range []string{"Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
		if v := response.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.Header().Set("Content-Type", m.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", m.ID))
	w.WriteHeader(response.StatusCode)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, io.LimitReader(response.Body, m.Size+1))
	}
	return nil
}
