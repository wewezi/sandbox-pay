package db

// 建表 DDL。
//
// 首次建库执行基础表；已有库由 migrations.go 进行事务内升级。

const schemaSQL = `
CREATE TABLE IF NOT EXISTS "users" (
  "id" integer PRIMARY KEY AUTOINCREMENT NOT NULL,
  "username" varchar(20) NOT NULL,
  "password" varchar(100) NOT NULL,
  "role" varchar(20) NOT NULL DEFAULT ('Admin'),
  "createdAt" datetime NOT NULL DEFAULT (datetime('now')),
  "updatedAt" datetime NOT NULL DEFAULT (datetime('now')),
  CONSTRAINT "UQ_users_username" UNIQUE ("username")
);

CREATE TABLE IF NOT EXISTS "groups" (
  "id" integer PRIMARY KEY AUTOINCREMENT NOT NULL,
  "name" varchar NOT NULL,
  "isActive" boolean NOT NULL DEFAULT (1),
  "createdAt" datetime NOT NULL DEFAULT (datetime('now')),
  "updatedAt" datetime NOT NULL DEFAULT (datetime('now'))
);

-- 凭证从表。一个分组可以签发多把 Key（轮换 / 多环境），
-- 入站按 key 全局查找，所以 UNIQUE 在 key 上而不是 (groupId, key)。
-- key 在版本 3 迁移后保存单向哈希，前后缀和独立回调密钥由迁移增加。
CREATE TABLE IF NOT EXISTS "api_keys" (
  "id" integer PRIMARY KEY AUTOINCREMENT NOT NULL,
  "groupId" integer NOT NULL,
  "name" varchar NOT NULL,
  "key" varchar NOT NULL,
  "isActive" boolean NOT NULL DEFAULT (1),
  "lastUsedAt" datetime,
  "createdAt" datetime NOT NULL DEFAULT (datetime('now')),
  "updatedAt" datetime NOT NULL DEFAULT (datetime('now')),
  CONSTRAINT "UQ_api_keys_key" UNIQUE ("key"),
  CONSTRAINT "FK_api_keys_groupId" FOREIGN KEY ("groupId")
    REFERENCES "groups" ("id") ON DELETE CASCADE ON UPDATE NO ACTION
);

CREATE TABLE IF NOT EXISTS "payment_orders" (
  "id" varchar(32) PRIMARY KEY NOT NULL,
  "orderNo" varchar(64) NOT NULL,
  "amount" integer NOT NULL,
  "productName" varchar NOT NULL,
  "payMethod" varchar(32) NOT NULL,
  "status" integer NOT NULL DEFAULT (0),
  "createdAt" datetime NOT NULL DEFAULT (datetime('now')),
  "updatedAt" datetime NOT NULL DEFAULT (datetime('now')),
  "expireAt" datetime NOT NULL,
  "payAt" datetime,
  "exchangeRate" real,
  "foreignAmount" integer,
  "foreignCurrency" varchar(10),
  "externalOrderNo" varchar(64),
  "thirdPartyTradeNo" varchar(128),
  "providerOrderId" varchar(128),
  "refundedAmount" integer NOT NULL DEFAULT (0),
  "refundTradeNo" varchar(128),
  "refundAt" datetime,
  "channelCost" integer NOT NULL DEFAULT (0),
  "fee" integer NOT NULL DEFAULT (0),
  "settleAmount" integer NOT NULL DEFAULT (0),
  "returnUrl" varchar(512),
  "notifyUrl" varchar(512),
  "groupId" integer,
  "apiKeyId" integer,
  "userId" integer NOT NULL DEFAULT (1),
  CONSTRAINT "UQ_payment_orders_orderNo" UNIQUE ("orderNo"),
  CONSTRAINT "FK_payment_orders_groupId" FOREIGN KEY ("groupId")
    REFERENCES "groups" ("id") ON DELETE SET NULL ON UPDATE NO ACTION,
  CONSTRAINT "FK_payment_orders_apiKeyId" FOREIGN KEY ("apiKeyId")
    REFERENCES "api_keys" ("id") ON DELETE SET NULL ON UPDATE NO ACTION
);

CREATE TABLE IF NOT EXISTS "notify_queues" (
  "id" integer PRIMARY KEY AUTOINCREMENT NOT NULL,
  "orderNo" varchar(64),
  "url" varchar(512) NOT NULL,
  "body" text NOT NULL,
  "signature" text NOT NULL,
  "retryCount" integer NOT NULL DEFAULT (0),
  "status" integer NOT NULL DEFAULT (0),
  "createdAt" datetime NOT NULL DEFAULT (datetime('now')),
  "lastAttemptAt" datetime,
  "lastError" text
);

CREATE INDEX IF NOT EXISTS "IDX_orders_status" ON "payment_orders" ("status");
CREATE INDEX IF NOT EXISTS "IDX_orders_expireAt" ON "payment_orders" ("expireAt");
CREATE INDEX IF NOT EXISTS "IDX_orders_externalOrderNo" ON "payment_orders" ("externalOrderNo");
CREATE INDEX IF NOT EXISTS "IDX_orders_groupId" ON "payment_orders" ("groupId");
CREATE INDEX IF NOT EXISTS "IDX_orders_group_status" ON "payment_orders" ("groupId", "status");
CREATE INDEX IF NOT EXISTS "IDX_orders_group_created" ON "payment_orders" ("groupId", "createdAt");
CREATE INDEX IF NOT EXISTS "IDX_api_keys_groupId" ON "api_keys" ("groupId");
CREATE INDEX IF NOT EXISTS "IDX_notify_status" ON "notify_queues" ("status");
CREATE INDEX IF NOT EXISTS "IDX_notify_orderNo" ON "notify_queues" ("orderNo");
CREATE INDEX IF NOT EXISTS "IDX_notify_status_created" ON "notify_queues" ("status", "createdAt");
`
