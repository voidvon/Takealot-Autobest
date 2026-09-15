package db

import (
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	_ "modernc.org/sqlite"
	"takealot/pkg/config"
)

type RepriceHistoryRecord struct {
	ID              int64  `json:"id"`
	TargetKey       string `json:"target_key"`
	TSINID          string `json:"tsin_id"`
	Title           string `json:"title"`
	OldPrice        int    `json:"old_price"`
	NewPrice        int    `json:"new_price"`
	CompetitorPrice int    `json:"competitor_price"`
	Reason          string `json:"reason"`
	CreatedAt       string `json:"created_at"`
}

type FollowHistoryRecord struct {
	ID        int64  `json:"id"`
	SourceURL string `json:"source_url"`
	TSINID    string `json:"tsin_id"`
	PLID      string `json:"plid"`
	Title     string `json:"title"`
	Stock     int    `json:"stock"`
	MinPrice  int    `json:"min_price"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
}

type BatchJobRecord struct {
	ID            int64  `json:"id"`
	BatchID       string `json:"batch_id"`
	ActionType    string `json:"action_type"`
	ItemCount     int    `json:"item_count"`
	Status        string `json:"status"`
	ResultSummary string `json:"result_summary"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type DB struct {
	mu   sync.RWMutex
	conn *sql.DB
}

func New(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite 失败: %w", err)
	}

	// Set connection pool parameters
	conn.SetMaxOpenConns(1) // SQLite single writer concurrency
	conn.SetMaxIdleConns(1)
	conn.SetConnMaxLifetime(time.Hour)

	d := &DB{conn: conn}
	if err := d.migrate(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("数据库迁移失败: %w", err)
	}

	log.Printf("📦 SQLite 数据库已就绪: %s", dbPath)
	return d, nil
}

func (d *DB) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conn != nil {
		return d.conn.Close()
	}
	return nil
}

