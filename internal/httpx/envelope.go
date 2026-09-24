package httpx

import (
	"encoding/json"
	"net/http"
)

// envelope 统一响应信封。前端 public/app.js 按 code !== 200 判失败、从 data 取数据，
// 改结构要连前端一起改。
//
// 形状必须保持 { code, msg, message, data } —— 前端 app.js 的 request()
// 认 code !== 200 为失败、从 data 取业务数据、从 msg/message 取错误文案。
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
