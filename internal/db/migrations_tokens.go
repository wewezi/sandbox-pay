package db

import "database/sql"

func migrateVersion3(tx *sql.Tx) error {
	if _, err := tx.Exec(`ALTER TABLE "api_keys" ADD COLUMN "keyPrefix" TEXT NOT NULL DEFAULT '';
ALTER TABLE "api_keys" ADD COLUMN "keySuffix" TEXT NOT NULL DEFAULT '';
ALTER TABLE "groups" ADD COLUMN "webhookSecret" TEXT NOT NULL DEFAULT '';
ALTER TABLE "payment_orders" ADD COLUMN "signingSecret" TEXT;
ALTER TABLE "payment_orders" ADD COLUMN "signatureVersion" INTEGER NOT NULL DEFAULT 1;
UPDATE "payment_orders" SET "signingSecret" = (
  SELECT "key" FROM "api_keys" WHERE "id" = "payment_orders"."apiKeyId"
) WHERE "notifyUrl" IS NOT NULL AND "notifyUrl" <> '' AND "apiKeyId" IS NOT NULL;`); err != nil {
		return err
	}

	// Read before rewriting so every historical callback keeps its original
	// signing key, including callbacks for individually disabled API tokens.
	type originalKey struct {
		id  int64
		key string
	}
	rows, err := tx.Query(`SELECT "id", "key" FROM "api_keys"`)
	if err != nil {
		return err
	}
	var keys []originalKey
	for rows.Next() {
		var key originalKey
		if err := rows.Scan(&key.id, &key.key); err != nil {
			rows.Close()
			return err
		}
		keys = append(keys, key)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, key := range keys {
		prefix, suffix := keyDisplayParts(key.key)
		if _, err := tx.Exec(`UPDATE "api_keys" SET "key" = ?, "keyPrefix" = ?, "keySuffix" = ? WHERE "id" = ?`,
			HashAPIKey(key.key), prefix, suffix, key.id); err != nil {
			return err
		}
	}

	rows, err = tx.Query(`SELECT "id" FROM "groups"`)
	if err != nil {
		return err
	}
	var groupIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		groupIDs = append(groupIDs, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range groupIDs {
		secret, err := generateWebhookSecret()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE "groups" SET "webhookSecret" = ? WHERE "id" = ?`, secret, id); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`PRAGMA user_version = 3`)
	return err
}
