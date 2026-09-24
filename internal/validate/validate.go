// Package validate 手写校验器。
//
// 只做这个项目真正需要的四件事：必填、类型、范围、白名单。
// 未声明的字段直接报错而不是静默丢掉。
//
// 错误信息统一是「字段 + 具体原因」（如 orderNo should not be empty），
// 前端 public/app.js 的 pickMessage 按这个口径展示，改文案要一起改。
package validate

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"sandbox-pay/internal/httperr"
)

// Rule 单字段的校验规则。
type Rule struct {
	// Type: "string" | "number" | "url"
	Type     string
	Optional bool
	Min      *float64 // number 下限 / string 最小长度
	Max      *float64 // number 上限 / string 最大长度
	In       []string // string 白名单
}

// Schema 字段名 → 规则。
type Schema map[string]Rule

// 构造函数，省掉到处写 &float64。
func f(v float64) *float64 { return &v }

// Str 必填字符串。min/max 传 0 表示不限。
//
// 0 当「没有这道限制」处理，不是「长度必须为 0」——
// 否则 `Str(0, 0)` 这种「必填但不管长度」的规则根本写不出来。
func Str(min, max int) Rule {
	r := Rule{Type: "string"}
	if min > 0 {
		r.Min = f(float64(min))
	}
	if max > 0 {
		r.Max = f(float64(max))
	}
	return r
}

// OptStr 可选字符串。min/max 传 0 表示不限。
func OptStr(min, max int) Rule {
	r := Rule{Type: "string", Optional: true}
	if min > 0 {
		r.Min = f(float64(min))
	}
	if max > 0 {
		r.Max = f(float64(max))
	}
	return r
}

// StrIn 必填字符串，限制白名单。
func StrIn(values ...string) Rule { return Rule{Type: "string", In: values} }

// OptStrIn 可选字符串，限制白名单。
func OptStrIn(values ...string) Rule {
	return Rule{Type: "string", Optional: true, In: values}
}

// Num 必填数字。min/max 传 0 表示不设该侧边界（同 Str 的口径）。
func Num(min, max float64) Rule {
	r := Rule{Type: "number"}
	if min != 0 {
		r.Min = f(min)
	}
	if max != 0 {
		r.Max = f(max)
	}
	return r
}

// OptNum 可选数字。min/max 传 0 表示不限。
func OptNum(min, max float64) Rule {
	r := Rule{Type: "number", Optional: true}
	if min != 0 {
		r.Min = f(min)
	}
	if max != 0 {
		r.Max = f(max)
	}
	return r
}

// URLRule 必填 URL。
func URLRule() Rule { return Rule{Type: "url"} }

// OptURLRule 可选 URL。
func OptURLRule() Rule { return Rule{Type: "url", Optional: true} }

// Result 校验结果，只含 schema 里声明的字段。
type Result struct {
	m map[string]any
}

// Str 取必填字符串。
func (r *Result) Str(key string) string {
	v, _ := r.m[key].(string)
	return v
}

// OptStr 取可选字符串，没传返回 ""。
func (r *Result) OptStr(key string) string {
	v, _ := r.m[key].(string)
	return v
}

// Has 字段是否出现过。
func (r *Result) Has(key string) bool {
	_, ok := r.m[key]
	return ok
}

// Num 取必填数字。
func (r *Result) Num(key string) float64 {
	v, _ := r.m[key].(float64)
	return v
}

// Map 底层的 map，个别场景直接读。
func (r *Result) Map() map[string]any { return r.m }

// Errf 400 校验错误。
func errf(format string, args ...any) error {
	return httperr.BadRequest(fmt.Sprintf(format, args...))
}

// numStr 数字的错误文案，和原 TS 版 Number() 的字符串化一致（255 不带 .0）。
func numStr(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }

func checkString(key string, value any, rule Rule) (any, error) {
	s, ok := value.(string)
	if !ok {
		return nil, errf("%s must be a string", key)
	}
	if rule.Min != nil && float64(len(s)) < *rule.Min {
		return nil, errf("%s must be longer than or equal to %s characters", key, numStr(*rule.Min))
	}
	if rule.Max != nil && float64(len(s)) > *rule.Max {
		return nil, errf("%s must be shorter than or equal to %s characters", key, numStr(*rule.Max))
	}
	if len(rule.In) > 0 {
		found := false
		for _, v := range rule.In {
			if v == s {
				found = true
				break
			}
		}
		if !found {
			return nil, errf("%s must be one of: %s", key, strings.Join(rule.In, ", "))
		}
	}
	return s, nil
}

func checkNumber(key string, value any, rule Rule) (any, error) {
	// JSON body 里可能是数字也可能是数字字符串，两种都收，但空串不算数字。
	var n float64
	switch v := value.(type) {
	case float64:
		n = v
	case json.Number:
		parsed, err := v.Float64()
		if err != nil {
			return nil, errf("%s must be a number", key)
		}
		n = parsed
	case string:
		if v == "" {
			return nil, errf("%s must be a number", key)
		}
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return nil, errf("%s must be a number", key)
		}
		n = parsed
	default:
		return nil, errf("%s must be a number", key)
	}
	if rule.Min != nil && n < *rule.Min {
		return nil, errf("%s must not be less than %s", key, numStr(*rule.Min))
	}
	if rule.Max != nil && n > *rule.Max {
		return nil, errf("%s must not be greater than %s", key, numStr(*rule.Max))
	}
	return n, nil
}

func checkURL(key string, value any) (any, error) {
	s, ok := value.(string)
	if !ok {
		return nil, errf("%s must be a URL address", key)
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errf("%s must be a URL address", key)
	}
	return s, nil
}

// Check 按 schema 校验，返回只含声明字段的结果。
//
// 未声明的字段直接报错而不是静默丢弃 —— 对应原版 ValidationPipe 的
// forbidNonWhitelisted，能第一时间发现拼错的字段名。
func Check(input any, schema Schema) (*Result, error) {
	src, ok := input.(map[string]any)
	if !ok || src == nil {
		return nil, httperr.BadRequest("请求体必须是 JSON 对象")
	}

	for key := range src {
		if _, declared := schema[key]; !declared {
			return nil, errf("property %s should not exist", key)
		}
	}

	out := make(map[string]any, len(schema))
	for key, rule := range schema {
		value, present := src[key]
		if !present || value == nil {
			if !rule.Optional {
				return nil, errf("%s should not be empty", key)
			}
			continue
		}
		var checked any
		var err error
		switch rule.Type {
		case "string":
			checked, err = checkString(key, value, rule)
		case "number":
			checked, err = checkNumber(key, value, rule)
		default:
			checked, err = checkURL(key, value)
		}
		if err != nil {
			return nil, err
		}
		out[key] = checked
	}
	return &Result{m: out}, nil
}
