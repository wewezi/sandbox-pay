package routes

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const staticSPAShell = "<!doctype html><html><head><title>Mintweft SPA fixture</title></head><body><div id=\"app\"></div></body></html>\n"

// The dist fixture deliberately has a single HTML entry. Canonical frontend
// routes must work without separate page files.
func newStaticSPAFixture(t *testing.T) http.Handler {
	t.Helper()
	root := t.TempDir()
	dist := filepath.Join(root, "dist")
	if err := os.MkdirAll(filepath.Join(dist, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"index.html":        staticSPAShell,
		"assets/styles.css": "body { color: #123456; }\n",
		"assets/app.js":     "document.documentElement.dataset.fixture = 'mintweft';\n",
	} {
		if err := os.WriteFile(filepath.Join(dist, filepath.FromSlash(name)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Synthetic content outside dist makes traversal checks independent of any
	// workspace files or host credentials.
	if err := os.WriteFile(filepath.Join(root, "outside.txt"), []byte("outside-dist-fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	return Static(os.DirFS(dist))
}

func TestStaticSPAKnownRoutes(t *testing.T) {
	handler := newStaticSPAFixture(t)
	paths := []string{"/"}
	for _, page := range []string{
		"dashboard", "orders", "notifications", "groups", "sandbox",
		"developer", "operations", "team", "login", "cashier",
	} {
		paths = append(paths, "/"+page)
	}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		for _, path := range paths {
			for _, query := range []string{
				"",
				"?orderNo=MW%2F2026%2B01&returnUrl=https%3A%2F%2Fmerchant.example%2Freceipt%3Fx%3D1&tag=first&tag=second",
			} {
				target := path + query
				t.Run(method+" "+target, func(t *testing.T) {
					rec := staticSPARequest(t, handler, method, target)
					if rec.Code != http.StatusOK {
						t.Fatalf("status = %d, want 200; body = %q", rec.Code, rec.Body.String())
					}
					staticSPAMediaType(t, rec, "text/html")
					if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
						t.Errorf("Cache-Control = %q, want no-cache", got)
					}
					if got := rec.Header().Get("Content-Length"); got != strconv.Itoa(len(staticSPAShell)) {
						t.Errorf("Content-Length = %q, want %d", got, len(staticSPAShell))
					}
					wantBody := staticSPAShell
					if method == http.MethodHead {
						wantBody = ""
					}
					if got := rec.Body.String(); got != wantBody {
						t.Errorf("body = %q, want %q", got, wantBody)
					}
				})
			}
		}
	}
}

func TestStaticSPAOldHTMLPathsStayJSON404(t *testing.T) {
	handler := newStaticSPAFixture(t)
	for _, page := range []string{
		"dashboard", "orders", "notifications", "groups", "sandbox",
		"developer", "operations", "team", "login", "cashier",
	} {
		path := "/" + page + ".html"
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			for _, query := range []string{"", "?orderNo=MW%2F2026%2B01&tag=first&tag=second"} {
				target := path + query
				t.Run(method+" "+target, func(t *testing.T) {
					rec := staticSPARequest(t, handler, method, target)
					staticSPAAssertNotFound(t, rec, method, path)
				})
			}
		}
	}
}

func TestStaticSPAUnknownPathsStayJSON404(t *testing.T) {
	handler := newStaticSPAFixture(t)
	for _, target := range []string{
		"/v1/api/typo",
		"/v1/api/admin/typo?orderNo=MW202601",
		"/api/missing",
		"/unknown",
		"/unknown.html",
		"/dashboard/child",
		"/dashboard/",
		"/dashboard.html/",
		"/orders.json",
		"/cashier.js",
		"/assets/missing.js",
		"/assets/missing.css?version=123",
		"/assets/index.html",
		"/missing.js",
		"/missing.css",
		"/favicon.ico",
	} {
		t.Run(target, func(t *testing.T) {
			rec := staticSPARequest(t, handler, http.MethodGet, target)
			staticSPAAssertNotFound(t, rec, http.MethodGet, httptest.NewRequest(http.MethodGet, target, nil).URL.Path)
		})
	}
}

func TestStaticSPARealAssets(t *testing.T) {
	handler := newStaticSPAFixture(t)
	for _, asset := range []struct {
		path       string
		body       string
		mediaTypes []string
	}{
		{"/assets/styles.css", "body { color: #123456; }\n", []string{"text/css"}},
		{"/assets/app.js", "document.documentElement.dataset.fixture = 'mintweft';\n", []string{"text/javascript", "application/javascript"}},
	} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			for _, query := range []string{"", "?version=20261101%2B1"} {
				target := asset.path + query
				t.Run(method+" "+target, func(t *testing.T) {
					rec := staticSPARequest(t, handler, method, target)
					if rec.Code != http.StatusOK {
						t.Fatalf("status = %d, want 200; body = %q", rec.Code, rec.Body.String())
					}
					staticSPAMediaType(t, rec, asset.mediaTypes...)
					if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
						t.Errorf("Cache-Control = %q, want no-cache", got)
					}
					if got := rec.Header().Get("Content-Length"); got != strconv.Itoa(len(asset.body)) {
						t.Errorf("Content-Length = %q, want %d", got, len(asset.body))
					}
					wantBody := asset.body
					if method == http.MethodHead {
						wantBody = ""
					}
					if got := rec.Body.String(); got != wantBody {
						t.Errorf("body = %q, want %q", got, wantBody)
					}
				})
			}
		}
	}
}

