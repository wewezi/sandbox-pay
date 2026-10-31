package routes

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"sandbox-pay/internal/apiurl"
	"sandbox-pay/internal/config"
	"sandbox-pay/internal/httpx"
)

//go:embed openapi.json
var openAPIDocument []byte

//go:embed developer_example.mjs
var developerExample []byte

func (a *App) registerDeveloper(r chi.Router) {
	r.Get("/openapi.json", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, err := w.Write(openAPIDocument)
		return err
	}))
	r.Get("/developer/example", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="mintweft-demo.mjs"`)
		_, err := w.Write(developerExample)
		return err
	}))
}

func (a *App) registerAdminDeveloper(r chi.Router) {
	r.Get("/developer", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		var document map[string]any
		if err := json.Unmarshal(openAPIDocument, &document); err != nil {
			return err
		}
		base := strings.TrimRight(httpx.BaseURL(r), "/")
		if fixed := strings.TrimRight(strings.TrimSpace(a.cfg.Get("CLIENT_URL")), "/"); fixed != "" {
			base = fixed
		}
		httpx.OK(w, map[string]any{"baseUrl": base, "sandboxEnabled": config.Bool(a.cfg, "ENABLE_SANDBOX"), "openapiUrl": apiurl.Prefix + "/openapi.json", "exampleUrl": apiurl.Prefix + "/developer/example", "openapi": document})
		return nil
	}))
}
