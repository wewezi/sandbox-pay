# Mintweft

沙箱支付系统，支持下单、收银台、退款、异步回调、项目令牌和团队权限。支付宝与 PayPal 接入真实沙箱 API；native 用于本地模拟或人工确认收款。

后端 Go + chi，存储 SQLite（纯 Go 驱动），前端 TypeScript + Lit + Vite。前端嵌入 Go 二进制，运行时只需一个服务进程。

## 运行

需要 Go 1.26.3+、Node.js 22.12+。在项目根目录执行：

```sh
cp .env.example .env
node -e "console.log(require('node:crypto').randomBytes(32).toString('hex'))"
```

将生成的随机值填入 `.env` 的 `JWT_SECRET`，然后：

```sh
npm ci
npm run build
go run . create-admin admin your_password
go run .
```

访问 <http://localhost:18080>。`create-admin` 默认创建超级管理员，同名账号会重置密码和角色。

## 配置

配置写入 `.env`，进程环境变量优先。完整选项见 [.env.example](.env.example)。

| 配置 | 默认值 / 用途 |
|---|---|
| `PORT` | `18080` |
| `DB_FILE` | `data/sandbox-pay.db`，相对路径按服务工作目录解析 |
| `CLIENT_URL` | 本地留空；对外访问时填写固定 HTTPS 地址 |
| `ENABLE_SANDBOX` | `true`，控制测试下单、场景模拟和数据重置 |
| `DEMO_DATA_MODE` | `false`；开启后暂停自动回调、场景任务、订单过期和维护任务 |
| `NOTIFY_ALLOW_PRIVATE_NETWORK` | `false`；向 localhost 或内网接收方回调时设为 `true` |
| `TRUSTED_PROXY_CIDRS` | 留空；经反向代理部署时填写可信代理来源网段 |

渠道凭证留空时，该渠道不可用，但服务可以启动：

| 渠道 | 配置 |
|---|---|
| 支付宝沙箱 | `ALIPAY_APP_ID`、`ALIPAY_PRIVATE_KEY`、`ALIPAY_PUBLIC_KEY` |
| PayPal 沙箱 | `PAYPAL_CLIENT_ID`、`PAYPAL_CLIENT_SECRET`；仅支持 `sandbox` |

支付宝通知需要可访问的 `CLIENT_URL`，本地联调可使用 HTTPS 隧道。native 收款信息仅用于展示，系统不自动核验银行到账。

## 开发

完成首次构建后，在两个终端分别启动：

```sh
# 后端
go run .

# 前端
npm run dev
```

前端默认地址为 <http://localhost:5173>，将 `/v1/api` 和 `/health` 代理到后端。代理默认跟随 `.env` 的 `PORT`，可用 `VITE_API_TARGET` 覆盖。本地开发保持 `CLIENT_URL` 留空。

## API

在后台「API令牌」页创建令牌，完整值仅在创建时展示。接入方使用 `Authorization: Bearer <apiKey>`。

- 接入文档与调试：`/developer`
- OpenAPI：`/v1/api/openapi.json`，源码见 [openapi.json](internal/routes/openapi.json)
- Node.js 示例：[developer_example.mjs](internal/routes/developer_example.mjs)

```sh
curl http://localhost:18080/v1/api/gateway/pay \
  -H 'Authorization: Bearer YOUR_API_KEY' \
  -H 'Content-Type: application/json' \
  -d '{"amount":1,"productName":"测试订单","payMethod":"native","externalOrderNo":"demo-001"}'
```

金额单位为人民币元。下单重试复用 `externalOrderNo`；同一项目内金额与渠道一致时返回原订单。查单使用 `/v1/api/gateway/query`，传入 `orderNo` 或 `externalOrderNo` 中的一项。

回调使用项目独立密钥，按原始请求体、时间戳和 nonce 验证 HMAC-SHA256，并按事件去重；参考 Node.js 示例。

### 退款与重试

退款调用 `POST /v1/api/gateway/refund`。同一次退款重试须复用 `orderNo`、`requestId` 和金额；`pending` 表示尚未确认成功。无上游流水号的未知结果超过 30 分钟时需人工核对，不能换新 ID 重发。

## 部署

```sh
npm run build
go build -ldflags="-s -w" -o mintweft .
./mintweft
```

部署需要二进制、运行配置和持久化数据库；Windows 可输出为 `mintweft.exe`。通过 HTTPS 对外服务时设置 `NODE_ENV=production`、`CLIENT_URL` 和可信代理网段。

服务优先读取工作目录的 `web/dist/`，否则使用内嵌前端。更新前端后重新构建 Go。数据库启动时自动迁移，升级前备份数据库和配置；在线备份使用后台「运行维护」，不要在运行中只复制 `.db` 文件。

## 演示数据

此命令会清空应用数据。先停止服务并备份，设置 `ENABLE_SANDBOX=true`、`DEMO_DATA_MODE=true`，再执行：

```sh
go run . seed-demo --replace
go run .
```

恢复正常联调时，将 `DEMO_DATA_MODE` 改为 `false` 并重启。

## 检查

Go 编译和测试需要前端构建产物：

```sh
npm run build
go test ./...
go vet ./...
```

## 源码

- `internal/routes`、`internal/httpx`：chi 路由、鉴权和 HTTP 中间件。
- `internal/services`、`internal/db`、`internal/gateways`：业务、数据访问和支付渠道。
- `web/`：页面与组件；页面路径在 `web/services/router.ts` 和 `internal/routes/static.go` 登记。
- IP 归属地使用[离线数据](internal/geoip/data/README.md)。

[MIT License](LICENSE)。
