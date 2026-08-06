# ═══════════════════════════════════════════════════════════
#  Sandbox Pay — 一键部署 Dockerfile
#  内置 PostgreSQL，零外部依赖
# ═══════════════════════════════════════════════════════════

# ── Stage 1: 构建前端 ──
FROM node:22-alpine AS build-client
WORKDIR /app
COPY client/package.json ./
RUN npm install
COPY client/ .
RUN npx vite build

# ── Stage 2: 构建后端 ──
FROM node:22-alpine AS build-server
WORKDIR /app
COPY server/package.json ./
RUN npm install
COPY server/ .
RUN rm -rf dist && npx nest build

# ── Stage 3: 运行时（Node + PostgreSQL）──
FROM node:22-bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends postgresql postgresql-client curl && \
    rm -rf /var/lib/apt/lists/* && \
    mkdir -p /var/lib/postgresql/data /var/log/postgresql && \
    chown -R postgres:postgres /var/lib/postgresql /var/log/postgresql

WORKDIR /app

COPY --from=build-server /app/dist ./server/dist
COPY --from=build-server /app/node_modules ./server/node_modules
COPY --from=build-server /app/package.json ./server/
COPY --from=build-client /app/dist ./client

COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

ENV NODE_ENV=production
ENV PORT=3000
ENV DB_HOST=127.0.0.1
ENV DB_PORT=5432
ENV DB_USERNAME=postgres
ENV DB_PASSWORD=""
ENV DB_DATABASE=sandbox_pay
ENV DB_SYNCHRONIZE=false
ENV DB_LOGGING=false
ENV ENABLE_API_DOCS=false
ENV ENABLE_SANDBOX=true

EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD curl -f http://localhost:${PORT}/v1/health || exit 1

CMD ["/docker-entrypoint.sh"]
