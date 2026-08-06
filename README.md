# Sandbox Pay

支付接入沙箱，用于开发联调与测试阶段模拟支付流程，提供商户接入、聚合下单、多渠道支付（支付宝 / PayPal 沙箱 / 自有兜底）、收银台、管理后台等能力。支付结果均为沙箱模拟，不涉及真实资金托管或清算。

## 仓库结构

```
sandbox-pay/
├── server/    NestJS 11 + TypeORM + PostgreSQL  服务端
└── client/    React 19 + Vite 8 + Tailwind 4  Web 端
```

详细文档见各子项目：

- [`server/README.md`](./server/README.md)
- [`client/README.md`](./client/README.md)

## 技术栈速览

| 端 | 主要技术 |
|---|---|
| 后端 | NestJS 11、TypeORM、PostgreSQL/PGlite、Passport-JWT、Socket.IO、Swagger、Winston |
| 前端 | React 19、Vite 8、TailwindCSS 4、HeroUI、Framer Motion、React Router 7 |
| 支付 | 支付宝（alipay-sdk）、PayPal（Checkout Server SDK）、自有兜底通道 |

## Docker 一键部署（推荐）

```bash
docker build -t sandbox-pay .
docker run -d -p 3000:3000 -v sandbox-pay-data:/var/lib/postgresql/data sandbox-pay
```

访问 http://localhost:3000

详见 [`DOCKER.md`](./DOCKER.md)

## 本地启动

需要：Node 20+，pnpm，PostgreSQL 16。

### 1. 后端

```bash
cd server
cp .env.example .env       # 填写数据库 / JWT / 第三方凭证
pnpm install
pnpm migration:run         # 初始化数据库表结构
pnpm create:admin admin your_password   # 创建管理员账号（用于登录后台）
pnpm dev                   # http://localhost:3000
```

登录后台使用管理员账号密码，账号通过 `pnpm create:admin <用户名> <密码>` 创建（重复执行会重置密码）。

接入文档位于登录后的管理后台：`/admin/docs`。

Swagger 默认关闭；仅在受控开发环境设置 `ENABLE_API_DOCS=true` 后访问 <http://localhost:3000/v1/api/docs>。

### 2. 前端

```bash
cd client
cp .env.example .env.local
pnpm install
pnpm dev                   # http://localhost:5173
```

前端开发服务器已通过 `vite.config.ts` 将 `/api` 和 `/v1/api` 代理到后端 `http://localhost:3000`，同时代理 `/socket.io` WebSocket 连接。

## Docker 部署

```bash
cp server/.env.example .env  # 按需填写环境变量
docker compose up -d
```

- 客户端：`http://localhost`
- 后端 API：`http://localhost/v1/api/`
- 接入文档：登录管理后台后访问 `/admin/docs`
- Swagger：默认关闭；按需设置 `ENABLE_API_DOCS=true` 后访问 `http://localhost/v1/api/docs`

## 支付流程架构

```
商户系统                  Sandbox Pay 网关                上游渠道
   │                           │                           │
   │  POST /gateway/pay        │                           │
   │  (HMAC-SHA256 签名)       │                           │
   ├──────────────────────────>│                           │
   │                           │  创建订单 (Pending)        │
   │                           │  计算手续费/汇率            │
   │                           │──┐                        │
   │                           │  │ 按 payMethod 路由       │
   │                           │<─┘                        │
   │                           │                           │
   │                           │  alipay ─────────────────>│ 支付宝
   │                           │  paypal ─────────────────>│ PayPal
   │                           │  native ─────────┐        │ 官方存管
   │  返回支付链接/表单          │                 │        │
   │<──────────────────────────┤                 │        │
   │                           │                 │        │
   │                           │  异步回调通知     │        │
   │                           │<────────────────┘        │
   │                           │                           │
   │                           │  markPaid (行锁事务)       │
   │                           │  计算手续费                │
   │                           │  WebSocket 推送收银台      │
   │                           │  入队异步通知              │
   │                           │                           │
   │  POST /notify (签名)       │                           │
   │<──────────────────────────┤  指数退避重试              │
   │                           │  (5s→15s→60s→5m→15m)     │
```

