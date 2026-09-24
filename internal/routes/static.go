package routes

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"

	"sandbox-pay/internal/httpx"
)

// Static 前端静态资源处理器（public/ 下的纯 HTML/CSS/JS）。
//
// no-cache 是「用前先回源验证」，不是「别存」。缺了这个头，浏览器只看到
// 文件的 last-modified 就按启发式自己定过期（大致是距上次修改时间的 10%），
// 几天没动的 css 能被缓存很久 —— 改完刷新根本看不到新的。
//
// 不处理 If-Modified-Since，每次都是完整 200 换不来 304。自用后台就这
// 几十 KB，不值得自己接条件请求。
//
// 找不到文件时走统一信封 404 —— 这是 Hono 的 notFound 在 Go 侧的对应物，
// 否则 /v1/api/typo 会回 "404 page not found" 纯文本，前端解析不出来。
func Static(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeStatic404(w, r)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		f, err := fsys.Open(name)
		if err != nil {
			writeStatic404(w, r)
			return
		}
		_ = f.Close()

		w.Header().Set("Cache-Control", "no-cache")
		fileServer.ServeHTTP(w, r)
	})
}

// writeStatic404 统一的 404 信封，直接用 httpx 那一套，
// 保证静态资源 404 和 API 404 的形状完全一致。
func writeStatic404(w http.ResponseWriter, r *http.Request) {
	httpx.WriteNotFound(w, r)
}

// DiskFS 用给定目录当静态资源根（开发时直接改 public/ 就生效，
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
