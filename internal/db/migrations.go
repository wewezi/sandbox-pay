package db

import (
	"database/sql"
	"fmt"
)

// migrate upgrades transactionally, retaining business history and legacy
// callback signing snapshots while replacing authentication tokens with hashes.
func migrate(conn *sql.DB) error {
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version > 5 {
		return fmt.Errorf("数据库版本 %d 比当前程序支持的版本 5 更新", version)
	}
	if version < 1 {
		if err := migrateVersion1(tx); err != nil {
			return err
		}
	}
	if version < 2 {
		if _, err := tx.Exec(`ALTER TABLE "payment_orders" ADD COLUMN "clientIp" TEXT;
ALTER TABLE "payment_orders" ADD COLUMN "clientLocation" TEXT;
PRAGMA user_version = 2;`); err != nil {
			return err
		}
	}
	if version < 3 {
		if err := migrateVersion3(tx); err != nil {
			return err
		}
	}
	if version < 4 {
		if err := migrateVersion4(tx); err != nil {
			return err
		}
	}
	if version < 5 {
		for _, migration := range []func(*sql.Tx) error{migrateTeamVersion5, migrateDiagnosticsVersion5, migrateSandboxVersion5, migrateOperationsVersion5} {
			if err := migration(tx); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`PRAGMA user_version = 5`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// migrateVersion1 remains a separate step so v0 constraints and v2 request
// metadata are committed together or rolled back together.
func migrateVersion1(tx *sql.Tx) error {
	rows, err := tx.Query(`PRAGMA table_info("payment_orders")`)
	if err != nil {
		return err
	}
	hasProviderID := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		hasProviderID = hasProviderID || name == "providerOrderId"
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if !hasProviderID {
		if _, err := tx.Exec(`ALTER TABLE "payment_orders" ADD COLUMN "providerOrderId" varchar(128)`); err != nil {
			return err
		}
	}
	checks := []struct{ query, message string }{
		{`SELECT COUNT(*) FROM (SELECT 1 FROM "payment_orders" WHERE "groupId" IS NOT NULL AND "externalOrderNo" IS NOT NULL AND "externalOrderNo" <> '' GROUP BY "groupId", "externalOrderNo" HAVING COUNT(*) > 1)`, "存在同分组同 externalOrderNo 的重复订单"},
		{`SELECT COUNT(*) FROM (SELECT 1 FROM "payment_orders" WHERE "payMethod" IN ('alipay', 'paypal') AND "thirdPartyTradeNo" IS NOT NULL AND "thirdPartyTradeNo" <> '' GROUP BY "payMethod", "thirdPartyTradeNo" HAVING COUNT(*) > 1)`, "存在同渠道同上游交易号的重复订单"},
	}
	for _, check := range checks {
		var count int
		if err := tx.QueryRow(check.query).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("%s；请先备份数据库并人工核对重复记录后重试，升级未删除数据", check.message)
		}
	}
	if _, err := tx.Exec(`
CREATE UNIQUE INDEX "UQ_orders_group_external" ON "payment_orders" ("groupId", "externalOrderNo")
  WHERE "groupId" IS NOT NULL AND "externalOrderNo" IS NOT NULL AND "externalOrderNo" <> '';
CREATE UNIQUE INDEX "UQ_orders_provider" ON "payment_orders" ("providerOrderId")
  WHERE "providerOrderId" IS NOT NULL AND "providerOrderId" <> '';
CREATE UNIQUE INDEX "UQ_orders_trade" ON "payment_orders" ("payMethod", "thirdPartyTradeNo")
  WHERE "payMethod" IN ('alipay', 'paypal') AND "thirdPartyTradeNo" IS NOT NULL AND "thirdPartyTradeNo" <> '';
CREATE TABLE "refund_operations" (
  "id" varchar(64) PRIMARY KEY NOT NULL,
  "requestId" varchar(64) NOT NULL DEFAULT '',
  "reason" text NOT NULL DEFAULT '',
  "orderNo" varchar(64) NOT NULL REFERENCES "payment_orders" ("orderNo") ON DELETE CASCADE,
  "amount" integer NOT NULL CHECK ("amount" > 0),
  "tradeNo" varchar(128),
  "status" varchar(16) NOT NULL,
  "createdAt" datetime NOT NULL,
  "updatedAt" datetime NOT NULL
);
CREATE UNIQUE INDEX "UQ_refunds_trade" ON "refund_operations" ("orderNo", "tradeNo") WHERE "tradeNo" IS NOT NULL AND "tradeNo" <> '';
CREATE UNIQUE INDEX "UQ_refunds_active" ON "refund_operations" ("orderNo") WHERE "status" IN ('processing', 'pending', 'unknown');
PRAGMA user_version = 1;
`); err != nil {
		return err
	}
	return nil
}
