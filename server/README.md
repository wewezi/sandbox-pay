# Sandbox Pay Server

Sandbox Pay 支付接入沙箱的后端服务，基于 **NestJS 11 + TypeORM + MySQL** 构建。提供多渠道下单路由（支付宝 / PayPal 沙箱、自有兜底通道）、商户管理、对账与退款等接口，用于支付联调与测试，支付结果为沙箱模拟，不涉及真实资金。

## 技术栈

- **框架**: NestJS 11 (Express)
- **数据库**: MySQL 8 + TypeORM 0.3
- **鉴权**: Passport-JWT（管理员账号密码登录，签发本地 JWT 会话 Cookie）
- **支付渠道**: 支付宝 (alipay-sdk)、PayPal (Checkout Server SDK)、自有兜底通道 (Native)
- **实时通信**: Socket.IO（订单状态推送到收银台）
- **接入文档**: 登录管理后台后访问 `/admin/docs`
- **Swagger**: 默认关闭，仅在受控开发环境设置 `ENABLE_API_DOCS=true` 后开放 `/v1/api/docs`
- **定时任务**: `@nestjs/schedule` + cron（异步通知重试、订单过期清理）
- **日志**: Winston
- **校验**: class-validator + 全局 ValidationPipe

## 目录结构

```
src/
├── auth/              JWT 鉴权（账号密码登录 / 会话 / 登出）
├── admin/             管理后台 API（统计 / 订单 / 商户 / 通知 / 对账 / 审计）
├── payment/           支付核心
│   ├── controllers/       alipay / paypal / native-pay 回调与操作
│   ├── gateways/          第三方网关封装（alipay-sdk / paypal-sdk / native）
│   ├── services/          统一下单 / 退款服务
│   ├── dto/               请求校验 DTO
│   └── payment.gateway.ts Socket.IO Gateway（实时状态推送）
├── gateway/           商户接入网关（HMAC-SHA256 签名校验）
├── common/            公共模块
│   ├── filters/           全局异常过滤器
│   ├── guards/            沙箱守卫 / 限流
│   ├── interceptors/      响应包装拦截器
│   ├── logging/           Winston 配置 + 日志脱敏
│   ├── middleware/        Correlation-Id 中间件
│   ├── services/          加密 / 签名 / 汇率 / 手续费 / 通知队列 / 审计 / 非对存储
│   ├── tasks/             定时任务（订单过期清理）
│   └── util/              CORS / 错误处理 / 请求工具 / 环境校验
├── entities/          数据实体（User / Merchant / PaymentOrder / NotifyQueue / ReconciliationRecord / AuditLog）
├── migrations/        TypeORM 迁移脚本
├── health/            健康检查端点
└── main.ts            入口（全局管道 / 过滤器 / Swagger / CORS / Helmet）
```

## 快速开始

```bash
pnpm install
cp .env.example .env   # 填写数据库 / JWT / 第三方凭证
pnpm dev               # http://localhost:3000
```

接入文档：登录管理后台后访问 `/admin/docs`。

Swagger 默认关闭；仅在受控开发环境设置 `ENABLE_API_DOCS=true` 后访问 <http://localhost:3000/v1/api/docs>。

## API 模块

所有业务接口以 `/v1/api/` 为前缀（URI 版本策略）。

| 路径前缀 | 模块 | 鉴权 | 说明 |
|---|---|---|---|
| `/v1/api/auth` | Auth | 混合 | 账号密码登录 / 会话查询 / 登出 |
| `/v1/api/admin` | Admin | JWT | 统计、订单、商户、通知、对账、审计、沙箱 |
| `/v1/api/gateway` | Gateway | HMAC 签名 | 商户接入：下单、查询、退款 |
| `/v1/api/native-pay` | NativePay | 混合 | 收银台、沙箱确认、渠道切换、公开测试 |
| `/v1/api/alipay` | Alipay | — | 支付宝异步通知回调 |
| `/v1/api/paypal` | PayPal | — | PayPal return callback |
| `/health` | Health | — | 健康检查（Docker healthcheck） |

## Admin API 详情

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/admin/stats` | 交易统计（总额 / 成功率） |
| GET | `/admin/transactions` | 订单列表（分页 / 筛选） |
| DELETE | `/admin/transactions/:orderNo` | 删除订单 |
| POST | `/admin/refund` | 管理员退款 |
| GET | `/admin/merchant` | 当前商户信息 |
| GET | `/admin/merchants` | 商户列表 |
| POST | `/admin/merchants` | 创建商户 |
| POST | `/admin/merchants/:id` | 编辑商户 |
| POST | `/admin/merchants/:id/toggle` | 启停商户 |
| POST | `/admin/merchant/reset-secret` | 重置商户密钥 |
| POST | `/admin/test-pay` | 创建测试订单 |
| GET | `/admin/notifications` | 通知队列列表 |
| POST | `/admin/notifications/:id/replay` | 重发通知 |
| POST | `/admin/reconciliation/upload` | 上传对账 CSV |
| GET | `/admin/reconciliation` | 对账记录列表 |
| GET | `/admin/audit-logs` | 审计日志 |
| POST | `/admin/reset-data` | 清除沙箱数据 |

## 架构要点

- **全局响应包装**: `TransformInterceptor` → `{ code, data, msg }`
- **全局异常**: `AllExceptionsFilter` 统一格式化
- **Correlation-Id**: 每个请求注入唯一 ID
- **异步通知**: `NotifyQueue` + cron 指数退避重试（5s → 15s → 60s → 5min → 15min）
- **金额安全**: 所有金额以整数分存储，浮点转换仅在 API 边界
- **幂等下单**: 基于 `(merchantId, externalOrderNo)` 去重
- **通知签名**: 每次回调携带 HMAC-SHA256 签名，商户可验真
- **Socket.IO**: 支付成功后实时推送到收银台，HTTP 轮询兜底

## 开发命令

```bash
pnpm dev            # 开发模式（热重载）
pnpm build          # 编译
pnpm start:prod     # 运行编译产物
pnpm lint           # ESLint 修复
pnpm format         # Prettier 格式化
pnpm test           # 单元测试
pnpm test:e2e       # 端到端测试
```
