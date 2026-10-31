package httpx

import (
	"encoding/json"
	"net/http"
)

// envelope 统一响应信封，形状为 { code, msg, message, data }。
// 前端 web/app.ts 同时检查 HTTP 状态与 code，从 data 取业务数据，
// 优先读取 message，其次读取 msg 作为错误文案；改结构时同步更新前端。
type envelope struct {
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// pickMsg 取 data 里 message/msg 的字符串值，都没有就用 "success"。
func pickMsg(data any) string {
	if m, ok := data.(map[string]any); ok {
		if s, ok := m["message"].(string); ok {
			return s
		}
		if s, ok := m["msg"].(string); ok {
			return s
		}
	}
	return "success"
}

// OK 包成信封返回。data 为 nil 时落成 null，前端拿到的是明确的空值。
// 调用前想 Set-Cookie 之类就先写 header，写完再调这里。
func OK(w http.ResponseWriter, data any) {
	msg := pickMsg(data)
	writeJSON(w, http.StatusOK, envelope{Code: 200, Msg: msg, Message: msg, Data: data})
}

// writeJSON 原样写一个 JSON 响应（错误信封也走它）。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
