# Sandbox Pay

自用的沙箱支付系统。给实验性项目提供一个能跑通完整支付流程的收款入口：接入方凭 Key 下单 → 收银台付款 → 异步回调通知 → 后台确认/退款。

支付宝和 PayPal 走的是**两家真实的沙箱 API**，不是 mock；native 是自有兜底通道，靠后台手动核销。全部是假数据，不涉及真实资金。

## 技术栈

| 层 | 用什么 |
|---|---|
| 后端 | Go + `net/http` 标准库路由，没有 Web 框架 |
| 存储 | SQLite（`modernc.org/sqlite`，纯 Go 驱动，无 CGO） |
| 测试 | `go test` —— 单测和 e2e 同一条命令 |
| 前端 | 原生 HTML / CSS / JavaScript + Web Components，由后端 embed 托管，无构建步骤。类型靠 JSDoc + `checkJs` |
| 渠道 | 支付宝沙箱（`github.com/smartwalle/alipay/v3`）、PayPal 沙箱（`github.com/plutov/paypal`）、native 兜底 |

单进程、单库文件、单个二进制。没有 Docker、没有迁移脚本、没有前端构建。

```
sandbox-pay/
├── internal/
│   ├── config/       .env 解析 + 默认值
│   ├── logger/       分级日志
│   ├── money/        整数分 ↔ 元
│   ├── signature/    回调 HMAC-SHA256 签名与验签
│   ├── apiurl/       /v1/api 前缀与 URL 拼接
│   ├── jwt/          手写 HS256，只认 HS256
│   ├── httperr/      带状态码的错误
│   ├── httpclient/   带超时的 HTTP 客户端
│   ├── validate/     请求体校验
│   ├── db/           建表 SQL、行类型、各表 repo（裸 SQL）
│   ├── httpx/        中间件：信封、错误、安全头、CORS、限流、鉴权
│   ├── services/     业务逻辑
│   ├── gateways/     支付宝 / PayPal / native 三个渠道
│   └── routes/       路由装配 + 依赖容器 + e2e 测试
├── public/           静态前端（7 个页面），同时被 go:embed 打进二进制
├── main.go           入口：读 .env → 开库 → 装配 → 起服务
├── go.mod / go.sum
├── package.json      只为跑 tsc 做前端类型检查，没有构建步骤
├── .env.example
├── data/             SQLite 库文件（gitignore）
└── README.md
```

### 为什么是标准库

原来是 NestJS + TypeORM，后来换成 Hono + better-sqlite3，现在换成 Go。动机是**目标机器只有 2c2G** —— Node 常驻的堆开销和 TS 运行时的 CPU 都吃不消，换成 Go 之后常驻内存从几百 MB 掉到几十 MB，冷启动也快得多。

具体到每层的替代关系：

| 关心的事 | 用什么 |
|---|---|
| 路由 | `http.ServeMux` 的 `method + path` 模式（Go 1.22+） |
| 中间件 | `func(http.Handler) http.Handler`，`httpx.Chain` 串联 |
| 错误处理 | handler 签名 `func(w, r) error`，返回即进统一信封 |
| SQLite | `modernc.org/sqlite`（纯 Go，不用 CGO，交叉编译随便玩） |
| ORM / 迁移 | 没有。裸 SQL + `CREATE TABLE IF NOT EXISTS` |
| DI 容器 | 没有。`routes.NewContainer` 手写构造顺序，一眼看完谁依赖谁 |

## 起步

需要 **Go 1.26+**。前端类型检查另外要 Node（只为跑 `tsc`，不参与运行时）。以下命令都在仓库根目录执行。

```bash
cp .env.example .env                      # 至少改 JWT_SECRET
go run . create-admin admin your_password # 创建后台账号，重复执行会重置密码
go run .                                  # http://localhost:3000
```

表结构在首次连库时由 `internal/db/schema.go` 建好（全是 `CREATE TABLE IF NOT EXISTS`，可重复执行）。首次启动会顺带创建种子分组，并把生成的 `GROUP_API_KEY` 打到日志里 —— 想固定下来就抄回 `.env`，否则每次重启都会重新生成。

只有 `CREATE TABLE IF NOT EXISTS`，没有迁移层。**改了列，已存在的库不会跟着变**，所以改列名或删列之后老库必须删掉重建：

