package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
	"sandbox-pay/internal/geoip"
	"sandbox-pay/internal/money"
	"sandbox-pay/internal/signature"
)

type DemoExample struct {
	Scenario string `json:"scenario"`
	OrderNo  string `json:"orderNo"`
	Project  string `json:"project"`
}

type DemoSeedSummary struct {
	Administrator string         `json:"administrator"`
	Counts        map[string]int `json:"counts"`
	Examples      []DemoExample  `json:"examples"`
}

type demoProject struct {
	id                       int64
	name, code, slug, secret string
	operator                 int64
	ip, location             string
	oldKey, currentKey       map[string]any
}

type demoOrder struct {
	row        map[string]any
	project    *demoProject
	key        map[string]any
	created    time.Time
	paid       time.Time
	number     string
	external   string
	method     string
	amount     int64
	refunded   int64
	trade      string
	lastNotify map[string]any
}

type demoBuilder struct {
	data              db.DemoData
	now               time.Time
	fees              *FeeCalculator
	users             map[int64]string
	projects          []*demoProject
	orderID, notifyID int64
	taskID, auditID   int64
	logID             int64
	examples          []DemoExample
}

// SeedDemo derives a sixty-day operating history from business scenarios.
// Credentials use real hashes; business fields come from the same event chain.
func SeedDemo(store *db.Store, cfg config.Config, now time.Time) (*DemoSeedSummary, error) {
	if !config.Bool(cfg, "ENABLE_SANDBOX") {
		return nil, fmt.Errorf("种子数据仅允许在沙箱模式中生成")
	}
	administrator, err := store.DemoAdministrator()
	if err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("DemoPay2026!"), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	if administrator == nil {
		administrator = &db.User{Username: "admin", Password: string(hash), SessionVersion: 0}
	}
	b := &demoBuilder{data: db.DemoData{}, now: now.UTC().Truncate(time.Second), fees: NewFeeCalculator(cfg), users: map[int64]string{}}
	b.setup(administrator, string(hash))
	b.history()
	b.showcase()
	b.data["operation_settings"] = []map[string]any{{"id": 1, "usageRetentionDays": 90, "auditRetentionDays": 180, "attemptRetentionDays": 90, "backupIntervalHours": 24, "backupKeepCount": 7, "updatedAt": seedTime(b.now.Add(-24 * time.Hour))}}
	if err := store.ReplaceDemoData(b.data); err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for table, rows := range b.data {
		counts[table] = len(rows)
	}
	return &DemoSeedSummary{Administrator: administrator.Username, Counts: counts, Examples: b.examples}, nil
}

func seedTime(t time.Time) string { return t.UTC().Format("2006-01-02 15:04:05.000") }
func seedID(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:16])
}
func (b *demoBuilder) add(table string, row map[string]any) {
	b.data[table] = append(b.data[table], row)
}

