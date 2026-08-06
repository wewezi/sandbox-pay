# Docker 一键部署

零外部依赖，内置 PostgreSQL。

## 快速启动

```bash
docker build -t sandbox-pay .
docker run -d -p 3000:3000 --name sandbox-pay sandbox-pay
```

访问 http://localhost:3000

## 首次使用

```bash
# 创建管理员账号
docker exec -it sandbox-pay bash -c \
  "cd /app/server && node -e \"
    const { AppDataSource } = require('./dist/data-source');
    AppDataSource.initialize().then(async () => {
      const bcrypt = require('bcrypt');
      const repo = AppDataSource.getRepository('User');
      const hash = await bcrypt.hash('admin123', 10);
      await repo.save({ username: 'admin', password: hash, role: 'SuperAdmin' });
      console.log('Admin created: admin / admin123');
      await AppDataSource.destroy();
    });
  \""
```

## 持久化数据

```bash
# 使用 Docker Volume 保存数据库
docker run -d -p 3000:3000 \
  -v sandbox-pay-data:/var/lib/postgresql/data \
  --name sandbox-pay \
  sandbox-pay
```

## 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PORT` | `3000` | 服务端口 |
| `JWT_SECRET` | 自动生成 | JWT 密钥 |
| `GATEWAY_SECRET` | 自动生成 | 商户网关签名密钥 |
| `ENABLE_API_DOCS` | `false` | 是否开启 Swagger |
| `ENABLE_SANDBOX` | `true` | 是否开启沙箱接口 |

> 内置 PostgreSQL 数据库变量（DB_*）已预配置，无需修改。
