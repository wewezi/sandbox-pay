package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"sandbox-pay/internal/routes"
)

// Check the actual embedded bundle, independently of any working-directory files.
func TestEmbeddedFrontend(t *testing.T) {
	assets, err := fs.Sub(frontend, "web/dist")
	if err != nil {
		t.Fatal(err)
	}
	shell, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		t.Fatal(err)
	}
	handler := routes.Static(assets)
	scriptRE := regexp.MustCompile(`<script[^>]+src="([^"]+)"`)
	for _, path := range []string{"/", "/login", "/dashboard", "/orders", "/notifications", "/groups", "/sandbox", "/cashier", "/developer", "/operations", "/team"} {
		t.Run(path, func(t *testing.T) {
			r := httptest.NewRecorder()
			handler.ServeHTTP(r, httptest.NewRequest(http.MethodGet, path, nil))
			if r.Code != http.StatusOK {
				t.Fatalf("page status = %d", r.Code)
			}
			body := r.Body.String()
			if body != string(shell) {
				t.Fatal("frontend route did not serve the embedded app shell")
			}
			matches := scriptRE.FindAllStringSubmatch(body, -1)
			if len(matches) < 2 || !strings.Contains(body, `src="/theme.js"`) {
				t.Fatal("missing theme or bundled entry script")
			}
			for _, match := range matches {
				path := match[1]
				if strings.HasSuffix(path, ".ts") {
					t.Fatalf("uncompiled source in page: %s", path)
				}
				asset := httptest.NewRecorder()
				handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, path, nil))
				if asset.Code != http.StatusOK || asset.Body.Len() == 0 {
					t.Fatalf("script %s missing from embed: status=%d", path, asset.Code)
				}
			}
		})
	}
}