func (b *demoBuilder) setup(admin *db.User, passwordHash string) {
	created := b.now.AddDate(0, 0, -90)
	names := []string{admin.Username, "platform_ops", "store_ops", "cloud_ops", "content_ops", "archived_ops"}
	for i := 1; i < len(names); i++ {
		if names[i] == admin.Username {
			names[i] += "_demo"
		}
	}
	for i, name := range names {
		id, role, active, version, userHash := int64(i+1), db.RoleAdmin, true, int64(1), passwordHash
		updated := created
		if i < 2 {
			role = db.RoleSuperAdmin
		}
		if i == 0 {
			userHash, version = admin.Password, admin.SessionVersion+1
		}
		if i == 5 {
			active, updated = false, b.now.AddDate(0, 0, -21)
		}
		b.users[id] = name
		b.add("users", map[string]any{"id": id, "username": name, "password": userHash, "role": role, "isActive": active, "sessionVersion": version, "createdAt": seedTime(created), "updatedAt": seedTime(updated)})
		// Historical sessions have matching login/logout audit events. No old
		// or invented usable browser token is retained by the replacement.
		login := b.now.AddDate(0, 0, -25).Add(time.Duration(i) * time.Hour)
		logout := login.Add(90 * time.Minute)
		sessionID := seedID("session:"+name) + seedID("session-revoked:"+name)
		b.add("admin_sessions", map[string]any{"id": sessionID, "userId": id, "sessionVersion": version, "clientIp": "10.20.0." + strconv.Itoa(10+i), "userAgent": "Mozilla/5.0 Chrome/142.0.0.0", "createdAt": seedTime(login), "lastSeenAt": seedTime(logout), "expiresAt": seedTime(login.Add(24 * time.Hour)), "revokedAt": seedTime(logout)})
		b.audit(id, "POST", "/v1/api/auth/sessions/"+sessionID+"/revoke", "revoke-session", sessionID, logout)
	}
	definitions := []struct {
		name, code, slug, ip string
		operator             int64
	}{
		{"云栈文具商城", "SHOP", "yunzhan-shop", "120.76.1.20", 3},
		{"帆舟设计协作云", "SUB", "fanzhou-cloud", "47.98.10.20", 4},
		{"北辰数字内容", "DIGI", "beichen-digital", "8.210.10.20", 5},
		{"云栈商城 · 旧站迁移", "OLD", "yunzhan-legacy", "120.76.1.21", 6},
	}
	locator := geoip.New()
	for i, definition := range definitions {
		location, _ := json.Marshal(locator.Lookup(definition.ip))
		project := &demoProject{id: int64(i + 1), name: definition.name, code: definition.code, slug: definition.slug, ip: definition.ip, location: string(location), operator: definition.operator, secret: "whsec_" + randomHex(32)}
		active, updated := true, created.Add(2*time.Hour)
		if project.id == 4 {
			active, updated = false, b.now.AddDate(0, 0, -21)
		}
		b.add("groups", map[string]any{"id": project.id, "name": project.name, "isActive": active, "webhookSecret": project.secret, "createdAt": seedTime(created.Add(time.Hour)), "updatedAt": seedTime(updated)})
		b.audit(1, "POST", "/v1/api/admin/projects", "create-project", strconv.FormatInt(project.id, 10), created.Add(time.Hour))
		project.oldKey = b.makeKey(project, int64(i*2+1), "服务端接入 · 第一版", created.Add(2*time.Hour), false)
		keyCreated := b.now.AddDate(0, 0, -14)
		if project.id == 4 {
			keyCreated = created.Add(3 * time.Hour)
		}
		project.currentKey = b.makeKey(project, int64(i*2+2), "服务端接入 · 轮换版", keyCreated, active)
		b.add("project_members", map[string]any{"groupId": project.id, "userId": project.operator, "createdAt": seedTime(created.Add(4 * time.Hour))})
		if project.id == 4 {
			b.add("project_members", map[string]any{"groupId": project.id, "userId": int64(3), "createdAt": seedTime(b.now.AddDate(0, 0, -30))})
			b.audit(1, "PATCH", "/v1/api/admin/groups/4", "pause-project", "4", updated)
		}
		b.audit(1, "PUT", fmt.Sprintf("/v1/api/admin/team/users/%d/projects", project.operator), "assign-project", strconv.FormatInt(project.id, 10), created.Add(4*time.Hour))
		b.projects = append(b.projects, project)
	}
	b.audit(1, "PATCH", "/v1/api/admin/team/users/6", "disable-user", "6", b.now.AddDate(0, 0, -21))
}

func (b *demoBuilder) makeKey(project *demoProject, id int64, name string, created time.Time, active bool) map[string]any {
	credential := GenerateApiKey()
	updated := created
	if !active {
		updated = b.now.AddDate(0, 0, -14)
		if project.id == 4 {
			updated = b.now.AddDate(0, 0, -21)
		}
	}
	row := map[string]any{"id": id, "groupId": project.id, "name": name, "key": db.HashAPIKey(credential), "keyPrefix": credential[:16], "keySuffix": credential[len(credential)-4:], "isActive": active, "lastUsedAt": nil, "createdAt": seedTime(created), "updatedAt": seedTime(updated)}
	b.add("api_keys", row)
	b.audit(2, "POST", "/v1/api/admin/api-keys", "issue-token", strconv.FormatInt(id, 10), created)
	if !active {
		b.audit(2, "POST", fmt.Sprintf("/v1/api/admin/api-keys/%d/toggle", id), "disable-token", strconv.FormatInt(id, 10), updated)
	}
	return row
}

