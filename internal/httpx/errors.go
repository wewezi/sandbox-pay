package httpx

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"

	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/logger"
)

var errorLogger = logger.New("ExceptionFilter")

// Handler 返回 error 即进入统一错误信封。
type Handler func(w http.ResponseWriter, r *http.Request) error

// H 把 Handler 适配成 http.HandlerFunc，顺带兜住 panic ——
// 未捕获的 panic 和抛错走同一套信封，不把调用栈吐给客户端。
func H(h Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				errorLogger.Error(
					"Http Status: 500 Error Message: "+strconv.Quote(panicText(rec)),
					string(debug.Stack()),
				)
				writeErrorBody(w, r, http.StatusInternalServerError, "Internal server error")
			}
		}()
		if err := h(w, r); err != nil {
			writeError(w, r, err)
		}
	}
}

func panicText(v any) string {
	switch t := v.(type) {
	case error:
		return t.Error()
	case string:
		return t
	default:
		return fmt.Sprintf("%v", v)
	}
}

// writeError 全局错误处理。
//
// 日志里留真实错误（含 500 的原始 message），但响应里 500 只回笼统文案。
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	message := "Internal server error"
	if he := httperr.As(err); he != nil {
		status = he.Status
		message = he.Message
	}

	stack := ""
	if status >= http.StatusInternalServerError {
		stack = err.Error()
	}
	errorLogger.Error(
		"Http Status: "+strconv.Itoa(status)+" Error Message: "+strconv.Quote(err.Error()),
		stack,
	)
	writeErrorBody(w, r, status, message)
}

// writeErrorBody 统一错误信封的 JSON 形状。
// message 和 msg 都给 —— 前端 pickMessage 两个字段都看。
func writeErrorBody(w http.ResponseWriter, r *http.Request, status int, message string) {
	bindUsageError(r.Context(), status, message)
	writeJSON(w, status, map[string]any{
		"statusCode":    status,
		"code":          status,
		"timestamp":     time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		"path":          r.URL.Path,
		"message":       message,
		"msg":           message,
		"correlationId": CorrelationID(r.Context()),
	})
}

// WriteNotFound 404：统一成同一套信封。
func WriteNotFound(w http.ResponseWriter, r *http.Request) {
	writeErrorBody(w, r, http.StatusNotFound, "Cannot "+r.Method+" "+r.URL.Path)
}

// WriteMethodNotAllowed 路径存在，但不支持请求方法。
func WriteMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	writeErrorBody(w, r, http.StatusMethodNotAllowed, "Method not allowed")
}
