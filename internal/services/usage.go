package services

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/geoip"
	"sandbox-pay/internal/httperr"
)

type UsageService struct{ store *db.Store }

func NewUsageService(store *db.Store) *UsageService { return &UsageService{store: store} }

type UsageLogOpts struct {
	Page       string
	ProjectIDs []int64
	PageSize   string
	GroupID    *int64
	APIKeyID   *int64
	Method     string
	Status     string
	Keyword    string
	DateFrom   string
	DateTo     string
	UTCOffset  int
}

type UsageLogDTO struct {
	ID             int64           `json:"id"`
	RequestID      string          `json:"requestId"`
	Method         string          `json:"method"`
	Path           string          `json:"path"`
	StatusCode     int             `json:"statusCode"`
	DurationMs     float64         `json:"durationMs"`
	GroupID        *int64          `json:"groupId,omitempty"`
	GroupName      string          `json:"groupName"`
	APIKeyID       *int64          `json:"apiKeyId,omitempty"`
	APIKeyName     string          `json:"apiKeyName"`
	KeyPrefix      string          `json:"keyPrefix"`
	KeySuffix      string          `json:"keySuffix"`
	ClientIP       string          `json:"clientIp"`
	ClientLocation *geoip.Location `json:"clientLocation,omitempty"`
	OrderNo        string          `json:"orderNo"`
	ErrorMessage   string          `json:"errorMessage"`
	CreatedAt      string          `json:"createdAt"`
}

type UsagePage struct {
	Items    []UsageLogDTO   `json:"items"`
	Total    int64           `json:"total"`
	Page     int64           `json:"page"`
	PageSize int64           `json:"pageSize"`
	Summary  db.UsageSummary `json:"summary"`
}

func (s *UsageService) GetLogs(opts UsageLogOpts) (*UsagePage, error) {
	opts.Method = strings.ToUpper(strings.TrimSpace(opts.Method))
	opts.Status = strings.ToLower(strings.TrimSpace(opts.Status))
	opts.Keyword = strings.TrimSpace(opts.Keyword)
	opts.DateFrom, opts.DateTo = strings.TrimSpace(opts.DateFrom), strings.TrimSpace(opts.DateTo)
	if err := validateUsageOpts(opts); err != nil {
		return nil, err
	}
	page, size, offset, err := usagePagination(opts.Page, opts.PageSize)
	if err != nil {
		return nil, err
	}
	out := &UsagePage{Items: make([]UsageLogDTO, 0), Page: page, PageSize: size}
	// A concurrent completed request must not make the page and its statistics
	// describe different filtered sets.
	err = s.store.Do(func(tx *db.TxScope) error {
		logs, total, summary, err := tx.UsageLogs.Search(db.UsageSearchOpts{
			ProjectIDs: opts.ProjectIDs,
			GroupID:    opts.GroupID, APIKeyID: opts.APIKeyID, Method: opts.Method,
			Status: opts.Status, Keyword: opts.Keyword, DateFrom: opts.DateFrom,
			DateTo: opts.DateTo, UTCOffset: opts.UTCOffset, Offset: offset, Limit: size,
		})
		if err != nil {
			return err
		}
		out.Total, out.Summary = total, summary
		for _, log := range logs {
			out.Items = append(out.Items, usageLogDTO(log))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *UsageService) GetLog(id int64) (*UsageLogDTO, error) {
	if id < 1 {
		return nil, httperr.BadRequest("使用日志编号无效")
	}
	log, err := s.store.UsageLogs.FindByID(id)
	if err != nil {
		return nil, err
	}
	if log == nil {
		return nil, httperr.NotFound("使用日志不存在")
	}
	item := usageLogDTO(log)
	return &item, nil
}

func usageLogDTO(log *db.UsageLog) UsageLogDTO {
	out := UsageLogDTO{
		ID: log.ID, RequestID: log.RequestID, Method: log.Method, Path: log.Path,
		StatusCode: log.StatusCode, DurationMs: log.DurationMs, GroupID: log.GroupID,
		GroupName: log.GroupName, APIKeyID: log.APIKeyID, APIKeyName: log.APIKeyName,
		KeyPrefix: log.KeyPrefix, KeySuffix: log.KeySuffix, ClientIP: log.ClientIP,
		OrderNo: log.OrderNo, ErrorMessage: log.ErrorMessage, CreatedAt: JsTime(log.CreatedAt),
	}
	if raw := strings.TrimSpace(log.ClientLocation); raw != "" && raw != "null" {
		var location geoip.Location
		if err := json.Unmarshal([]byte(raw), &location); err == nil {
			out.ClientLocation = &location
		}
	}
	return out
}

func validateUsageOpts(opts UsageLogOpts) error {
	if opts.GroupID != nil && *opts.GroupID < 1 {
		return httperr.BadRequest("项目编号无效")
	}
	if opts.APIKeyID != nil && *opts.APIKeyID < 1 {
		return httperr.BadRequest("API令牌编号无效")
	}
	switch opts.Method {
	case "", "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "CONNECT", "TRACE":
	default:
		return httperr.BadRequest("请求方法无效")
	}
	if opts.Status != "" && opts.Status != "success" && opts.Status != "failure" {
		return httperr.BadRequest("调用结果无效")
	}
	if utf8.RuneCountInString(opts.Keyword) > 200 {
		return httperr.BadRequest("搜索内容不能超过200个字符")
	}
	for _, date := range []string{opts.DateFrom, opts.DateTo} {
		if date != "" {
			if parsed, err := time.Parse("2006-01-02", date); err != nil || parsed.Format("2006-01-02") != date {
				return httperr.BadRequest("日期必须为有效的YYYY-MM-DD格式")
			}
		}
	}
	if opts.DateFrom != "" && opts.DateTo != "" && opts.DateFrom > opts.DateTo {
		return httperr.BadRequest("开始日期不能晚于结束日期")
	}
	if opts.UTCOffset < -840 || opts.UTCOffset > 840 {
		return httperr.BadRequest("时区偏移无效")
	}
	return nil
}

func usagePagination(rawPage, rawSize string) (page, size, offset int64, err error) {
	parse := func(raw string, fallback int64) int64 {
		value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil || value == 0 {
			return fallback
		}
		if value < 1 {
			return 1
		}
		return value
	}
	page, size = parse(rawPage, 1), parse(rawSize, 50)
	if size > 200 {
		size = 200
	}
	if page-1 > math.MaxInt64/size {
		return 0, 0, 0, httperr.BadRequest("页码过大")
	}
	return page, size, (page - 1) * size, nil
}
