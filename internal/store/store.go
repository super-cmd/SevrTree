// Package store 封装 SQLite 持久化：受管任务配置、系统设置。
// 使用纯 Go 驱动 modernc.org/sqlite，无 CGO 依赖，便于单文件分发。
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound 表示查询的记录不存在。
var ErrNotFound = errors.New("记录不存在")

// Task 描述一个受管任务（可执行程序或脚本）。
type Task struct {
	ID                 int64             `json:"id"`
	Name               string            `json:"name"`
	Exe                string            `json:"exe"`
	Args               []string          `json:"args"`
	WorkDir            string            `json:"workDir"`
	Env                map[string]string `json:"env"`
	DelaySeconds       int               `json:"delaySeconds"`
	SortOrder          int               `json:"sortOrder"`
	Enabled            bool              `json:"enabled"`
	AutoStart          bool              `json:"autoStart"`
	DependsOn          []int64           `json:"dependsOn"`
	HealthPort         int               `json:"healthPort"`
	StopTimeoutSeconds int               `json:"stopTimeoutSeconds"`
	RestartOnExit      bool              `json:"restartOnExit"`
	MaxRestarts        int               `json:"maxRestarts"`
	CreatedAt          time.Time         `json:"createdAt"`
	UpdatedAt          time.Time         `json:"updatedAt"`
}

// Store 持有数据库连接。
type Store struct {
	db *sql.DB
}

// Open 打开（或创建）数据库并完成结构迁移。
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.ToSlash(path) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	s := &Store{db: db}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close 关闭数据库连接。
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS tasks (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    name                 TEXT    NOT NULL UNIQUE,
    exe                  TEXT    NOT NULL,
    args                 TEXT    NOT NULL DEFAULT '[]',
    work_dir             TEXT    NOT NULL DEFAULT '',
    env                  TEXT    NOT NULL DEFAULT '{}',
    delay_seconds        INTEGER NOT NULL DEFAULT 0,
    sort_order           INTEGER NOT NULL DEFAULT 0,
    enabled              INTEGER NOT NULL DEFAULT 1,
    auto_start           INTEGER NOT NULL DEFAULT 1,
    depends_on           TEXT    NOT NULL DEFAULT '[]',
    health_port          INTEGER NOT NULL DEFAULT 0,
    stop_timeout_seconds INTEGER NOT NULL DEFAULT 15,
    restart_on_exit      INTEGER NOT NULL DEFAULT 0,
    max_restarts         INTEGER NOT NULL DEFAULT 3,
    created_at           TEXT    NOT NULL,
    updated_at           TEXT    NOT NULL
);
CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS audit_logs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at TEXT NOT NULL,
    username   TEXT NOT NULL,
    action     TEXT NOT NULL,
    target     TEXT NOT NULL DEFAULT '',
    ip         TEXT NOT NULL DEFAULT '',
    detail     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created ON audit_logs(created_at DESC);
`
	_, err := s.db.Exec(schema)
	return err
}

// ---------- 任务 ----------

// ListTasks 返回按排序权重升序排列的全部任务。
func (s *Store) ListTasks() ([]*Task, error) {
	rows, err := s.db.Query(`SELECT id, name, exe, args, work_dir, env, delay_seconds, sort_order,
		enabled, auto_start, depends_on, health_port, stop_timeout_seconds, restart_on_exit,
		max_restarts, created_at, updated_at FROM tasks ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