```bash
rm data/sandbox-pay.db*
go run . create-admin admin your_password
```

打开 http://localhost:3000 登录后台。

### 打包部署

前端通过 `go:embed` 打进二进制，所以部署只需要一个文件：

```bash
go build -ldflags="-s -w" -o sandbox-pay .
./sandbox-pay
```

**开发时**如果 cwd 下有 `public/` 目录，就直接读磁盘 —— 改完前端刷新即可，不用重新编译。找不到 `public/` 才回落到 embed 里那份（也就是 `./sandbox-pay` 单文件部署的形态）。

### 配置渠道

`.env` 里的渠道凭证**留空即该渠道不可用**，调用时报「未配置」，不影响启动：

- 支付宝：`ALIPAY_APP_ID` / `ALIPAY_PRIVATE_KEY` / `ALIPAY_PUBLIC_KEY`
- PayPal：`PAYPAL_CLIENT_ID` / `PAYPAL_CLIENT_SECRET`

支付宝的异步通知要能打回本机，`CLIENT_URL` 得填内网穿透地址（如 `https://xxx.ngrok.app`）；只在本地点着玩就留空，用请求自身的 baseUrl。收银台对支付宝还会主动查一次订单状态，所以异步通知打不进来时也能刷出结果。

其余业务参数（手续费率、汇率兜底、限流、native 收款信息）都有默认值，见 [`.env.example`](.env.example)。

## 页面

后台在 `public/`，纯静态，改完刷新即可，不用重启也不用构建。

| 页面 | 说明 |
|---|---|
| `index.html` | 落地页，按有没有 token 跳 `orders.html` 或 `login.html` |
| `login.html` | 后台登录 |
| `orders.html` | 订单列表：关键词/状态/渠道/日期筛选、分页、确认收款、退款、删除 |
| `notifications.html` | 通知队列：回调状态、重试次数、最近错误、失败重发 |
| `groups.html` | 分组与 API Key：新建分组、签发/停用 Key、凭证查看/复制、签名说明 |
| `sandbox.html` | 测试下单、重置数据 |
| `cashier.html` | 收银台（公开页，凭 orderNo 访问）：二维码、倒计时、切渠道、手动确认 |

收银台状态走 5 秒轮询，没有 WebSocket。

亮色/暗色可切，三态循环：亮色 → 暗色 → 跟随系统，选择存在 localStorage。
有侧边栏的页面开关在「退出」上面，`login.html` / `cashier.html` 在左上角。

配色只在 `app.css` 的 `:root` 里声明一份，写法是 `light-dark(亮, 暗)`；
切换只改 `color-scheme`，不覆盖具体颜色。要加新颜色就照这个写，
别再开 `@media (prefers-color-scheme: dark)` 覆盖一遍 —— 那样三态会对不上。
非颜色的值（比如 `filter`）塞不进 `light-dark()`，得自己写选择器，
现成的例子是日期选择器图标那条。

## 支付流程

```
接入方系统              Sandbox Pay                    上游
   │                        │                           │
   │ POST /gateway/pay      │                           │
   │ (Bearer apiKey)        │                           │
   ├───────────────────────>│ 建单 Pending + 算手续费    │
   │                        │ 按 payMethod 路由          │
   │                        │  alipay ────────────────> 支付宝沙箱
   │                        │  paypal ────────────────> PayPal 沙箱
   │ <── 支付链接 / 表单 ────┤  native ──> 收银台手动核销
   │                        │                           │
   │                        │<──── 异步回调 / 主动查单 ───┤
   │                        │ markPaid（事务 + 状态幂等） │
   │                        │ 入队异步通知               │
   │ POST /notify (HMAC 签名)│                           │
   │<───────────────────────┤ 失败指数退避 5s→15s→60s→5m→15m
```

## 接口

业务接口统一 `/v1/api/` 前缀（常量在 `internal/apiurl`，只有 `/health` 不带）。