func TestStaticSPADirectoriesAndTraversalStayJSON404(t *testing.T) {
	handler := newStaticSPAFixture(t)
	for _, target := range []string{
		"/assets",
		"/assets/",
		"/assets/?list=true",
		"/assets/styles.css/",
		"/../outside.txt",
		"/assets/../../outside.txt",
		"/../index.html",
		"/assets/../index.html",
		"/assets/../dashboard",
		"/%2e%2e/outside.txt",
		"/assets/%2e%2e/index.html",
		"/assets%2f..%2findex.html",
		"/assets/%2e%2e/dashboard",
		"/assets/./app.js",
		"//dashboard",
		"/assets//app.js",
		"/..%5coutside.txt",
		"/assets%5c..%5cindex.html",
		"/assets/app.js%00",
	} {
		t.Run(target, func(t *testing.T) {
			rec := staticSPARequest(t, handler, http.MethodGet, target)
			staticSPAAssertNotFound(t, rec, http.MethodGet, httptest.NewRequest(http.MethodGet, target, nil).URL.Path)
		})
	}
}

func TestStaticSPANonReadMethodsStayJSON404(t *testing.T) {
	handler := newStaticSPAFixture(t)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions} {
		for _, path := range []string{"/", "/dashboard", "/cashier", "/assets/app.js"} {
			t.Run(method+" "+path, func(t *testing.T) {
				rec := staticSPARequest(t, handler, method, path)
				staticSPAAssertNotFound(t, rec, method, path)
			})
		}
	}
}

func staticSPARequest(t *testing.T, handler http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	beforeURL := *req.URL
	beforeRequestURI := req.RequestURI
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if *req.URL != beforeURL {
		t.Errorf("handler changed URL: got %q, want %q", req.URL.String(), beforeURL.String())
	}
	if req.RequestURI != beforeRequestURI {
		t.Errorf("handler changed RequestURI: got %q, want %q", req.RequestURI, beforeRequestURI)
	}
	if location := rec.Header().Get("Location"); location != "" {
		t.Errorf("unexpected redirect Location = %q", location)
	}
	return rec
}

func staticSPAMediaType(t *testing.T, rec *httptest.ResponseRecorder, allowed ...string) {
	t.Helper()
	got, _, err := mime.ParseMediaType(rec.Header().Get("Content-Type"))
	if err != nil {
		t.Fatalf("invalid Content-Type %q: %v", rec.Header().Get("Content-Type"), err)
	}
	for _, want := range allowed {
		if got == want {
			return
		}
	}
	t.Errorf("Content-Type = %q, want one of %v", got, allowed)
}

func staticSPAAssertNotFound(t *testing.T, rec *httptest.ResponseRecorder, method, path string) {
	t.Helper()
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %q", rec.Code, rec.Body.String())
	}
	staticSPAMediaType(t, rec, "application/json")
	var body struct {
		StatusCode int    `json:"statusCode"`
		Code       int    `json:"code"`
		Message    string `json:"message"`
		Msg        string `json:"msg"`
		Path       string `json:"path"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("404 body is not JSON: %v; body = %q", err, rec.Body.String())
	}
	if body.StatusCode != http.StatusNotFound || body.Code != http.StatusNotFound {
		t.Errorf("404 envelope statusCode/code = %d/%d", body.StatusCode, body.Code)
	}
	wantMessage := fmt.Sprintf("Cannot %s %s", method, path)
	if body.Message != wantMessage || body.Msg != wantMessage || body.Path != path {
		t.Errorf("404 envelope = %+v, want message/msg %q and path %q", body, wantMessage, path)
	}
	if strings.Contains(rec.Body.String(), staticSPAShell) || strings.Contains(rec.Body.String(), "outside-dist-fixture") {
		t.Fatal("404 response exposed shell or outside-dist content")
	}
}
