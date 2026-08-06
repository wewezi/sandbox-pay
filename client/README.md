# Sandbox Pay Client

Sandbox Pay 支付接入沙箱的前端，基于 **React 19 + Vite 8 + TailwindCSS 4 + HeroUI** 构建。用于开发联调阶段测试多渠道下单、收银台、商户管理与对账流程，支付结果均为沙箱模拟，不涉及真实资金。

## 技术栈

- **框架**: React 19 + TypeScript
- **构建**: Vite 8
- **样式**: TailwindCSS 4 + HeroUI 3
- **动画**: Framer Motion 12
- **路由**: React Router 7
- **图标**: lucide-react + react-icons
- **实时通信**: socket.io-client

## 目录结构

```
src/
├── pages/
│   ├── Home.tsx             首页 / API 介绍
│   ├── Login.tsx            登录路由（登录入口暂未接入，仅静态提示页）
│   ├── Cashier.tsx          收银台（WebSocket + 轮询、多渠道支付）
│   └── MobilePay.tsx        移动端 H5 支付页（轮询 + 倒计时 + WebSocket）
├── pages/admin/
│   ├── DashboardOverview.tsx    总览仪表盘
│   ├── OrdersPage.tsx           订单管理（筛选 / 分页 / 退款 / 删除）
│   ├── MerchantsPage.tsx        商户管理（CRUD / 启停）
│   ├── NotificationsPage.tsx    通知队列监控（重发）
│   ├── ReconciliationPage.tsx   对账管理（CSV 上传 / 记录查看）
│   ├── AuditLogsPage.tsx        审计日志
│   └── SandboxPage.tsx          开发沙箱（密钥 / 测试下单 / 数据重置）
├── components/
│   ├── admin/
│   │   └── AdminLayout.tsx      侧边栏布局（导航 / 顶栏）
│   ├── dashboard/
│   │   ├── MerchantKeyPanel.tsx  商户密钥面板
│   │   ├── TestPayPanel.tsx      测试下单面板
│   │   ├── RefundDialog.tsx      退款弹窗
│   │   ├── DeleteConfirmDialog.tsx 删除确认弹窗
│   │   └── types.ts              类型定义
│   ├── RequireAuth.tsx           路由守卫
│   └── ThemeToggle.tsx           主题切换按钮
├── context/
│   ├── AuthContext.tsx       认证状态（会话校验 / 登出）
│   └── ThemeContext.tsx      主题切换（深浅色）
├── utils/
│   ├── api.ts                HTTP 请求封装（/v1/api）
│   ├── socket.ts             Socket.IO 订单状态订阅
│   └── toast.ts              Toast 通知
├── App.tsx                   路由配置
└── main.tsx                  入口
```

## 路由

| 路径 | 页面 | 说明 |
|---|---|---|
| `/` | Home | 平台首页 |
| `/login` | Login | 登录路由（登录入口暂未接入，显示占位提示页） |
| `/admin` | DashboardOverview | 控制台总览（需登录） |
| `/admin/orders` | OrdersPage | 订单管理（需登录） |
| `/admin/merchants` | MerchantsPage | 商户管理（需登录） |
| `/admin/notifications` | NotificationsPage | 通知队列（需登录） |
| `/admin/reconciliation` | ReconciliationPage | 对账管理（需登录） |
| `/admin/audit` | AuditLogsPage | 审计日志（需登录） |
| `/admin/sandbox` | SandboxPage | 开发沙箱（需登录） |
| `/dashboard` | — | 重定向到 `/admin` |
| `/cashier?orderNo=xxx` | Cashier | 收银台 |
| `/mobile-pay?orderNo=xxx` | MobilePay | 移动端支付 |

## 快速开始

```bash
pnpm install
pnpm dev          # http://localhost:5173
pnpm build        # 类型检查 + 生产构建
pnpm preview      # 预览生产构建
pnpm lint         # ESLint
```

## 环境变量

复制 `.env.example` 为 `.env.local`：

```bash
cp .env.example .env.local
```

| 变量 | 说明 |
|---|---|
| `VITE_API_BASE` | 后端 API 基础路径，默认走 Vite 代理 |

## 与后端联调

`vite.config.ts` 配置了开发代理：

- `/v1/api/*` → `http://localhost:3000`
- `/api/*` → `http://localhost:3000`
- `/socket.io` → `http://localhost:3000`（WebSocket）

生产环境 Nginx 反向代理同样需要配置 `/socket.io/` 的 WebSocket 升级支持。

## UI 主题

- 暗色 / 亮色双主题（`ThemeContext` + `ThemeToggle`）
- 全局选中色：emerald-500
- 主题切换瞬间完成（`theme-switching` class 禁用所有 transition）