func (b *demoBuilder) history() {
	local := b.now.In(time.FixedZone("Asia/Hong_Kong", 8*3600))
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	for ago := 59; ago >= 0; ago-- {
		date := midnight.AddDate(0, 0, -ago)
		weekend := date.Weekday() == time.Saturday || date.Weekday() == time.Sunday
		for _, project := range b.projects {
			if project.id == 4 && ago < 22 {
				continue
			}
			count := []int{5, 3, 3, 1}[project.id-1]
			if weekend {
				if project.id == 1 {
					count = 8
				} else if project.id == 2 {
					count = 1
				}
			}
			if ago < 7 && project.id == 1 {
				count += 2 // The final-week stationery promotion has real volume.
			}
			for slot := 0; slot < count; slot++ {
				created := date.Add(time.Duration(9*60+slot*43+int(project.id)*17) * time.Minute)
				if created.Add(3 * time.Hour).After(b.now) {
					continue
				}
				index := 59 - ago + slot
				name, amount, method := demoProduct(project.id, index)
				order := b.order(project, name, amount, method, created)
				lifecycle := (ago*7 + slot*3 + int(project.id)) % 29
				switch lifecycle {
				case 0, 1:
					b.closeOrder(order, db.StatusExpired, created.Add(30*time.Minute))
				case 2:
					b.closeOrder(order, db.StatusFailed, created.Add(4*time.Minute))
				default:
					b.pay(order, created.Add(time.Duration(2+slot%5)*time.Minute), "success")
					switch lifecycle {
					case 3:
						b.refund(order, max(int64(1), amount/4), "活动价格调整，补退原订单金额的25%", "completed", order.paid.Add(45*time.Minute))
					case 4:
						b.refund(order, amount, "客户取消购买，原路全额退款", "completed", order.paid.Add(75*time.Minute))
					case 5:
						b.refund(order, amount/2, "售后协商分两笔退回，先退原订单金额的50%", "completed", order.paid.Add(30*time.Minute))
						b.refund(order, amount-amount/2, "第二笔退回剩余金额，订单全额退清", "completed", order.paid.Add(90*time.Minute))
					case 6:
						b.refund(order, amount/3, "协商退回三分之一金额（按分取整），首次渠道拒绝", "failed", order.paid.Add(30*time.Minute))
						b.refund(order, amount/3, "同一售后金额重新申请，渠道退款成功", "completed", order.paid.Add(60*time.Minute))
					case 7:
						b.cloneCallback(order, order.paid.Add(3*time.Minute), "duplicate")
					}
				}
			}
		}
	}
}

func demoProduct(project int64, index int) (string, int64, string) {
	switch project {
	case 1, 4:
		products := []struct {
			name  string
			price int64
		}{{"A5 点阵手账本", 3900}, {"桌面收纳套装", 8900}, {"黄铜自动铅笔", 2900}, {"会议便签套装", 5900}, {"水性笔补充装 · 12支", 2400}}
		product, units := products[index%len(products)], int64(1+index%3)
		method := "alipay"
		if index%5 == 0 {
			method = "native"
		}
		return fmt.Sprintf("%s × %d", product.name, units), product.price * units, method
	case 2:
		seats := int64(3 + index%5)
		if index%4 == 0 {
			return fmt.Sprintf("协作云年付订阅 · %d席位", seats), 99900 * seats, "native"
		}
		return fmt.Sprintf("协作云月付续费 · %d席位", seats), 9900 * seats, "alipay"
	default:
		names := []string{"极简图标包 · 商用授权", "品牌展示模板 · 团队授权", "产品原型组件库 · 年度授权"}
		return names[index%3], []int64{13900, 27900, 55900}[index%3], "paypal"
	}
}