## 核心功能

- **商户接入**：基于 `appKey + appSecret` HMAC-SHA256 签名的下单网关
- **聚合下单**：自动按规则路由到支付宝 / PayPal / 自有兜底
- **收银台**：WebSocket 实时推送 + HTTP 轮询兜底、二维码 / 钱包登录、渠道切换
- **异步通知**：失败重试队列（指数退避 5s → 15s → 60s → 5min → 15min）
- **管理后台**（8 个页面）：
  - 总览仪表盘 — 统计卡片 + 最近交易 + 快速操作
  - 订单管理 — 关键词搜索、状态/渠道/日期筛选、分页、确认收款、退款、删除
  - 商户管理 — 创建/编辑/启停商户、密钥查看与复制
  - 通知队列 — 异步回调状态监控、失败通知一键重发
  - 对账管理 — 上传支付宝/PayPal 日账单 CSV、自动匹配、差异识别
  - 审计日志 — 敏感操作追溯、按操作类型/操作者筛选
  - 站点设置 — 手续费/汇率/限流/沙箱/邮件等业务参数热修改（无需重启）
  - 开发沙箱 — 密钥管理、测试下单、数据重置
- **对账**：上传支付宝/PayPal 日账单 CSV，自动匹配本地订单，识别差异
- **实时推送**：Socket.IO 推送订单状态变更到收银台

> 说明：原有 OIDC SSO 登录已移除，目前尚未接入替代登录方式，管理后台暂无可用的运行时登录入口。

## 鉴权机制

| 接口类型 | 鉴权方式 | 说明 |
|---|---|---|
| 商户网关 (`/api/gateway/*`) | HMAC-SHA256 签名 | 请求头携带 `X-Sandbox-Pay-AppKey` + `Timestamp` + `Nonce` + `Signature` |
| 管理后台 (`/api/admin/*`) | JWT Bearer Token | 本地 JWT 会话 Cookie（`sandbox_pay_access_token`）；登录入口暂未接入 |
| 收银台 (`/api/native-pay/cashier`) | 无（公开） | 凭 orderNo 查询，设计如此 |
| 沙箱接口 (`sandbox-confirm`, `test-pay`) | SandboxGuard | 仅 `ENABLE_SANDBOX=true` 时开放 |
| 健康检查 (`/health`) | 无 | Docker healthcheck 用 |

## 安全建议

- 生产环境务必将 `DB_SYNCHRONIZE` 设为 `false`，使用 TypeORM 迁移管理表结构
- `JWT_SECRET` / `GATEWAY_SECRET` / `ENCRYPTION_KEY` 使用 `openssl rand -hex 32` 生成
- 生产环境 `ENCRYPTION_KEY` 为必填项（与 `JWT_SECRET` 必须不同）
- 严禁提交 `.env` / `.env.local` 等密钥文件（`.gitignore` 已配置）
- 支付宝 / PayPal 凭证通过环境变量注入
- `ENABLE_SANDBOX` 生产环境必须设为 `false`
- `PAYPAL_ENVIRONMENT` 生产环境设为 `live`

## 开发规范

### 提交规范

```
feat: 新功能
fix: 修复
refactor: 重构（不改变行为）
docs: 文档
test: 测试
chore: 构建/工具链
```

### 分支策略

- `main` — 生产分支，保护分支
- `feat/*` — 功能分支，合并前需 CI 通过
- `fix/*` — 修复分支

### 代码规范

- 后端：NestJS 模块化、TypeORM 实体、class-validator DTO
- 前端：React 函数组件 + Hooks、Tailwind CSS、HeroUI
- 错误消息统一中文
- 金额统一整数分存储，API 边界转 yuan
- 敏感信息不入日志（使用 `redact()` 函数）

### 测试

```bash
# 后端单元测试
cd server && pnpm test

# 后端 e2e 测试
cd server && pnpm test:e2e

# 前端构建检查
cd client && pnpm build
```

## License

UNLICENSED — Internal use only.
