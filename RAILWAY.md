# Railway 部署指南

本项目已改造为支持 Railway.com 部署（PostgreSQL）。

## 架构

```
┌─────────────────────────────────────────────────┐
│  Railway Project                                │
│                                                 │
│  ┌──────────┐  ┌──────────┐  ┌──────────────┐  │
│  │ PostgreSQL│  │  Server  │  │    Client    │  │
│  │ (插件)    │←─│ NestJS   │←─│ nginx + SPA  │  │
│  │          │  │ :3000    │  │ :80          │  │
│  └──────────┘  └──────────┘  └──────────────┘  │
│       ↑              ↑              ↑           │
│    DB_HOST      RAILWAY_*     BACKEND_URL       │
│    DB_PORT      内部域名       内部域名          │
└─────────────────────────────────────────────────┘
```

## 部署步骤

### 1. 创建 Railway 项目

1. 登录 [railway.com](https://railway.com)
2. New Project → 添加 **PostgreSQL** 插件（Provision PostgreSQL）
3. 记下 PostgreSQL 插件自动生成的环境变量（后续会用到）

### 2. 部署后端 (Server)

1. **New Service** → **GitHub Repo** → 选择你的 fork/仓库
2. **Root Directory** 设为 `server`
3. Railway 会自动检测 `server/Dockerfile` 并构建
4. 在 **Variables** 中配置环境变量：

| 变量 | 说明 | 来源 |
|---|---|---|
| `DB_HOST` | 数据库地址 | 引用 PostgreSQL 插件变量 |
| `DB_PORT` | 数据库端口 | 引用 PostgreSQL 插件变量 |
| `DB_USERNAME` | 数据库用户 | 引用 PostgreSQL 插件变量 |
| `DB_PASSWORD` | 数据库密码 | 引用 PostgreSQL 插件变量 |
| `DB_DATABASE` | 数据库名 | 引用 PostgreSQL 插件变量 |
| `JWT_SECRET` | JWT 密钥 | `openssl rand -hex 32` 生成 |
| `GATEWAY_SECRET` | 网关签名密钥 | `openssl rand -hex 32` 生成 |
| `ENCRYPTION_KEY` | 加密密钥 | `openssl rand -hex 32` 生成 |
| `DB_SYNCHRONIZE` | 设为 `false` | 固定值 |
| `CLIENT_URL` | 前端公网 URL | 部署前端后填入 |
| `ENABLE_SANDBOX` | 沙箱开关 | 按需 |

> 💡 Railway 支持变量引用：`DB_HOST=${{PostgreSQL.PGHOST}}`

5. 部署后在 **Settings → Networking → Public Networking** 生成公网域名
6. 在 Railway Shell 中执行初始化：
   ```bash
   pnpm migration:run
   pnpm create:admin admin your_password
   ```

### 3. 部署前端 (Client)

1. **New Service** → **GitHub Repo** → 同一仓库
2. **Root Directory** 设为 `client`
3. 配置环境变量：

| 变量 | 说明 |
|---|---|
| `BACKEND_URL` | 后端服务的内部/公网地址 |

4. Railway 内部通信可使用后端的内部域名（如 `http://server.railway.internal:3000`）
5. 如果前端也需要公网访问，在 **Settings → Networking** 中生成公网域名
6. 将前端公网 URL 回填到后端的 `CLIENT_URL` 变量

### 4. 验证

- 前端：`https://your-client.up.railway.app`
- 后端 API：`https://your-server.up.railway.app/v1/api/`
- 健康检查：`https://your-server.up.railway.app/health`
- 管理后台：登录后访问 `/admin/docs`

## 本地开发（Docker Compose）

```bash
cp server/.env.example .env
# 按需编辑 .env 中的环境变量
docker compose up -d
# 访问 http://localhost
```

## 与原版的差异

| 项目 | 原版 | Railway 版 |
|---|---|---|
| 数据库 | MySQL 8 | PostgreSQL 16 |
| TypeORM driver | `mysql2` | `pg` |
| 用户角色字段 | `enum` 类型 | `varchar` 类型 |
| nginx 代理目标 | 硬编码 `server:3000` | 环境变量 `BACKEND_URL` |
| 迁移文件 | MySQL 语法 | PostgreSQL 语法 |