func (b *demoBuilder) order(project *demoProject, product string, amount int64, method string, created time.Time) *demoOrder {
	b.orderID++
	local := created.In(time.FixedZone("HK", 8*3600))
	number := fmt.Sprintf("SP_%s_%s_%05d", local.Format("20060102"), project.code, b.orderID)
	external := fmt.Sprintf("%s-%s-%05d", project.code, local.Format("20060102"), b.orderID)
	key := project.oldKey
	if !created.Before(b.now.AddDate(0, 0, -14)) && project.id != 4 {
		key = project.currentKey
	}
	row := map[string]any{"id": seedID(external), "orderNo": number, "externalOrderNo": external, "amount": amount, "productName": product, "payMethod": method, "status": db.StatusPending, "createdAt": seedTime(created), "updatedAt": seedTime(created), "expireAt": seedTime(created.Add(30 * time.Minute)), "payAt": nil, "exchangeRate": nil, "foreignAmount": nil, "foreignCurrency": nil, "thirdPartyTradeNo": nil, "providerOrderId": nil, "refundedAmount": int64(0), "refundTradeNo": nil, "refundAt": nil, "channelCost": int64(0), "fee": int64(0), "settleAmount": int64(0), "returnUrl": "https://" + project.slug + ".example/orders/" + external, "notifyUrl": "https://" + project.slug + ".example/api/payment/notify", "groupId": project.id, "apiKeyId": key["id"], "userId": int64(0), "clientIp": project.ip, "clientLocation": project.location, "signingSecret": project.secret, "signatureVersion": 2}
	if method == "paypal" {
		rate := 0.139 + float64(local.Day()/7)*0.0005
		row["exchangeRate"], row["foreignCurrency"] = rate, "USD"
		row["foreignAmount"] = int64(math.Round(float64(amount) * rate))
		row["providerOrderId"] = strings.ToUpper(seedID("provider:" + number)[:17])
	}
	order := &demoOrder{row: row, project: project, key: key, created: created, number: number, external: external, method: method, amount: amount}
	b.add("payment_orders", row)
	b.usage(order, "pay", 200, "", created, 120+float64(project.id)*23)
	b.usage(order, "query", 200, "", created.Add(2*time.Second), 18)
	return order
}

func (b *demoBuilder) closeOrder(order *demoOrder, status int, when time.Time) {
	order.row["status"], order.row["updatedAt"] = status, seedTime(when)
	b.usage(order, "query", 200, "", when.Add(time.Second), 16)
}

func (b *demoBuilder) pay(order *demoOrder, when time.Time, delivery string) {
	order.paid, order.trade = when, "TX_"+order.method+"_"+seedID(order.number)[:20]
	if order.method == "paypal" {
		order.trade = strings.ToUpper(seedID("capture:" + order.number)[:17])
	} else if order.method == "alipay" {
		order.trade = when.UTC().Format("20060102150405") + fmt.Sprintf("000000%08d", b.orderID)
	}
	fees := b.fees.Calculate(order.method, order.amount)
	order.row["status"], order.row["payAt"], order.row["updatedAt"], order.row["thirdPartyTradeNo"] = db.StatusPaid, seedTime(when), seedTime(when), order.trade
	order.row["fee"], order.row["channelCost"], order.row["settleAmount"] = fees.Fee, fees.ChannelCost, fees.SettleAmount
	if order.method == "native" && delivery != "recovered" {
		b.audit(order.project.operator, "POST", "/v1/api/native-pay/confirm", "confirm-payment", order.number, when)
	}
	b.notification(order, "paid", order.trade, when.Add(time.Millisecond), delivery)
	b.usage(order, "query", 200, "", when.Add(4*time.Second), 21)
}

func (b *demoBuilder) refund(order *demoOrder, amount int64, reason, state string, when time.Time) {
	request := fmt.Sprintf("refund_%s_%d", order.external, len(b.data["refund_operations"])+1)
	trade := "REF_" + seedID(request)[:20]
	row := map[string]any{"id": refundOperationID(order.number, request), "requestId": request, "reason": reason, "orderNo": order.number, "amount": amount, "tradeNo": nil, "status": state, "createdAt": seedTime(when), "updatedAt": seedTime(when.Add(450 * time.Millisecond))}
	status, message := 200, ""
	switch state {
	case "completed":
		order.refunded += amount
		order.row["refundedAmount"], order.row["refundTradeNo"], order.row["refundAt"] = order.refunded, trade, row["updatedAt"]
		row["tradeNo"] = trade
		order.row["status"] = db.StatusPaid
		notifyStatus := "partial_refunded"
		if order.refunded == order.amount {
			order.row["status"], notifyStatus = db.StatusRefunded, "refunded"
		}
		b.notification(order, notifyStatus, trade, when.Add(451*time.Millisecond), "success")
	case "pending":
		row["tradeNo"], order.row["status"] = trade, db.StatusRefunding
	case "failed":
		status, message = 400, "支付渠道拒绝退款，请核对订单后重新申请"
	}
	order.row["updatedAt"] = row["updatedAt"]
	b.add("refund_operations", row)
	b.usage(order, "refund", status, message, when, 450)
	b.usage(order, "query", 200, "", when.Add(5*time.Second), 20)
}

