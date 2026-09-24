// Package httperr HTTP 错误。返回即由 routes 的全局错误处理兜住，转成统一错误信封。
//
// 只有一个类型 + 状态码，测试里按 Status 断言；构造函数保证默认文案和原 TS 版一致。
package httperr

import "fmt"

// Error 带 HTTP 状态码的错误。
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

func new(status int, msg string) *Error { return &Error{Status: status, Message: msg} }

// Newf 自定义状态码和格式化消息。
func Newf(status int, format string, args ...any) *Error {
	return new(status, fmt.Sprintf(format, args...))
}

// BadRequest 400。
func BadRequest(msg string) *Error { return new(400, msg) }

// Conflictf 409，格式化。和 Newf(409, ...) 等价，给状态语义的调用点用。
func Conflictf(format string, args ...any) *Error {
	return new(409, fmt.Sprintf(format, args...))
}

// BadRequestf 400，格式化。
func BadRequestf(format string, args ...any) *Error {
	return new(400, fmt.Sprintf(format, args...))
}

// Unauthorized 401，默认文案。
func Unauthorized(msg ...string) *Error { return new(401, pick(msg, "Unauthorized")) }

// Forbidden 403，默认文案。
func Forbidden(msg ...string) *Error { return new(403, pick(msg, "Forbidden")) }

// NotFound 404，默认文案。
func NotFound(msg ...string) *Error { return new(404, pick(msg, "Not Found")) }

// Conflict 409，默认文案。
func Conflict(msg ...string) *Error { return new(409, pick(msg, "Conflict")) }

// TooManyRequests 429，默认文案。
func TooManyRequests(msg ...string) *Error {
	return new(429, pick(msg, "请求过于频繁，请稍后再试"))
}

// Internal 500，默认文案。
func Internal(msg ...string) *Error { return new(500, pick(msg, "Internal server error")) }

func pick(msg []string, def string) string {
	if len(msg) > 0 && msg[0] != "" {
		return msg[0]
	}
	return def
}

// As 从 error 里取出 *Error，不是则返回 nil。
func As(err error) *Error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*Error); ok {
		return e
	}
	return nil
}
