// Package httpclient 极简 HTTP 客户端。
//
// 只覆盖用得到的两种调用：拉汇率（GET JSON）和发回调通知（POST）。
// 非 2xx 不报错，交给调用方按 status 判定 —— NotifyService 要区分
// 「HTTP 4xx/5xx」和「连不上」两种失败。
package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Response 响应状态 + 解析出来的 JSON（非 JSON 或空 body 为 nil，不报错）。
type Response struct {
	Status int
	Data   any
	// BodySummary is bounded raw response text. Callers must redact it before storing.
	BodySummary string
}

// Client 用到的两个方法，测试里塞假的。
type Client interface {
	Get(url string, timeout time.Duration) (*Response, error)
	// Post body 原样发送（[]byte/string 不再序列化）—— 回调签名是对
	// enqueue 时那份 body 字符串算的，重新 marshal 会改掉字节导致验签失败。
	Post(url string, body any, headers map[string]string, timeout time.Duration) (*Response, error)
}

// Default 基于 net/http 的默认实现。
type Default struct {
	HTTP *http.Client
}

// New 返回默认客户端。
func New() *Default { return NewWithOptions(Options{}) }

const maxResponseBytes = 1 << 20

func readJSON(resp *http.Response) (any, error) {
	data, _, err := readResponse(resp)
	return data, err
}

func readResponse(resp *http.Response) (any, string, error) {
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxResponseBytes {
		return nil, "", fmt.Errorf("HTTP response exceeds %d bytes", maxResponseBytes)
	}
	summaryBytes := data
	if len(summaryBytes) > 2048 {
		summaryBytes = summaryBytes[:2048]
	}
	summary := strings.ToValidUTF8(string(summaryBytes), "")
	if len(data) == 0 {
		return nil, "", nil
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, summary, nil
	}
	return v, summary, nil
}

func (d *Default) Get(url string, timeout time.Duration) (*Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, summary, err := readResponse(resp)
	if err != nil {
		return nil, err
	}
	return &Response{Status: resp.StatusCode, Data: data, BodySummary: summary}, nil
}

func (d *Default) Post(url string, body any, headers map[string]string, timeout time.Duration) (*Response, error) {
	var reader io.Reader
	switch b := body.(type) {
	case []byte:
		reader = bytes.NewReader(b)
	case string:
		reader = bytes.NewReader([]byte(b))
	case nil:
		reader = bytes.NewReader(nil)
	default:
		data, err := json.Marshal(b)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	// undefined 值的 header 直接不发，别兜底成空字符串 —— 下游会收到一个空签名头。
	for k, v := range headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}

	resp, err := d.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, summary, err := readResponse(resp)
	if err != nil {
		return &Response{Status: resp.StatusCode}, err
	}
	return &Response{Status: resp.StatusCode, Data: data, BodySummary: summary}, nil
}