func (b *demoBuilder) notification(order *demoOrder, status, trade string, when time.Time, delivery string) map[string]any {
	b.notifyID++
	event := PaymentNotification{OrderNo: order.number, ExternalOrderNo: &order.external, Status: status, Amount: money.ToYuan(order.amount), PayMethod: order.method, PayAt: JsTime(order.paid), ThirdPartyTradeNo: &trade}
	body, _ := json.Marshal(event)
	timestamp, nonce := strconv.FormatInt(when.UnixMilli(), 10), seedID(order.number + status + strconv.FormatInt(b.notifyID, 10))[:16]
	signed, _ := json.Marshal(notifySignature{Timestamp: timestamp, Nonce: nonce, Sign: signature.Sign(signature.BuildPayload(string(body), timestamp, nonce), order.project.secret), Version: 2})
	row := map[string]any{"id": b.notifyID, "orderNo": order.number, "url": order.row["notifyUrl"], "body": string(body), "signature": string(signed), "retryCount": 0, "status": db.NotifyPending, "createdAt": seedTime(when), "lastAttemptAt": nil, "lastError": nil, "availableAt": nil, "leaseUntil": nil, "sourceNotificationId": nil, "simulationMode": ""}
	b.add("notify_queues", row)
	b.deliver(row, order, when, delivery)
	order.lastNotify = row
	return row
}

func (b *demoBuilder) deliver(row map[string]any, order *demoOrder, when time.Time, delivery string) {
	if delivery == "pending" {
		return
	}
	codes := []int{200}
	switch delivery {
	case "recovered":
		codes = []int{503, 503, 200}
	case "failed":
		codes = []int{503, 503}
	case "exhausted":
		codes = []int{503, 503, 503, 503, 503}
	}
	delays := []time.Duration{time.Second, 15 * time.Second, 60 * time.Second, 300 * time.Second, 900 * time.Second}
	at, retry := when, 0
	for i, code := range codes {
		at = at.Add(delays[i])
		message, response := "", `{"accepted":true,"externalOrderNo":"`+order.external+`"}`
		if code == 503 {
			message, response = "HTTP 503", `{"message":"order worker temporarily unavailable","externalOrderNo":"`+order.external+`"}`
			retry++
		}
		b.add("notify_attempts", map[string]any{"notifyId": row["id"], "attemptNo": i + 1, "httpStatus": code, "responseSummary": response, "durationMs": 38 + i*14, "errorMessage": message, "createdAt": seedTime(at)})
		row["lastAttemptAt"] = seedTime(at)
		if code == 200 {
			row["status"], row["lastError"] = db.NotifySuccess, nil
		} else {
			row["status"], row["lastError"] = db.NotifyFailed, message
		}
	}
	row["retryCount"] = retry
	if retry == 5 {
		row["status"] = db.NotifyExhausted
	} else if delivery == "failed" {
		row["availableAt"] = seedTime(at.Add(60 * time.Second))
	}
}

func (b *demoBuilder) cloneCallback(order *demoOrder, when time.Time, mode string) {
	source := order.lastNotify
	if source == nil {
		return
	}
	b.notifyID++
	row := map[string]any{"id": b.notifyID, "orderNo": order.number, "url": source["url"], "body": source["body"], "signature": source["signature"], "retryCount": 0, "status": db.NotifyPending, "createdAt": seedTime(when), "lastAttemptAt": nil, "lastError": nil, "availableAt": nil, "leaseUntil": nil, "sourceNotificationId": source["id"], "simulationMode": mode}
	b.add("notify_queues", row)
	b.deliver(row, order, when, "success")
	b.audit(order.project.operator, "POST", fmt.Sprintf("/v1/api/admin/notifications/%v/simulate", source["id"]), "simulate-callback", order.number, when)
}