| 前缀 | 鉴权 | 说明 |
|---|---|---|
| `/v1/api/auth` | 混合 | 登录 / 会话 / 登出 |
| `/v1/api/admin` | JWT | 统计、订单、退款、通知队列、测试下单、重置数据 |
| `/v1/api/gateway` | API Key | 接入网关：下单、查询、退款 |
| `/v1/api/native-pay` | 混合 | 收银台信息、切渠道、确认收款 |
| `/v1/api/alipay` | — | 支付宝异步通知 |
| `/v1/api/paypal` | — | PayPal return callback |
| `/health` | — | 健康检查 |

### 接入方鉴权

入站只要一个 Key，没有时间戳也没有签名：

```
Authorization: Bearer <apiKey>
```

```bash
curl -X POST http://localhost:3000/v1/api/gateway/pay \
  -H "Authorization: Bearer sp_sandbox_xxx" \
  -H "Content-Type: application/json" \
  -d '{"amount":1,"productName":"测试","payMethod":"native"}'
```

**出站回调仍然签名**，方向相反、用途也不同：接入方靠它确认这条「订单已支付」
真是 Sandbox Pay 发的，不然谁扫到 `notifyUrl` 都能伪造入账。密钥是同一个 `apiKey`：

```
X-Sandbox-Pay-Timestamp  毫秒时间戳
X-Sandbox-Pay-Nonce      随机串
X-Sandbox-Pay-Signature  HMAC-SHA256(body + "&timestamp=" + ts + "&nonce=" + nonce, apiKey)
```

验签的参考实现是 `internal/signature`；`internal/services/notify_test.go`
用它证明签出来的东西验得回去。`apiKey` 既是凭证也是密钥，别外传。

### 分组与 Key

一个分组下可以挂多把 Key，用哪把下的单就记在订单的 `apiKeyId` 上。
分组是隔离边界，Key 只是同一个边界里的多个入口：

- **数据隔离按分组走**：A 分组的 Key 查不到 B 分组的订单，退款同理
- **幂等去重按 `(groupId, externalOrderNo)`**：同分组换把 Key 用同一个 `externalOrderNo`
  重下会命中原来那一单；不同分组用同一个 `externalOrderNo` 则各自建单，互不干扰
- **回调用下单那把 Key 签名**，所以停用一把 Key 只挡后续入站，
  已有订单的回调仍按原名签 —— 接入方那边验签逻辑不用动
- **停用分组**会让它名下所有 Key 一起调不通（守卫 JOIN 时连分组的 `isActive` 一起查）

界面在「分组与 Key」页（`groups.html`）。

## 实现要点

- 金额一律按**整数分**存储，只在 API 边界转元（`internal/money`）
- 幂等下单基于 `(groupId, externalOrderNo)` 去重 —— 换把同分组的 Key 重下会命中同一单，不同分组各自独立
- `markPaid` / `markRefunded` 走 `Store.Do`，在事务里按状态判定幂等。SQLite 是单写者（`SetMaxOpenConns(1)`），写事务本就串行，没有 `SELECT ... FOR UPDATE`
- **回调这类「事务外才做的动作」一律放在 `Do` 返回之后** —— 事务里不能等网络。发通知入队紧跟在 `markPaid` 的事务之后
- 时间列格式沿用 TypeORM 那套 `YYYY-MM-DD HH:MM:SS.sss`（UTC，不带 `T` 和 `Z`）。别改成 ISO：排序是文本比较，空格 `0x20` < `T` `0x54`，两种格式混在一列里 `ORDER BY createdAt` 就乱了（见 `internal/db/datetime.go`）。**出给前端时**才转成 `2006-01-02T15:04:05.000Z`
- 回调金额与订单金额比对（含 1 分容差）后才入账，不信任上游报的数
- 部分退款落地后状态回到 `Paid` 而不是停在 `Refunding`，否则剩余余额再也退不出来
- 异步通知落 `notify_queues` 表，指数退避重试 5s→15s→60s→5m→15m，耗尽标记 `Exhausted`
- 配置集中在 `internal/config` 的 `Defaults`，全部从 `.env` 读，改完重启生效
- CSP 是 `script-src 'self'` + `style-src 'self'`，所以静态页面里**不能写 inline script 和 style 属性**，样式全走 class。支付宝那边原来返回的自提交表单里带 `<script>`，现在一律改成返回签名好的 GET URL，前端直接 `location.href`
- 限流按 IP 计数（`internal/httpx` 的滑动窗口 Map），全局限流挂在每条 `/v1/api` 路由的最外层，路由级限流再往里叠一层

