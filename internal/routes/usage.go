package routes

import (
	"encoding/json"
	"net/http"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httpx"
)

// captureGatewayUsage 记录网关分组的请求，包括限流、鉴权失败和未知接口。
func (a *App) captureGatewayUsage() func(http.Handler) http.Handler {
	return httpx.CaptureAPIUsage(httpx.UsageOptions{
		Record: func(record httpx.UsageRecord) error {
			return a.c.Store.UsageLogs.Insert(&db.UsageLog{
				RequestID: record.RequestID, Method: record.Method, Path: record.Path,
				StatusCode: record.StatusCode, DurationMs: record.DurationMS,
				GroupID: record.GroupID, GroupName: record.GroupName,
				APIKeyID: record.APIKeyID, APIKeyName: record.APIKeyName,
				KeyPrefix: record.KeyPrefix, KeySuffix: record.KeySuffix,
				ClientIP: record.ClientIP, ClientLocation: record.ClientLocation,
				OrderNo: record.OrderNo, ErrorMessage: record.ErrorMessage,
				CreatedAt: record.CreatedAt,
			})
		},
		LookupKey: func(raw string) (*db.ActiveKeyHit, error) {
			key, err := a.c.Store.ApiKeys.FindByKey(raw)
			if err != nil || key == nil {
				return nil, err
			}
			group, err := a.c.Store.Groups.FindByID(key.GroupID)
			if err != nil {
				return nil, err
			}
			return &db.ActiveKeyHit{Key: key, Group: group}, nil
		},
		LocateIP: func(ip string) string {
			if a.c.Geolocation != nil {
				if location, err := json.Marshal(a.c.Geolocation.Lookup(ip)); err == nil {
					return string(location)
				}
			}
			return ""
		},
		WriteFailed: func() { a.log.Warn("API usage log could not be recorded") },
	})
}