func (b *demoBuilder) usage(order *demoOrder, action string, status int, message string, when time.Time, duration float64) {
	b.logID++
	method := "POST"
	if action == "query" {
		method = "GET"
	}
	b.add("api_usage_logs", map[string]any{"requestId": "req_" + seedID(order.number+action+strconv.FormatInt(b.logID, 10)), "method": method, "path": "/v1/api/gateway/" + action, "statusCode": status, "durationMs": duration, "groupId": order.project.id, "groupName": order.project.name, "apiKeyId": order.key["id"], "apiKeyName": order.key["name"], "keyPrefix": order.key["keyPrefix"], "keySuffix": order.key["keySuffix"], "clientIp": order.project.ip, "clientLocation": order.project.location, "orderNo": order.number, "errorMessage": message, "createdAt": seedTime(when)})
	current, _ := order.key["lastUsedAt"].(string)
	if current < seedTime(when) {
		order.key["lastUsedAt"] = seedTime(when)
	}
}

func (b *demoBuilder) audit(user int64, method, path, action, target string, when time.Time) {
	b.auditID++
	labels := map[string]string{"create-project": "创建项目", "pause-project": "管理项目", "issue-token": "管理API令牌", "disable-token": "管理API令牌", "assign-project": "管理团队", "disable-user": "管理团队", "revoke-session": "撤销会话", "confirm-payment": "确认收款", "simulate-callback": "模拟回调", "schedule-sandbox": "模拟支付场景", "cancel-sandbox": "模拟支付场景"}
	if label := labels[action]; label != "" {
		action = label
	}
	b.add("admin_audit_logs", map[string]any{"userId": user, "username": b.users[user], "method": method, "path": path, "action": action, "target": target, "clientIp": "10.20.0." + strconv.FormatInt(10+user, 10), "statusCode": 200, "durationMs": 24, "requestId": "audit_" + seedID(path+target+strconv.FormatInt(b.auditID, 10)), "createdAt": seedTime(when)})
}