## 测试

单测和 e2e 都是 `go test`，同一条命令：

| 位置 | 测什么 | 怎么测 |
|---|---|---|
| `internal/**/**_test.go` | service / lib 的逻辑分支 | 构造函数塞假 repo、假 HTTP、假配置，不碰库 |
| `internal/db/repos_test.go` | 数据层自己的约定 | 真临时 SQLite，直接调 repo |
| `internal/routes/*_test.go` | 路由与中间件链 | 真 handler + 真临时库，`httptest` 打 HTTP |

e2e 不碰外部第三方（支付宝 / PayPal / 汇率），只走 native 通道，所以离线可跑。装配在 `internal/routes/harness_test.go`：临时库文件 + 固定分组凭证 + 已登录管理员，用完删。

用例是从原来的 TS 版 spec 逐条移植过来的。测试文件头部的
`// Port of server/src/....spec.ts` 是**出处注释**，写的是这套代码重写前的真实路径 ——
那些 TS 文件已随后端重写删除，要对原文去 git 历史里翻。

两个坑：

- **限流是包级单例。** 同一个包里多个用例打同一路由会互相挤掉配额，所以 harness 给每个用例一个独立 IP。
- **日志会刷屏。** 用例会故意走失败分支，测试里 `logger.SetSilent(true)` 关掉输出（不改行为）。

e2e 的顶层用例名：`TestAuthE2E`、`TestAdminE2E`、`TestAppShell`、
`TestGatewayAPIKeyGuard`、`TestPaymentFlow`、`TestGlobalRateLimit`。

`-run` 是正则、不是全名匹配，父子用例用 `/` 连：

```bash
go test ./...                                              # 全部
go test ./internal/routes                                  # 只跑 e2e
go test ./internal/routes -run TestAuthE2E -v              # 单个顶层用例
go test ./internal/routes -run 'TestAuthE2E/.+logout.+' -v # 单个子用例
go test ./internal/services -run TestRefundServiceExecute -v
```

## 命令

```bash
go run .                                  # 开发模式（改完重新 run）
go run . create-admin <用户名> <密码>      # 建/改管理员，重复执行会重置密码
go build ./...                            # 编译检查
gofmt -l .                                # 格式检查（CI 会挡）
go vet ./...                              # 静态检查
go test ./...                             # 单测 + e2e
go mod tidy                               # 整理依赖
```

### 前端的类型检查

前端**仍然没有构建步骤** —— 浏览器加载的就是 `public/` 下的 `.js` 原文。
类型写在 JSDoc 注释里，由 `public/tsconfig.json` 的 `allowJs + checkJs` 检查；
注释在运行时不存在，所以不需要编译。`typescript` 是唯一的 devDependency，只为跑 `tsc`。
CI 里对应 `npm ci && npx tsc -p public --noEmit`。

```bash
npm ci           # 或 npm install；只有一个 devDependency
npm run typecheck
```

接口返回的形状写在 `public/api-types.d.ts`，是一份**独立**声明 —— 后端是 Go，
前端的 tsconfig 只看 `public/`，引不进 Go 的 struct。改了 Go 那边的 DTO 字段
这里要一起改，否则 tsc 不会报，只会在运行时渲染出 `undefined`。
日期字段过 JSON 之后是 `string` 而不是 `Date`。

`public/tsconfig.json` 的 `include` 就是被检查的文件清单，加新页面记得一起加。
目前覆盖：`app.js`、`index.js`、`login.js`、`orders.js`、`notifications.js`、
`groups.js`、`sandbox.js`、`cashier.js`。
`theme.js` 不在内（classic script，契约写在 `globals.d.ts`）。

## 注意

- `JWT_SECRET` 用 `openssl rand -hex 32` 生成
- `apiKey` **明文存库**并在「分组与 Key」页面直接显示 —— 自用假数据才这么干，别放真实凭证
- 入站不验签，Key 明文走在请求头里，安全性全靠 HTTPS —— 别在裸 HTTP 的公网上跑
- `.env` 别提交（`.gitignore` 已配）
- `ENABLE_SANDBOX=false` 会关掉沙箱确认、测试下单、重置数据这几个接口

## License

UNLICENSED — 自用。