func (d *DB) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS system_config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS reprice_targets (
			target_key TEXT PRIMARY KEY,
			selected INTEGER DEFAULT 0,
			min_price INTEGER DEFAULT 0,
			max_price INTEGER DEFAULT 0,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS reprice_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			target_key TEXT NOT NULL,
			tsin_id TEXT,
			title TEXT,
			old_price INTEGER,
			new_price INTEGER,
			competitor_price INTEGER,
			reason TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS follow_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_url TEXT,
			tsin_id TEXT,
			plid TEXT,
			title TEXT,
			stock INTEGER,
			min_price INTEGER,
			status TEXT,
			message TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS batch_jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id TEXT UNIQUE NOT NULL,
			action_type TEXT,
			item_count INTEGER DEFAULT 0,
			status TEXT,
			result_summary TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_reprice_history_created ON reprice_history(created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_follow_history_created ON follow_history(created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_batch_jobs_created ON batch_jobs(created_at DESC);`,
	}

	for _, q := range queries {
		if _, err := d.conn.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// --- Target Configs in SQLite ---

func (d *DB) SaveTargets(targets map[string]config.Target) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO reprice_targets (target_key, selected, min_price, max_price, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(target_key) DO UPDATE SET
			selected = excluded.selected,
			min_price = excluded.min_price,
			max_price = excluded.max_price,
			updated_at = CURRENT_TIMESTAMP
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for key, t := range targets {
		sel := 0
		if t.Selected {
			sel = 1
		}
		if _, err := stmt.Exec(key, sel, t.MinPrice, t.MaxPrice); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (d *DB) LoadTargets() (map[string]config.Target, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rows, err := d.conn.Query(`SELECT target_key, selected, min_price, max_price FROM reprice_targets`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := make(map[string]config.Target)
	for rows.Next() {
		var key string
		var selInt, minP, maxP int
		if err := rows.Scan(&key, &selInt, &minP, &maxP); err != nil {
			continue
		}
		res[key] = config.Target{
			Selected: selInt == 1,
			MinPrice: minP,
			MaxPrice: maxP,
		}
	}
	return res, nil
}

// --- Reprice History in SQLite ---

func (d *DB) RecordReprice(targetKey, tsinID, title string, oldPrice, newPrice, competitorPrice int, reason string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.conn.Exec(`
		INSERT INTO reprice_history (target_key, tsin_id, title, old_price, new_price, competitor_price, reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, targetKey, tsinID, title, oldPrice, newPrice, competitorPrice, reason)
	return err
}

func (d *DB) GetRecentRepriceHistory(limit int) ([]RepriceHistoryRecord, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}
	rows, err := d.conn.Query(`
		SELECT id, target_key, COALESCE(tsin_id, ''), COALESCE(title, ''), old_price, new_price, competitor_price, COALESCE(reason, ''), datetime(created_at, 'localtime')
		FROM reprice_history
		ORDER BY id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]RepriceHistoryRecord, 0)
	for rows.Next() {
		var item RepriceHistoryRecord
		if err := rows.Scan(&item.ID, &item.TargetKey, &item.TSINID, &item.Title, &item.OldPrice, &item.NewPrice, &item.CompetitorPrice, &item.Reason, &item.CreatedAt); err != nil {
			continue
		}
		list = append(list, item)
	}
	return list, nil
}

// --- Follow History in SQLite ---

func (d *DB) RecordFollow(sourceURL, tsinID, plid, title string, stock, minPrice int, status, message string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.conn.Exec(`
		INSERT INTO follow_history (source_url, tsin_id, plid, title, stock, min_price, status, message, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, sourceURL, tsinID, plid, title, stock, minPrice, status, message)
	return err
}

func (d *DB) GetRecentFollowHistory(limit int) ([]FollowHistoryRecord, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}
	rows, err := d.conn.Query(`
		SELECT id, COALESCE(source_url, ''), COALESCE(tsin_id, ''), COALESCE(plid, ''), COALESCE(title, ''), stock, min_price, COALESCE(status, ''), COALESCE(message, ''), datetime(created_at, 'localtime')
		FROM follow_history
		ORDER BY id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]FollowHistoryRecord, 0)
	for rows.Next() {
		var item FollowHistoryRecord
		if err := rows.Scan(&item.ID, &item.SourceURL, &item.TSINID, &item.PLID, &item.Title, &item.Stock, &item.MinPrice, &item.Status, &item.Message, &item.CreatedAt); err != nil {
			continue
		}
		list = append(list, item)
	}
	return list, nil
}

// --- Official Batch Jobs in SQLite ---

func (d *DB) RecordBatchJob(batchID, actionType string, itemCount int, status string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.conn.Exec(`
		INSERT INTO batch_jobs (batch_id, action_type, item_count, status, result_summary, created_at, updated_at)
		VALUES (?, ?, ?, ?, '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(batch_id) DO UPDATE SET
			status = excluded.status,
			updated_at = CURRENT_TIMESTAMP
	`, batchID, actionType, itemCount, status)
	return err
}

func (d *DB) UpdateBatchJob(batchID, status, resultSummary string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.conn.Exec(`
		UPDATE batch_jobs
		SET status = ?, result_summary = ?, updated_at = CURRENT_TIMESTAMP
		WHERE batch_id = ?
	`, status, resultSummary, batchID)
	return err
}

func (d *DB) GetRecentBatchJobs(limit int) ([]BatchJobRecord, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}
	rows, err := d.conn.Query(`
		SELECT id, batch_id, COALESCE(action_type, ''), item_count, COALESCE(status, ''), COALESCE(result_summary, ''), datetime(created_at, 'localtime'), datetime(updated_at, 'localtime')
		FROM batch_jobs
		ORDER BY id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]BatchJobRecord, 0)
	for rows.Next() {
		var item BatchJobRecord
		if err := rows.Scan(&item.ID, &item.BatchID, &item.ActionType, &item.ItemCount, &item.Status, &item.ResultSummary, &item.CreatedAt, &item.UpdatedAt); err != nil {
			continue
		}
		list = append(list, item)
	}
	return list, nil
}

