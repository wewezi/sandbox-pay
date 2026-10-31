package routes

import (
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"

	"sandbox-pay/internal/httpx"
)

// Static serves the single app shell for known frontend routes and serves
// actual files for assets. Unknown paths keep the API's JSON 404 envelope.
//
// no-cache 是「用前先回源验证」，不是「别存」。缺了这个头，浏览器只看到
// 文件的 last-modified 就按启发式自己定过期（大致是距上次修改时间的 10%），
// 几天没动的 css 能被缓存很久 —— 改完刷新根本看不到新的。
//
// 找不到文件时返回统一的 JSON 404，供前端读取错误信息。
func Static(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeStatic404(w, r)
			return
		}
		requestPath := r.URL.Path
		if path.Clean(requestPath) != requestPath || strings.Contains(requestPath, "\\") ||
			requestPath == "/v1/api" || strings.HasPrefix(requestPath, "/v1/api/") {
			writeStatic404(w, r)
			return
		}
		if isFrontendRoute(requestPath) {
			serveAppShell(w, r, fsys)
			return
		}
		name := strings.TrimPrefix(requestPath, "/")
		f, err := fsys.Open(name)
		if err != nil {
			writeStatic404(w, r)
			return
		}
		info, err := f.Stat()
		_ = f.Close()
		if err != nil || info.IsDir() {
			writeStatic404(w, r)
			return
		}

		w.Header().Set("Cache-Control", "no-cache")
		fileServer.ServeHTTP(w, r)
	})
}

func isFrontendRoute(requestPath string) bool {
	switch requestPath {
	case "/", "/dashboard", "/orders", "/notifications", "/groups",
		"/sandbox", "/developer", "/operations", "/team", "/login", "/cashier":
		return true
	default:
		return false
	}
}

func serveAppShell(w http.ResponseWriter, r *http.Request, fsys fs.FS) {
	f, err := fsys.Open("index.html")
	if err != nil {
		writeStatic404(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		writeStatic404(w, r)
		return
	}
	content, err := io.ReadAll(f)
	if err != nil {
		writeStatic404(w, r)
		return
	}
	// ServeContent accepts the original request, including its query, headers,
	// and context.
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "index.html", info.ModTime(), bytes.NewReader(content))
}

// writeStatic404 统一的 404 信封，直接用 httpx 那一套，
// 保证静态资源 404 和 API 404 的形状完全一致。
func writeStatic404(w http.ResponseWriter, r *http.Request) {
	httpx.WriteNotFound(w, r)
}

// DiskFS 用给定目录当静态资源根（开发时重新构建 web/dist 即生效，
// 不必重编译）。目录不存在返回 nil，由调用方回落到 embed。
func DiskFS(dir string) fs.FS {
	fsys := os.DirFS(dir)
	f, err := fsys.Open(".")
	if err != nil {
		return nil
	}
	_ = f.Close()
	return fsys
}

// SubFS 从 embed.FS 里取子目录。
func SubFS(efs fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(efs, dir)
	if err != nil {
		return nil
	}
	return sub
}