// GetTask 按 ID 查询任务。
func (s *Store) GetTask(id int64) (*Task, error) {
	row := s.db.QueryRow(`SELECT id, name, exe, args, work_dir, env, delay_seconds, sort_order,
		enabled, auto_start, depends_on, health_port, stop_timeout_seconds, restart_on_exit,
		max_restarts, created_at, updated_at FROM tasks WHERE id = ?`, id)
	t, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

// CreateTask 新增任务，返回自增 ID。
func (s *Store) CreateTask(t *Task) (int64, error) {
	normalize(t)
	now := time.Now()
	t.CreatedAt, t.UpdatedAt = now, now
	res, err := s.db.Exec(`INSERT INTO tasks (name, exe, args, work_dir, env, delay_seconds, sort_order,
		enabled, auto_start, depends_on, health_port, stop_timeout_seconds, restart_on_exit, max_restarts,
		created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.Name, t.Exe, encodeJSON(t.Args), t.WorkDir, encodeJSON(t.Env), t.DelaySeconds, t.SortOrder,
		boolToInt(t.Enabled), boolToInt(t.AutoStart), encodeJSON(t.DependsOn), t.HealthPort,
		t.StopTimeoutSeconds, boolToInt(t.RestartOnExit), t.MaxRestarts,
		formatTime(t.CreatedAt), formatTime(t.UpdatedAt))
	if err != nil {
		return 0, friendlyErr(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	t.ID = id
	return id, nil
}

// UpdateTask 更新任务配置（不包含运行状态）。
func (s *Store) UpdateTask(t *Task) error {
	normalize(t)
	t.UpdatedAt = time.Now()
	res, err := s.db.Exec(`UPDATE tasks SET name=?, exe=?, args=?, work_dir=?, env=?, delay_seconds=?,
		sort_order=?, enabled=?, auto_start=?, depends_on=?, health_port=?, stop_timeout_seconds=?,
		restart_on_exit=?, max_restarts=?, updated_at=? WHERE id=?`,
		t.Name, t.Exe, encodeJSON(t.Args), t.WorkDir, encodeJSON(t.Env), t.DelaySeconds,
		t.SortOrder, boolToInt(t.Enabled), boolToInt(t.AutoStart), encodeJSON(t.DependsOn),
		t.HealthPort, t.StopTimeoutSeconds, boolToInt(t.RestartOnExit), t.MaxRestarts,
		formatTime(t.UpdatedAt), t.ID)
	if err != nil {
		return friendlyErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteTask 删除任务。
func (s *Store) DeleteTask(id int64) error {
	res, err := s.db.Exec(`DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ReorderTasks 按给定 ID 顺序重写排序权重。
func (s *Store) ReorderTasks(ids []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := formatTime(time.Now())
	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE tasks SET sort_order=?, updated_at=? WHERE id=?`, i, now, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---------- 设置 ----------

// GetSetting 读取设置项。
func (s *Store) GetSetting(key string) (string, bool, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// SetSetting 写入设置项（存在则覆盖）。
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings(key, value) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// ---------- 审计日志 ----------

// AuditLog 记录一次关键操作。
type AuditLog struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	Username  string    `json:"username"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	IP        string    `json:"ip"`
	Detail    string    `json:"detail"`
}

// AddAudit 写入审计记录。
func (s *Store) AddAudit(username, action, target, ip, detail string) error {
	_, err := s.db.Exec(`INSERT INTO audit_logs(created_at, username, action, target, ip, detail)
		VALUES(?,?,?,?,?,?)`, formatTime(time.Now()), username, action, target, ip, detail)
	return err
}

// ListAudit 返回最近的审计记录。
func (s *Store) ListAudit(limit int) ([]*AuditLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT id, created_at, username, action, target, ip, detail
		FROM audit_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*AuditLog
	for rows.Next() {
		var a AuditLog
		var ts string
		if err := rows.Scan(&a.ID, &ts, &a.Username, &a.Action, &a.Target, &a.IP, &a.Detail); err != nil {
			return nil, err
		}
		a.CreatedAt = parseTime(ts)
		list = append(list, &a)
	}
	return list, rows.Err()
}

// ---------- 内部辅助 ----------

type scanner interface {
	Scan(dest ...any) error
}

func scanTask(sc scanner) (*Task, error) {
	var t Task
	var args, env, deps, created, updated string
	var enabled, autoStart, restart int
	err := sc.Scan(&t.ID, &t.Name, &t.Exe, &args, &t.WorkDir, &env, &t.DelaySeconds, &t.SortOrder,
		&enabled, &autoStart, &deps, &t.HealthPort, &t.StopTimeoutSeconds, &restart,
		&t.MaxRestarts, &created, &updated)
	if err != nil {
		return nil, err
	}
	decodeJSON(args, &t.Args)
	decodeJSON(env, &t.Env)
	decodeJSON(deps, &t.DependsOn)
	t.Enabled = enabled != 0
	t.AutoStart = autoStart != 0
	t.RestartOnExit = restart != 0
	t.CreatedAt = parseTime(created)
	t.UpdatedAt = parseTime(updated)
	return &t, nil
}

func normalize(t *Task) {
	t.Name = strings.TrimSpace(t.Name)
	t.Exe = strings.TrimSpace(t.Exe)
	t.WorkDir = strings.TrimSpace(t.WorkDir)
	if t.Args == nil {
		t.Args = []string{}
	}
	if t.Env == nil {
		t.Env = map[string]string{}
	}
	if t.DependsOn == nil {
		t.DependsOn = []int64{}
	}
	if t.StopTimeoutSeconds <= 0 {
		t.StopTimeoutSeconds = 15
	}
	if t.MaxRestarts < 0 {
		t.MaxRestarts = 0
	}
}

func encodeJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}

func decodeJSON(s string, v any) {
	if s == "" {
		return
	}
	_ = json.Unmarshal([]byte(s), v)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// friendlyErr 将 SQLite 约束错误转换为可读提示。
func friendlyErr(err error) error {
	if strings.Contains(err.Error(), "UNIQUE constraint failed: tasks.name") {
		return errors.New("任务名称已存在")
	}
	return err
}

func formatTime(t time.Time) string { return t.Format(time.RFC3339) }

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// Validate 检查任务必填项。
func (t *Task) Validate() error {
	if t.Name == "" {
		return errors.New("任务名称不能为空")
	}
	if t.Exe == "" {
		return errors.New("可执行文件路径不能为空")
	}
	if t.HealthPort < 0 || t.HealthPort > 65535 {
		return errors.New("健康检查端口不合法")
	}
	if t.DelaySeconds < 0 || t.DelaySeconds > 3600 {
		return fmt.Errorf("启动延迟需在 0-3600 秒之间")
	}
	return nil
}