func (b *demoBuilder) showcase() {
	makeOrder := func(project int, product string, amount int64, method string, ago time.Duration, scenario string) *demoOrder {
		order := b.order(b.projects[project-1], product, amount, method, b.now.Add(-ago))
		b.examples = append(b.examples, DemoExample{Scenario: scenario, OrderNo: order.number, Project: order.project.name})
		return order
	}
	partial := makeOrder(1, "桌面收纳套装 + A5手账本 · 加购取消", 12800, "alipay", 120*time.Minute, "加购商品部分退款")
	b.pay(partial, partial.created.Add(3*time.Minute), "success")
	b.refund(partial, 3900, "取消加购的A5手账本，主商品保留", "completed", b.now.Add(-45*time.Minute))
	full := makeOrder(1, "桌面收纳套装 × 2 · 分批退货", 17800, "alipay", 170*time.Minute, "分两笔退款后全额退清")
	b.pay(full, full.created.Add(4*time.Minute), "success")
	b.refund(full, 8900, "第一套退货验收通过", "completed", b.now.Add(-120*time.Minute))
	b.refund(full, 8900, "第二套退货验收通过", "completed", b.now.Add(-35*time.Minute))
	refunding := makeOrder(3, "产品原型组件库 · 授权升级差价退回", 55900, "paypal", 80*time.Minute, "PayPal已受理，部分退款待确认")
	b.pay(refunding, refunding.created.Add(4*time.Minute), "success")
	b.refund(refunding, 13900, "基础图标授权139元重复计费，等待渠道原路退回", "pending", b.now.Add(-12*time.Minute))
	failed := makeOrder(1, "黄铜自动铅笔 × 3", 8700, "alipay", 50*time.Minute, "支付被渠道拒绝")
	b.closeOrder(failed, db.StatusFailed, failed.created.Add(4*time.Minute))
	expired := makeOrder(2, "协作云月付续费 · 5席位", 49500, "native", 100*time.Minute, "未付款超时关闭")
	b.closeOrder(expired, db.StatusExpired, expired.created.Add(30*time.Minute))
	delayed := makeOrder(2, "协作云年付订阅 · 3席位", 299700, "native", 18*time.Minute, "延迟到账完成、回调重试后恢复")
	b.pay(delayed, delayed.created.Add(8*time.Minute), "recovered")
	b.taskID++
	delayed.trade = fmt.Sprintf("SP_SANDBOX_%d", b.taskID)
	delayed.row["thirdPartyTradeNo"] = delayed.trade
	// Keep the notification trade aligned with the sandbox payment receipt.
	last := delayed.lastNotify
	var event PaymentNotification
	_ = json.Unmarshal([]byte(last["body"].(string)), &event)
	event.ThirdPartyTradeNo = &delayed.trade
	body, _ := json.Marshal(event)
	var meta notifySignature
	_ = json.Unmarshal([]byte(last["signature"].(string)), &meta)
	meta.Sign = signature.Sign(signature.BuildPayload(string(body), meta.Timestamp, meta.Nonce), delayed.project.secret)
	signed, _ := json.Marshal(meta)
	last["body"], last["signature"] = string(body), string(signed)
	b.add("sandbox_tasks", map[string]any{"id": b.taskID, "orderNo": delayed.number, "scenario": "delayed_paid", "status": "completed", "amount": 0, "runAt": seedTime(delayed.paid), "createdAt": seedTime(delayed.created.Add(time.Minute)), "updatedAt": seedTime(delayed.paid), "lastError": ""})
	b.audit(delayed.project.operator, "POST", "/v1/api/admin/sandbox/scenarios", "schedule-sandbox", delayed.number, delayed.created.Add(time.Minute))
	b.cloneCallback(delayed, b.now.Add(-5*time.Minute), "duplicate")
	pending := makeOrder(1, "A5点阵手账本 × 2 · 待付款", 7800, "alipay", 4*time.Minute, "新订单等待付款")
	_ = pending
	waiting := makeOrder(2, "协作云月付续费 · 3席位", 29700, "native", 3*time.Minute, "已预约延迟到账任务")
	b.taskID++
	b.add("sandbox_tasks", map[string]any{"id": b.taskID, "orderNo": waiting.number, "scenario": "delayed_paid", "status": "pending", "amount": 0, "runAt": seedTime(b.now.Add(12 * time.Minute)), "createdAt": seedTime(waiting.created.Add(time.Minute)), "updatedAt": seedTime(waiting.created.Add(time.Minute)), "lastError": ""})
	b.audit(waiting.project.operator, "POST", "/v1/api/admin/sandbox/scenarios", "schedule-sandbox", waiting.number, waiting.created.Add(time.Minute))
	callbackFailed := makeOrder(1, "水性笔补充装 · 12支 × 4", 9600, "native", 16*time.Minute, "付款成功，接入方503待重试")
	b.pay(callbackFailed, callbackFailed.created.Add(2*time.Minute), "failed")
	exhausted := makeOrder(3, "品牌展示模板 · 团队授权", 27900, "paypal", 60*time.Minute, "回调五次失败，等待人工重发")
	b.pay(exhausted, exhausted.created.Add(2*time.Minute), "exhausted")
	unconfirmed := makeOrder(3, "极简图标包 · 商用授权", 13900, "paypal", 2*time.Minute, "支付完成，回调尚未开始投递")
	b.pay(unconfirmed, unconfirmed.created.Add(time.Minute), "pending")
	cancelled := makeOrder(2, "协作云月付续费 · 4席位", 39600, "native", 150*time.Minute, "取消延迟任务后由财务手动核销")
	b.taskID++
	b.add("sandbox_tasks", map[string]any{"id": b.taskID, "orderNo": cancelled.number, "scenario": "delayed_paid", "status": "cancelled", "amount": 0, "runAt": seedTime(cancelled.created.Add(15 * time.Minute)), "createdAt": seedTime(cancelled.created.Add(time.Minute)), "updatedAt": seedTime(cancelled.created.Add(5 * time.Minute)), "lastError": ""})
	b.audit(cancelled.project.operator, "POST", fmt.Sprintf("/v1/api/admin/sandbox/scenarios/%d/cancel", b.taskID), "cancel-sandbox", cancelled.number, cancelled.created.Add(5*time.Minute))
	b.pay(cancelled, cancelled.created.Add(10*time.Minute), "success")
}
