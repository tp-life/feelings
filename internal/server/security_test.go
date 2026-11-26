package server_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"feelings-studio/internal/server"
	"feelings-studio/internal/studio"
)

func newTestServer(t *testing.T) (*studio.Service, http.Handler) {
	t.Helper()
	tmp := t.TempDir()
	svc, err := studio.Open(tmp, filepath.Join(tmp, "configs"))
	if err != nil {
		t.Fatalf("studio.Open: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc, server.New(svc).Handler()
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMediaEndpointOnlyServesWorkspaceMedia(t *testing.T) {
	svc, h := newTestServer(t)
	root := svc.RootDir

	writeFile(t, filepath.Join(root, ".feelings", "providers.yaml"), "api_key: sk-secret")
	writeFile(t, filepath.Join(root, ".feelings", "leak.jpeg"), "secret-in-hidden-dir")
	writeFile(t, filepath.Join(root, "projects", "p1", "project.yaml"), "id: p1")
	writeFile(t, filepath.Join(root, "projects", "p1", "output", "frames", "g1.jpeg"), "img")
	writeFile(t, filepath.Join(root, "g1.jpeg"), "imported-frame")
	outside := filepath.Join(t.TempDir(), "outside.jpeg")
	writeFile(t, outside, "outside")
	if err := os.Symlink(outside, filepath.Join(root, "projects", "p1", "link.jpeg")); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		path string
		want int
	}{
		{filepath.Join(root, "projects", "p1", "output", "frames", "g1.jpeg"), http.StatusOK},
		{filepath.Join(root, "g1.jpeg"), http.StatusOK},
		{"projects/p1/output/frames/g1.jpeg", http.StatusOK},
		{filepath.Join(root, ".feelings", "providers.yaml"), http.StatusForbidden},
		{filepath.Join(root, ".feelings", "leak.jpeg"), http.StatusForbidden},
		{filepath.Join(root, "projects", "p1", "project.yaml"), http.StatusForbidden},
		{filepath.Join(root, ".cache", "state.db"), http.StatusForbidden},
		{outside, http.StatusForbidden},
		{filepath.Join(root, "projects", "p1", "link.jpeg"), http.StatusForbidden},
		{filepath.Join(root, "projects", "..", "..", "etc", "passwd"), http.StatusForbidden},
	}
	for _, c := range cases {
		w := doReq(h, http.MethodGet, "/api/media?path="+c.path, "")
		if w.Code != c.want {
			t.Errorf("GET /api/media?path=%s: status=%d, want %d", c.path, w.Code, c.want)
		}
		if strings.Contains(w.Body.String(), "sk-secret") || strings.Contains(w.Body.String(), "secret-in-hidden-dir") {
			t.Fatalf("secret leaked via %s", c.path)
		}
	}
}

func TestEncodedProjectIDTraversalIsRejected(t *testing.T) {
	svc, h := newTestServer(t)
	parent := filepath.Dir(svc.RootDir)

	for _, id := range []string{"..%2F..%2Fevil", "%2E%2E", "..%2Fx", ".hidden"} {
		w := doReq(h, http.MethodPut, "/api/projects/"+id, `{"title":"x","track":"childhood_90s","style_preset":"film_90s"}`)
		if w.Code < 400 {
			t.Errorf("PUT /api/projects/%s: status=%d, want 4xx/5xx", id, w.Code)
		}
	}
	if _, err := os.Stat(filepath.Join(parent, "evil")); err == nil {
		t.Fatalf("traversal created a directory outside the workspace")
	}
	if _, err := os.Stat(filepath.Join(svc.RootDir, "project.yaml")); err == nil {
		t.Fatalf("traversal wrote project.yaml at the workspace root")
	}

	w := doReq(h, http.MethodPut, "/api/templates", `{"name":"prompts/story_outline.tmpl","content":"x","project_id":"../../evil","scope":"project"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("template save with traversal project id: status=%d, want 400", w.Code)
	}
}

func TestLocalOnlyGuard(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := server.LocalOnly(ok)

	cases := []struct {
		name    string
		method  string
		host    string
		headers map[string]string
		want    int
	}{
		{"loopback get", http.MethodGet, "127.0.0.1:8080", nil, http.StatusNoContent},
		{"localhost get", http.MethodGet, "localhost:8080", nil, http.StatusNoContent},
		{"ipv6 loopback", http.MethodGet, "[::1]:8080", nil, http.StatusNoContent},
		{"dns rebinding host", http.MethodGet, "evil.example:8080", nil, http.StatusForbidden},
		{"curl post without origin", http.MethodPost, "127.0.0.1:8080", nil, http.StatusNoContent},
		{"same origin post", http.MethodPost, "127.0.0.1:8080", map[string]string{"Origin": "http://127.0.0.1:8080", "Sec-Fetch-Site": "same-origin"}, http.StatusNoContent},
		{"cross origin post", http.MethodPost, "127.0.0.1:8080", map[string]string{"Origin": "https://evil.example"}, http.StatusForbidden},
		{"cross site delete", http.MethodDelete, "127.0.0.1:8080", map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"other local port", http.MethodPost, "127.0.0.1:8080", map[string]string{"Origin": "http://127.0.0.1:3000"}, http.StatusForbidden},
	}
	for _, c := range cases {
		r := httptest.NewRequest(c.method, "/api/projects/x", nil)
		r.Host = c.host
		for k, v := range c.headers {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%s: status=%d, want %d", c.name, w.Code, c.want)
		}
	}
}
