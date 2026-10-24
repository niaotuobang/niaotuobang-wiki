package wiki

import (
	"net/http/httptest"
	"testing"
)

func TestPublicReadSettingCannotBypassLogin(t *testing.T) {
	a, _ := newTestApp(t)
	if _, e := a.store.DB.Exec(`INSERT INTO settings(key,value) VALUES('public_read','true') ON CONFLICT(key) DO UPDATE SET value='true'`); e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"/api/v1/articles", "/api/v1/tags", "/api/v1/timeline", "/api/v1/articles/known/versions", "/api/v1/articles/known/backlinks", "/media/known"} {
		for _, method := range []string{"GET", "HEAD"} {
			w := httptest.NewRecorder()
			a.Handler().ServeHTTP(w, httptest.NewRequest(method, path, nil))
			if w.Code != 401 {
				t.Errorf("%s %s anonymous status=%d; want 401", method, path, w.Code)
			}
		}
	}
}
