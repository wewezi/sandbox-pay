package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // 纯 Go 驱动，注册 "sqlite"
)

// executor 让 repo 的查询同时跑在 *sql.DB 和 *sql.Tx 上 ——
// 事务里的「读-判状态-写」必须用同一个连接，跨连接就没有隔离了。
type executor interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// Store SQLite 连接 + 挂在它上面的各表 repo。
//
// SQLite 是单写者，写事务之间本来就串行，所以不需要（也没有）
// SELECT ... FOR UPDATE 那种行锁 ——「读出来判状态再写回去」在事务里天然是原子的。
type Store struct {
	db *sql.DB

	Orders   *OrdersRepo
	ApiKeys  *ApiKeysRepo
	Groups   *GroupsRepo
	Notifies *NotifyRepo
	Users    *UsersRepo
}

// Open 打开（必要时建表）数据库。
//
// dbFile 相对路径按 packageRoot 解析，留空用 data/sandbox-pay.db。
func Open(dbFile, packageRoot string) (*Store, error) {
	path, err := resolvePath(dbFile, packageRoot)
	if err != nil {
		return nil, err
	}

	// WAL：读写不互相阻塞。沙箱单进程也用得上，崩了之后恢复更快。
	// 外键约束默认是关的，payment_orders.groupId 的 ON DELETE SET NULL 要靠它。
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// modernc 的驱动是单连接语义下的最安全配置：限制成 1 个连接，
	// 避免多连接并发写时 SQLITE_BUSY 打架（单进程沙箱足够）。
	conn.SetMaxOpenConns(1)

	if _, err := conn.Exec(schemaSQL); err != nil {
		conn.Close()
		return nil, fmt.Errorf("建表失败: %w", err)
	}
	return newStore(conn), nil
}

func newStore(conn *sql.DB) *Store {
	s := &Store{db: conn}
	s.bind()
	return s
}

func (s *Store) bind() {
	s.Orders = &OrdersRepo{ex: s.db}
	s.ApiKeys = &ApiKeysRepo{ex: s.db}
	s.Groups = &GroupsRepo{ex: s.db}
	s.Notifies = &NotifyRepo{ex: s.db}
	s.Users = &UsersRepo{ex: s.db}
}

// resolvePath 解析库文件路径并确保目录存在。
func resolvePath(raw, packageRoot string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		raw = "data/sandbox-pay.db"
	}
	path := raw
	if !filepath.IsAbs(path) {
		path = filepath.Join(packageRoot, raw)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return path, nil
}

// Close 收尾用。
func (s *Store) Close() error { return s.db.Close() }

// Do 在写事务里跑 fn。任一步出错就回滚，错误原样返回。
//
// 回调必须同步做完「读-判状态-写」—— 事务里不能等 goroutine/网络，
// 发通知这类动作放到 Do 返回之后（和原版「事务外发通知」一致）。
func (s *Store) Do(fn func(tx *TxScope) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	scope := &TxScope{
		Orders:   &OrdersRepo{ex: tx},
		ApiKeys:  &ApiKeysRepo{ex: tx},
		Groups:   &GroupsRepo{ex: tx},
		Notifies: &NotifyRepo{ex: tx},
		Users:    &UsersRepo{ex: tx},
	}
	if err := fn(scope); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// TxScope 事务内的各表 repo，全部绑定到同一个 Tx。
type TxScope struct {
	Orders   *OrdersRepo
	ApiKeys  *ApiKeysRepo
	Groups   *GroupsRepo
	Notifies *NotifyRepo
	Users    *UsersRepo
}

// Runner 跑一个事务的最小接口。service 依赖它，测试里塞直接回调的假实现。
type Runner interface {
	Do(fn func(tx *TxScope) error) error
}

// Where 把若干「条件 / 参数」累积成 WHERE 子句。
type Where struct {
	clauses []string
	args    []any
}

// Add 追加一个条件。
func (w *Where) Add(clause string, args ...any) {
	w.clauses = append(w.clauses, clause)
	w.args = append(w.args, args...)
}

// AddIf 条件值非 nil 且非空串时才加，省掉一堆 if。
// 数字 0 会保留 —— 和原版 addIf 的 undefined/null/” 判定一致。
//
// 装箱进 any 的 typed-nil 指针（(*int64)(nil) 这类）type switch 走不到
// `case nil`，必须单独判掉：否则会绑出 `col = NULL` 这种永假条件，
// Stats(nil)/全量查询就一条行都匹配不到了。
func (w *Where) AddIf(cond any, clause string, args ...any) {
	switch v := cond.(type) {
	case nil:
		return
	case string:
		if v == "" {
			return
		}
	case *string:
		if v == nil || *v == "" {
			return
		}
	case *int:
		if v == nil {
			return
		}
	case *int64:
		if v == nil {
			return
		}
	case *float64:
		if v == nil {
			return
		}
	}
	w.Add(clause, args...)
}

// SQL 空条件返回空串，避免拼出 WHERE 后面没内容的语句。
func (w *Where) SQL() string {
	if len(w.clauses) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(w.clauses, " AND ")
}

// Args 条件对应的参数。
func (w *Where) Args() []any { return w.args }

// bindValue 把模型值转成可绑定的 SQL 值：time → 文本，bool → 0/1，nil → NULL。
func bindValue(v any) any {
	switch t := v.(type) {
	case nil:
		return nil
	case time.Time:
		return toSQL(t)
	case bool:
		if t {
			return 1
		}
		return 0
	case int:
		return int64(t)
	case *int:
		if t == nil {
			return nil
		}
		return int64(*t)
	case *string:
		if t == nil {
			return nil
		}
		return *t
	case *float64:
		if t == nil {
			return nil
		}
		return *t
	case *int64:
		if t == nil {
			return nil
		}
		return *t
	case *bool:
		if t == nil {
			return nil
		}
		if *t {
			return 1
		}
		return 0
	default:
		return v
	}
}

// setClause 从 patch 拼出 `"a" = ?, "b" = ?` 和对应的参数。
//
// 值为 nil 表示写 NULL（显式出现的 key 就会写，清空 lastError 之类靠这个）。
// automatically 给的列（updatedAt）由调用方塞进 patch。
func setClause(patch map[string]any) (string, []any) {
	if len(patch) == 0 {
		return "", nil
	}
	cols := make([]string, 0, len(patch))
	args := make([]any, 0, len(patch))
	for k, v := range patch {
		cols = append(cols, fmt.Sprintf("%q = ?", k))
		args = append(args, bindValue(v))
	}
	return strings.Join(cols, ", "), args
}
