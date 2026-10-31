package db

import (
	"database/sql"
	"strings"
	"time"
)

type SandboxTask struct {
	ID        int64     `json:"id"`
	OrderNo   string    `json:"orderNo"`
	Scenario  string    `json:"scenario"`
	Status    string    `json:"status"`
	Amount    int64     `json:"-"`
	RunAt     time.Time `json:"runAt"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	LastError string    `json:"lastError,omitempty"`
}

type SandboxTaskRepo struct{ ex executor }

func (s *Store) SandboxTasks() *SandboxTaskRepo   { return &SandboxTaskRepo{ex: s.db} }
func (t *TxScope) SandboxTasks() *SandboxTaskRepo { return &SandboxTaskRepo{ex: t.Orders.ex} }

const sandboxTaskColumns = `"id","orderNo","scenario","status","amount","runAt","createdAt","updatedAt","lastError"`

func scanSandboxTask(row scanner) (*SandboxTask, error) {
	var task SandboxTask
	var run, created, updated string
	if err := row.Scan(&task.ID, &task.OrderNo, &task.Scenario, &task.Status, &task.Amount, &run, &created, &updated, &task.LastError); err != nil {
		return nil, err
	}
	task.RunAt, task.CreatedAt, task.UpdatedAt = fromSQL(run), fromSQL(created), fromSQL(updated)
	return &task, nil
}

func (r *SandboxTaskRepo) Find(id int64) (*SandboxTask, error) {
	task, err := scanSandboxTask(r.ex.QueryRow(`SELECT `+sandboxTaskColumns+` FROM "sandbox_tasks" WHERE "id" = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return task, err
}

func (r *SandboxTaskRepo) Insert(task *SandboxTask) error {
	now := nowSQL()
	result, err := r.ex.Exec(`INSERT INTO "sandbox_tasks" ("orderNo","scenario","status","amount","runAt","createdAt","updatedAt") VALUES (?,?,?,?,?,?,?)`, task.OrderNo, task.Scenario, "pending", task.Amount, toSQL(task.RunAt), now, now)
	if err != nil {
		return err
	}
	task.ID, err = result.LastInsertId()
	task.Status = "pending"
	task.CreatedAt, task.UpdatedAt = fromSQL(now), fromSQL(now)
	return err
}

func (r *SandboxTaskRepo) List(orderNo string) ([]*SandboxTask, error) {
	return r.ListScoped(orderNo, nil)
}

func (r *SandboxTaskRepo) ListScoped(orderNo string, scope []int64) ([]*SandboxTask, error) {
	query := `SELECT ` + sandboxTaskColumns + ` FROM "sandbox_tasks"`
	var args []any
	var conditions []string
	if orderNo != "" {
		conditions = append(conditions, `"orderNo" = ?`)
		args = append(args, orderNo)
	}
	if scope != nil {
		if len(scope) == 0 {
			conditions = append(conditions, "1=0")
		} else {
			placeholders := make([]string, len(scope))
			for i, id := range scope {
				placeholders[i] = "?"
				args = append(args, id)
			}
			conditions = append(conditions, `"orderNo" IN (SELECT "orderNo" FROM "payment_orders" WHERE "groupId" IN (`+strings.Join(placeholders, ",")+`))`)
		}
	}
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, " AND ")
	}
	query += ` ORDER BY "id" DESC LIMIT 100`
	rows, err := r.ex.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*SandboxTask, 0)
	for rows.Next() {
		item, err := scanSandboxTask(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *SandboxTaskRepo) Due() ([]*SandboxTask, error) {
	rows, err := r.ex.Query(`SELECT `+sandboxTaskColumns+` FROM "sandbox_tasks" WHERE ("status" = 'pending' AND "runAt" <= ?) OR ("status" = 'running' AND "updatedAt" < ?) ORDER BY "runAt","id" LIMIT 50`, nowSQL(), toSQL(time.Now().Add(-2*time.Minute)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*SandboxTask, 0)
	for rows.Next() {
		item, err := scanSandboxTask(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *SandboxTaskRepo) Claim(id int64) (bool, error) {
	result, err := r.ex.Exec(`UPDATE "sandbox_tasks" SET "status"='running',"updatedAt"=? WHERE "id"=? AND (("status"='pending' AND "runAt"<=?) OR ("status"='running' AND "updatedAt"<?))`, nowSQL(), id, nowSQL(), toSQL(time.Now().Add(-2*time.Minute)))
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func (r *SandboxTaskRepo) Finish(id int64, status, message string) error {
	_, err := r.ex.Exec(`UPDATE "sandbox_tasks" SET "status"=?,"lastError"=?,"updatedAt"=? WHERE "id"=?`, status, message, nowSQL(), id)
	return err
}

func (r *SandboxTaskRepo) ActiveForOrder(orderNo string) (bool, error) {
	var count int
	err := r.ex.QueryRow(`SELECT COUNT(*) FROM "sandbox_tasks" WHERE "orderNo"=? AND "status" IN ('pending','running')`, orderNo).Scan(&count)
	return count > 0, err
}
