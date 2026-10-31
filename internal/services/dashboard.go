package services

import (
	"time"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/money"
)

type DashboardSummary struct {
	Amount         float64  `json:"amount"`
	SuccessCount   int64    `json:"successCount"`
	TotalCount     int64    `json:"totalCount"`
	SuccessRate    *float64 `json:"successRate"`
	RefundedAmount float64  `json:"refundedAmount"`
	Fee            float64  `json:"fee"`
}

type DashboardDay struct {
	Date         string  `json:"date"`
	Amount       float64 `json:"amount"`
	SuccessCount int64   `json:"successCount"`
	TotalCount   int64   `json:"totalCount"`
}

type DashboardChannel struct {
	PayMethod      string   `json:"payMethod"`
	Amount         float64  `json:"amount"`
	SuccessCount   int64    `json:"successCount"`
	TotalCount     int64    `json:"totalCount"`
	SuccessRate    *float64 `json:"successRate"`
	RefundedAmount float64  `json:"refundedAmount"`
}

// DashboardRecentOrder 「最近订单」的一行。status 用订单页那套展示名，
// 前端按同一张表上 badge，不用再翻状态数字。
type DashboardRecentOrder struct {
	OrderNo     string  `json:"orderNo"`
	ProductName string  `json:"productName"`
	PayMethod   string  `json:"payMethod"`
	Status      string  `json:"status"`
	Amount      float64 `json:"amount"`
	CreatedAt   string  `json:"createdAt"`
}

type DashboardResult struct {
	Days        int                `json:"days"`
	UTCOffset   int                `json:"utcOffset"`
	DateFrom    string             `json:"dateFrom"`
	DateTo      string             `json:"dateTo"`
	GeneratedAt string             `json:"generatedAt"`
	Summary     DashboardSummary   `json:"summary"`
	Previous    DashboardSummary   `json:"previous"`
	Trend       []DashboardDay     `json:"trend"`
	Channels    []DashboardChannel `json:"channels"`
	// RecentOrders 不受 days / utcOffset 影响，永远是全库最新几笔。
	RecentOrders []DashboardRecentOrder `json:"recentOrders"`
	Tasks        db.DashboardTasks      `json:"tasks"`
}

func (s *AdminService) GetDashboard(days, utcOffset int, scopes ...[]int64) (*DashboardResult, error) {
	return s.dashboardAt(days, utcOffset, time.Now(), scopes...)
}

func (s *AdminService) dashboardAt(days, utcOffset int, now time.Time, scopes ...[]int64) (*DashboardResult, error) {
	if days != 7 && days != 30 {
		return nil, httperr.BadRequest("统计天数仅支持 7 或 30")
	}
	if utcOffset < -720 || utcOffset > 840 {
		return nil, httperr.BadRequest("时区偏移须在 -720 至 840 分钟之间")
	}
	localNow := now.In(time.FixedZone("dashboard", utcOffset*60))
	year, month, day := localNow.Date()
	today := time.Date(year, month, day, 0, 0, 0, 0, localNow.Location())
	start := today.AddDate(0, 0, 1-days)
	end := today.AddDate(0, 0, 1)
	previousStart := start.AddDate(0, 0, -days)
	snapshot, err := s.store.Dashboard(previousStart, start, end, utcOffset, scopes...)
	if err != nil {
		return nil, err
	}
	out := &DashboardResult{
		Days: days, UTCOffset: utcOffset,
		DateFrom: start.Format("2006-01-02"), DateTo: today.Format("2006-01-02"),
		GeneratedAt: JsTime(now),
		Summary:     dashboardSummary(snapshot.Summary), Previous: dashboardSummary(snapshot.Previous),
		Trend: make([]DashboardDay, 0, days), Channels: make([]DashboardChannel, 0, len(snapshot.Channels)),
		RecentOrders: make([]DashboardRecentOrder, 0, len(snapshot.Recent)),
		Tasks:        snapshot.Tasks,
	}
	daily := make(map[string]db.DashboardDay, len(snapshot.Trend))
	for _, item := range snapshot.Trend {
		daily[item.Date] = item
	}
	for date := start; date.Before(end); date = date.AddDate(0, 0, 1) {
		key := date.Format("2006-01-02")
		item := daily[key]
		out.Trend = append(out.Trend, DashboardDay{
			Date: key, Amount: money.ToYuan(item.AmountCents),
			SuccessCount: item.SuccessCount, TotalCount: item.TotalCount,
		})
	}
	for _, item := range snapshot.Channels {
		summary := dashboardSummary(item.DashboardSummary)
		out.Channels = append(out.Channels, DashboardChannel{
			PayMethod: item.PayMethod, Amount: summary.Amount,
			SuccessCount: summary.SuccessCount, TotalCount: summary.TotalCount,
			SuccessRate: summary.SuccessRate, RefundedAmount: summary.RefundedAmount,
		})
	}
	for _, item := range snapshot.Recent {
		out.RecentOrders = append(out.RecentOrders, DashboardRecentOrder{
			OrderNo: item.OrderNo, ProductName: item.ProductName, PayMethod: item.PayMethod,
			Status: statusOf(item.Status), Amount: money.ToYuan(item.AmountCents),
			CreatedAt: JsTime(item.CreatedAt),
		})
	}
	return out, nil
}

func dashboardSummary(item db.DashboardSummary) DashboardSummary {
	out := DashboardSummary{
		Amount: money.ToYuan(item.AmountCents), SuccessCount: item.SuccessCount, TotalCount: item.TotalCount,
		RefundedAmount: money.ToYuan(item.RefundedAmountCents), Fee: money.ToYuan(item.FeeCents),
	}
	if item.TotalCount > 0 {
		rate := float64(item.SuccessCount) / float64(item.TotalCount)
		out.SuccessRate = &rate
	}
	return out
}
