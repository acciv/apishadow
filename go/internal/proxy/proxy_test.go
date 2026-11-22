package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/acciv/apishadow/internal/capture"
)

type chanWriter struct {
	ch chan string
}

func (w chanWriter) Write(p []byte) (int, error) {
	w.ch <- string(p)
	return len(p), nil
}

func TestMirrorAndCapture(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"version":"1.0"}`)
	}))
	defer primary.Close()

	shadowHit := make(chan string, 1)
	shadow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		shadowHit <- r.Header.Get("X-ApiShadow-Role")
		io.WriteString(w, `{"version":"2.0","debug":true}`)
	}))
	defer shadow.Close()

	lines := make(chan string, 4)
	writer := capture.NewWriter(chanWriter{ch: lines})
	p := New(primary.URL, shadow.URL, writer, 1.0, 2*time.Second)

	req := httptest.NewRequest(http.MethodPost, "/v1/users?x=1", strings.NewReader(`{"q":1}`))
	rec := httptest.NewRecorder()
	p.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"version":"1.0"`) {
		t.Fatalf("primary response not returned: %s", rec.Body.String())
	}
	select {
	case role := <-shadowHit:
		if role != "shadow" {
			t.Fatalf("shadow role header: %q", role)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shadow was not called")
	}

	roles := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case line := <-lines:
			if strings.Contains(line, `"role":"primary"`) {
				roles["primary"] = true
			}
			if strings.Contains(line, `"role":"shadow"`) {
				roles["shadow"] = true
			}
		case <-time.After(2 * time.Second):
			t.Fatal("capture records missing")
		}
	}
	if !roles["primary"] || !roles["shadow"] {
		t.Fatalf("missing roles: %+v", roles)
	}
}

func TestHealthz(t *testing.T) {
	p := New("http://a", "http://b", nil, 1, time.Second)
	rec := httptest.NewRecorder()
	p.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("bad health: %d %s", rec.Code, rec.Body.String())
	}
}
