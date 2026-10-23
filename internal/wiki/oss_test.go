package wiki

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestOSSIntentSignsLengthAndPromotesWithConditionalCopy(t *testing.T) {
	type object struct {
		data       []byte
		kind, etag string
	}
	objects := map[string]object{}
	var mu sync.Mutex
	copies := 0
	conditional := false
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		key := strings.TrimPrefix(r.URL.Path, "/test-bucket/")
		switch r.Method {
		case "PUT":
			if source := r.Header.Get("X-Oss-Copy-Source"); source != "" {
				source, _ = url.PathUnescape(source)
				o, ok := objects[strings.TrimPrefix(source, "/test-bucket/")]
				if !ok {
					http.NotFound(w, r)
					return
				}
				conditional = r.Header.Get("X-Oss-Copy-Source-If-Match") == o.etag && r.Header.Get("X-Oss-Object-Acl") == "private"
				if !conditional {
					w.WriteHeader(412)
					return
				}
				objects[key] = o
				copies++
				w.Header().Set("Content-Type", "application/xml")
				xml.NewEncoder(w).Encode(struct {
					XMLName xml.Name `xml:"CopyObjectResult"`
					ETag    string   `xml:"ETag"`
				}{ETag: o.etag})
				return
			}
			b, _ := io.ReadAll(r.Body)
			sum := md5.Sum(b)
			etag := `"` + hex.EncodeToString(sum[:]) + `"`
			objects[key] = object{b, r.Header.Get("Content-Type"), etag}
			w.Header().Set("ETag", etag)
			w.WriteHeader(200)
		case "HEAD", "GET":
			if strings.Contains(r.Header.Get("Authorization"), "AdditionalHeaders=content-length") && r.Header.Get("Content-Length") == "" {
				http.Error(w, "signed content-length is missing on the wire", 403)
				return
			}
			o, ok := objects[key]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", o.kind)
			w.Header().Set("Content-Length", strconv.Itoa(len(o.data)))
			w.Header().Set("ETag", o.etag)
			if r.Method == "GET" {
				w.Write(o.data)
			}
		case "DELETE":
			delete(objects, key)
			w.WriteHeader(204)
		default:
			w.WriteHeader(405)
		}
	}))
	defer cloud.Close()
	a, c := newTestApp(t)
	driver, e := newOSS(OSSConfig{Endpoint: cloud.URL, Region: "cn-test", Bucket: "test-bucket", AccessKeyID: "test-access-id", AccessKeySecret: "test-access-secret"})
	if e != nil {
		t.Fatal(e)
	}
	a.oss = driver
	var data bytes.Buffer
	png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 3, 3)))
	intent := c.call("POST", "/api/v1/media/uploads", map[string]any{"name": "test.png", "content_type": "image/png", "size": data.Len()}, 201)
	signed, e := url.Parse(intent["url"].(string))
	if e != nil {
		t.Fatal(e)
	}
	if signed.Query().Get("x-oss-additional-headers") != "content-length" {
		t.Fatalf("upload length is not signed: %s", signed.Query().Get("x-oss-additional-headers"))
	}
	req, e := http.NewRequest("PUT", signed.String(), bytes.NewReader(data.Bytes()))
	if e != nil {
		t.Fatal(e)
	}
	req.Header.Set("Content-Type", "image/png")
	response, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("upload fixture failed")
	}
	mid := intent["media"].(map[string]any)["id"].(string)
	c.call("POST", "/api/v1/media/uploads/"+mid+"/complete", map[string]any{}, 200)
	c.call("POST", "/api/v1/media/uploads/"+mid+"/complete", map[string]any{}, 200)
	m, e := a.media(mid)
	if e != nil {
		t.Fatal(e)
	}
	mu.Lock()
	defer mu.Unlock()
	if copies != 1 || !conditional || strings.HasPrefix(m.Key, "pending/") || !bytes.Equal(objects[m.Key].data, data.Bytes()) {
		t.Fatal("OSS finalization did not promote an immutable, verified object exactly once")
	}
}
