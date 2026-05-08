package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// DB 包装 *sql.DB，提供通话记录操作
type DB struct {
	db *sql.DB
}

// Call 对应数据库中的一条通话记录
type Call struct {
	ID            int64      `json:"id"`
	CallID        string     `json:"callId"`
	FromNumber    string     `json:"fromNumber"`
	ToNumber      string     `json:"toNumber"`
	StartTime     time.Time  `json:"startTime"`
	EndTime       *time.Time `json:"endTime,omitempty"`
	DurationSecs  *int       `json:"durationSecs,omitempty"`
	RecordingPath *string    `json:"recordingPath,omitempty"`
	Status        string     `json:"status"`
}

// Open 打开/创建 SQLite 数据库并初始化表结构
func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &DB{db: db}, nil
}

// Close 关闭数据库连接
func (d *DB) Close() error {
	return d.db.Close()
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS calls (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			call_id        TEXT    NOT NULL UNIQUE,
			from_number    TEXT    NOT NULL,
			to_number      TEXT    NOT NULL,
			start_time     DATETIME NOT NULL,
			end_time       DATETIME,
			duration_secs  INTEGER,
			recording_path TEXT,
			status         TEXT    NOT NULL DEFAULT 'active'
		);
		CREATE INDEX IF NOT EXISTS idx_calls_status ON calls(status);
		CREATE INDEX IF NOT EXISTS idx_calls_start_time ON calls(start_time DESC);
	`)
	return err
}

// CreateCall 插入一条新通话记录
func (d *DB) CreateCall(c *Call) (int64, error) {
	result, err := d.db.Exec(
		`INSERT OR IGNORE INTO calls (call_id, from_number, to_number, start_time, status)
		 VALUES (?, ?, ?, ?, ?)`,
		c.CallID, c.FromNumber, c.ToNumber, c.StartTime.UTC(), c.Status,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdateCallEnd 更新通话结束信息
func (d *DB) UpdateCallEnd(callID string, endTime time.Time, durationSecs int, recordingPath, status string) error {
	_, err := d.db.Exec(
		`UPDATE calls SET end_time=?, duration_secs=?, recording_path=?, status=? WHERE call_id=?`,
		endTime.UTC(), durationSecs, nullableString(recordingPath), status, callID,
	)
	return err
}

// ListCalls 分页查询通话记录
func (d *DB) ListCalls(status string, page, limit int) ([]*Call, int, error) {
	offset := (page - 1) * limit

	var total int
	countQuery := `SELECT COUNT(*) FROM calls`
	args := []interface{}{}
	if status != "" {
		countQuery += ` WHERE status=?`
		args = append(args, status)
	}
	if err := d.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT id, call_id, from_number, to_number, start_time, end_time,
	           duration_secs, recording_path, status FROM calls`
	if status != "" {
		query += ` WHERE status=?`
	}
	query += ` ORDER BY start_time DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var calls []*Call
	for rows.Next() {
		c := &Call{}
		var endTime sql.NullTime
		var duration sql.NullInt64
		var recPath sql.NullString
		err := rows.Scan(&c.ID, &c.CallID, &c.FromNumber, &c.ToNumber,
			&c.StartTime, &endTime, &duration, &recPath, &c.Status)
		if err != nil {
			return nil, 0, err
		}
		if endTime.Valid {
			t := endTime.Time
			c.EndTime = &t
		}
		if duration.Valid {
			d := int(duration.Int64)
			c.DurationSecs = &d
		}
		if recPath.Valid {
			c.RecordingPath = &recPath.String
		}
		calls = append(calls, c)
	}
	return calls, total, rows.Err()
}

// GetCallByID 按数据库 ID 查询
func (d *DB) GetCallByID(id int64) (*Call, error) {
	c := &Call{}
	var endTime sql.NullTime
	var duration sql.NullInt64
	var recPath sql.NullString
	err := d.db.QueryRow(
		`SELECT id, call_id, from_number, to_number, start_time, end_time,
		  duration_secs, recording_path, status FROM calls WHERE id=?`, id,
	).Scan(&c.ID, &c.CallID, &c.FromNumber, &c.ToNumber,
		&c.StartTime, &endTime, &duration, &recPath, &c.Status)
	if err != nil {
		return nil, err
	}
	if endTime.Valid {
		t := endTime.Time
		c.EndTime = &t
	}
	if duration.Valid {
		dv := int(duration.Int64)
		c.DurationSecs = &dv
	}
	if recPath.Valid {
		c.RecordingPath = &recPath.String
	}
	return c, nil
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
