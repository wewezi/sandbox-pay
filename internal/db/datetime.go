package db

import "time"

// SQLite 没有日期类型，datetime 列存的是文本。
//
// 格式和已有库里的存量数据保持一致：`YYYY-MM-DD HH:MM:SS.sss`，UTC，不带时区后缀。
//
// 别改成 ISO（带 T 和 Z）—— 排序是按文本比的，空格 (0x20) < 'T' (0x54)，
// 两种格式混在同一列里 ORDER BY createdAt 就乱了。

// sqlTimeLayout 写库用的格式，和原 TypeORM 时代的数据对齐。
const sqlTimeLayout = "2006-01-02 15:04:05.000"

// toSQL 文本 → 写库。一律 UTC。
func toSQL(t time.Time) string {
	return t.UTC().Format(sqlTimeLayout)
}

// nowSQL 当前时间的 SQL 文本。
func nowSQL() string { return toSQL(time.Now()) }

// fromSQL 读库。兼容三种历史格式：
//   - `YYYY-MM-DD HH:MM:SS.sss`（我们写的）
//   - `YYYY-MM-DD HH:MM:SS`（SQLite datetime('now') 的默认值）
//   - ISO 8601（极早期数据）
//
// 不带时区后缀的一律按 UTC 解析 —— 列里存的本来就是 UTC。
func fromSQL(raw string) time.Time {
	s := raw
	for _, layout := range []string{
		sqlTimeLayout,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05.000Z07:00",
		"2006-01-02T15:04:05Z07:00",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
