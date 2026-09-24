package routes

import (
	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
	"sandbox-pay/internal/gateways"
	"sandbox-pay/internal/httpclient"
	"sandbox-pay/internal/services"
)

// Container 手工装配所有依赖，全项目只有这一处知道谁依赖谁。
//
// 依赖关系一眼能看完，顺序就是构造顺序 —— 不需要 provider / inject / exports
// 那一整套元数据。服务之间全靠构造函数传，测试里各自 new 一个塞假依赖即可。
type Container struct {
	Config config.Config
	Store  *db.Store

	Admin   *services.AdminService
	Auth    *services.AuthService
	Fees    *services.FeeCalculator
	Rates   *services.ExchangeRateService
	Notify  *services.NotifyService
	Groups  *services.GroupService
	Payment *services.PaymentService
	Refund  *services.RefundService

	Alipay    *gateways.Alipay
	PayPal    *gateways.PayPal
	NativePay *gateways.NativePay
}

// NewContainer 构造。网关要能拿到 PaymentService（回调入账），
// PaymentService 又要能拿到 NotifyService（支付成功入队回调），
// 所以顺序是：基础件 → notify → payment → 网关 → 依赖它们的编排层。
func NewContainer(cfg config.Config, store *db.Store) *Container {
	httpc := httpclient.New()

	fees := services.NewFeeCalculator(cfg)
	rates := services.NewExchangeRateService(httpc, cfg)
	notify := services.NewNotifyService(store.Notifies, httpc)
	groups := services.NewGroupService(store, cfg)
	payment := services.NewPaymentService(store, notify, fees)

	alipay := gateways.NewAlipay(cfg, store, payment)
	paypal := gateways.NewPayPal(cfg, store, rates, payment)
	nativePay := gateways.NewNativePay(cfg, store, payment, alipay)

	return &Container{
		Config:    cfg,
		Store:     store,
		Admin:     services.NewAdminService(store),
		Auth:      services.NewAuthService(store, cfg),
		Fees:      fees,
		Rates:     rates,
		Notify:    notify,
		Groups:    groups,
		Payment:   payment,
		Refund:    services.NewRefundService(store, payment, alipay, paypal),
		Alipay:    alipay,
		PayPal:    paypal,
		NativePay: nativePay,
	}
}
