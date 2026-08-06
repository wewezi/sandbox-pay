#!/bin/bash
set -e

# ── 自动生成密钥（如果未设置）──
generate_secret() {
  openssl rand -hex 32 2>/dev/null || head -c 64 /dev/urandom | od -An -tx1 | tr -d ' \n'
}
export JWT_SECRET="${JWT_SECRET:-$(generate_secret)}"
export GATEWAY_SECRET="${GATEWAY_SECRET:-$(generate_secret)}"
export ENCRYPTION_KEY="${ENCRYPTION_KEY:-$(generate_secret)}"

# ── PostgreSQL 路径 ──
PG_BIN="/usr/lib/postgresql/15/bin"
PG_VOLUME="/var/lib/postgresql/data"
PGDATA="$PG_VOLUME/pgdata"

# 初始化数据库（仅首次）
if [ ! -f "$PGDATA/PG_VERSION" ]; then
  echo "[entrypoint] Initializing PostgreSQL..."
  mkdir -p "$PGDATA"
  chown -R postgres:postgres "$PG_VOLUME"
  su - postgres -c "$PG_BIN/initdb -D $PGDATA --encoding=UTF8 --locale=C"

  cat > "$PGDATA/pg_hba.conf" <<EOF
local all all trust
host  all all 127.0.0.1/32 trust
host  all all ::1/128 trust
EOF
  cat >> "$PGDATA/postgresql.conf" <<EOF
listen_addresses = '127.0.0.1'
port = 5432
EOF
fi

# 启动 PostgreSQL
echo "[entrypoint] Starting PostgreSQL..."
su - postgres -c "$PG_BIN/pg_ctl -D $PGDATA -l /var/log/postgresql/postgres.log start"
sleep 1

for i in $(seq 1 30); do
  su - postgres -c "$PG_BIN/pg_isready -q" && break
  echo "[entrypoint] Waiting for PostgreSQL... ($i)"
  sleep 1
done

# 创建数据库
su - postgres -c "$PG_BIN/psql -tc \"SELECT 1 FROM pg_database WHERE datname='sandbox_pay'\" | grep -q 1" || \
  su - postgres -c "$PG_BIN/createdb sandbox_pay"

echo "[entrypoint] PostgreSQL ready."

# 运行迁移
echo "[entrypoint] Running migrations..."
cd /app/server
node -e "
const { AppDataSource } = require('./dist/data-source');
AppDataSource.initialize()
  .then(async () => {
    await AppDataSource.runMigrations();
    console.log('[entrypoint] Migrations done.');
    await AppDataSource.destroy();
    process.exit(0);
  })
  .catch(e => { console.error(e); process.exit(1); });
"

# 启动 NestJS
echo "[entrypoint] Starting server on port ${PORT:-3000}..."
cd /app/server
exec node dist/main.js
