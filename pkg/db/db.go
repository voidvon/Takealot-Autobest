package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
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
		`CREATE TABLE IF NOT EXISTS cached_offers (
			target_key TEXT PRIMARY KEY,
			tsin_id TEXT,
			plid TEXT,
			title TEXT,
			selling_price INTEGER DEFAULT 0,
			rrp INTEGER DEFAULT 0,
			stock INTEGER DEFAULT 0,
			date_modified TEXT,
			best_price INTEGER DEFAULT 0,
			competing_offers INTEGER DEFAULT 0,
			priority_status TEXT DEFAULT 'solo',
			price_diff INTEGER DEFAULT 0,
			image_url TEXT,
			image_large_url TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_reprice_history_created ON reprice_history(created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_follow_history_created ON follow_history(created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_batch_jobs_created ON batch_jobs(created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_cached_offers_tsin ON cached_offers(tsin_id);`,
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

func (d *DB) CountTargets() (int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var count int
	err := d.conn.QueryRow(`SELECT count(*) FROM reprice_targets`).Scan(&count)
	return count, err
}

// MigrateGJData checks if legacy GJDATA file exists. If found, it parses and imports
// all target records into SQLite `reprice_targets` table, and renames the legacy file
// to .migrated.bak to preserve a safe backup while preventing re-migration.
func (d *DB) MigrateGJData(filePath string) (int, error) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return 0, nil
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return 0, fmt.Errorf("读取旧版 GJDATA 失败: %w", err)
	}

	content := strings.TrimSpace(string(data))
	targets := make(map[string]config.Target)
	if len(content) > 0 {
		items := strings.Split(content, "#")
		for _, item := range items {
			parts := strings.Split(strings.TrimSpace(item), "/")
			if len(parts) >= 5 {
				key := fmt.Sprintf("%s/%s", parts[0], parts[1])
				minP, _ := strconv.Atoi(parts[3])
				maxP, _ := strconv.Atoi(parts[4])
				targets[key] = config.Target{
					Selected: parts[2] == "1",
					MinPrice: minP,
					MaxPrice: maxP,
				}
			}
		}
	}

	if len(targets) > 0 {
		if err := d.SaveTargets(targets); err != nil {
			return 0, fmt.Errorf("将迁移数据写入 SQLite reprice_targets 失败: %w", err)
		}
	}

	// Rename legacy file to .migrated.bak for backup
	bakFile := filePath + ".migrated.bak"
	if err := os.Rename(filePath, bakFile); err != nil {
		_ = os.Remove(bakFile)
		_ = os.Rename(filePath, bakFile)
	}

	return len(targets), nil
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

// --- Cached Offers for 1000+ Items (Local SQLite Cache) ---

type CachedOffer struct {
	Key             string `json:"key"`
	TSINID          string `json:"tsin_id"`
	PLID            string `json:"plid"`
	Title           string `json:"title"`
	SellingPrice    int    `json:"selling_price"`
	RRP             int    `json:"rrp"`
	Stock           int    `json:"stock"`
	DateModified    string `json:"date_modified"`
	BestPrice       int    `json:"best_price"`
	CompetingOffers int    `json:"competing_offers"`
	PriorityStatus  string `json:"priority_status"`
	PriceDiff       int    `json:"price_diff"`
	ImageURL        string `json:"image_url"`
	ImageLargeURL   string `json:"image_large_url"`
	UpdatedAt       string `json:"updated_at"`
}

// SaveCachedOffers 批量保存/更新商品基础信息。如果已有竞品价，自动保留原竞品价，不被 0 冲掉。
func (d *DB) SaveCachedOffers(offers []CachedOffer) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO cached_offers (
			target_key, tsin_id, plid, title, selling_price, rrp, stock, date_modified,
			best_price, competing_offers, priority_status, price_diff, image_url, image_large_url, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(target_key) DO UPDATE SET
			title = excluded.title,
			selling_price = excluded.selling_price,
			rrp = excluded.rrp,
			stock = excluded.stock,
			date_modified = excluded.date_modified,
			best_price = CASE WHEN excluded.best_price > 0 THEN excluded.best_price ELSE cached_offers.best_price END,
			competing_offers = CASE WHEN excluded.competing_offers > 0 THEN excluded.competing_offers ELSE cached_offers.competing_offers END,
			priority_status = CASE WHEN excluded.best_price > 0 THEN excluded.priority_status ELSE cached_offers.priority_status END,
			price_diff = CASE WHEN excluded.best_price > 0 THEN excluded.price_diff ELSE cached_offers.price_diff END,
			image_url = excluded.image_url,
			image_large_url = excluded.image_large_url,
			updated_at = CURRENT_TIMESTAMP
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, item := range offers {
		if _, err := stmt.Exec(
			item.Key, item.TSINID, item.PLID, item.Title, item.SellingPrice, item.RRP, item.Stock, item.DateModified,
			item.BestPrice, item.CompetingOffers, item.PriorityStatus, item.PriceDiff, item.ImageURL, item.ImageLargeURL,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// UpdateSingleMPV 更新单个商品的竞品最低价与购物车竞争状态
func (d *DB) UpdateSingleMPV(tsinID string, bestPrice, competing int, priorityStatus string, priceDiff int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.conn.Exec(`
		UPDATE cached_offers
		SET best_price = ?, competing_offers = ?, priority_status = ?, price_diff = ?, updated_at = CURRENT_TIMESTAMP
		WHERE tsin_id = ?
	`, bestPrice, competing, priorityStatus, priceDiff, tsinID)
	return err
}

// LoadCachedOffers 从本地 SQLite 加载全部缓存的商品列表（毫秒级返回）
func (d *DB) LoadCachedOffers() ([]CachedOffer, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rows, err := d.conn.Query(`
		SELECT target_key, tsin_id, plid, COALESCE(title, ''), selling_price, rrp, stock,
		       COALESCE(date_modified, ''), best_price, competing_offers, COALESCE(priority_status, 'solo'),
		       price_diff, COALESCE(image_url, ''), COALESCE(image_large_url, ''),
		       datetime(updated_at, 'localtime')
		FROM cached_offers
		ORDER BY stock DESC, selling_price DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]CachedOffer, 0)
	for rows.Next() {
		var item CachedOffer
		if err := rows.Scan(
			&item.Key, &item.TSINID, &item.PLID, &item.Title, &item.SellingPrice, &item.RRP, &item.Stock,
			&item.DateModified, &item.BestPrice, &item.CompetingOffers, &item.PriorityStatus,
			&item.PriceDiff, &item.ImageURL, &item.ImageLargeURL, &item.UpdatedAt,
		); err != nil {
			continue
		}
		list = append(list, item)
	}
	return list, nil
}

// GetCachedOffersCount 获取当前缓存的商品总数
func (d *DB) GetCachedOffersCount() (int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var count int
	err := d.conn.QueryRow(`SELECT COUNT(*) FROM cached_offers`).Scan(&count)
	return count, err
}


