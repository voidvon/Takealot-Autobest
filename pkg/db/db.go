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

type Store struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Authorization     string  `json:"authorization"`
	IsActive          bool    `json:"is_active"`
	PriceDecreaseStep int     `json:"price_decrease_step"`
	PriceIncreaseStep int     `json:"price_increase_step"`
	RRPPercentage     float64 `json:"rrp_percentage"`
	IntervalMinutes   int     `json:"interval_minutes"`
	BulkStock         int     `json:"bulk_stock"`
	MaxFetchOffers    int     `json:"max_fetch_offers"`
	ProxyURL          string  `json:"proxy_url"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
}

type RepriceHistoryRecord struct {
	ID              int64  `json:"id"`
	StoreID         string `json:"store_id"`
	TargetKey       string `json:"target_key"`
	TSINID          string `json:"tsin_id"`
	SKU             string `json:"sku"`
	ImageURL        string `json:"image_url"`
	Title           string `json:"title"`
	StoreName       string `json:"store_name"`
	Action          string `json:"action"`
	OldPrice        int    `json:"old_price"`
	NewPrice        int    `json:"new_price"`
	CompetitorPrice int    `json:"competitor_price"`
	Reason          string `json:"reason"`
	CreatedAt       string `json:"created_at"`
}

type FollowHistoryRecord struct {
	ID        int64  `json:"id"`
	StoreID   string `json:"store_id"`
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
	StoreID       string `json:"store_id"`
	BatchID       string `json:"batch_id"`
	ActionType    string `json:"action_type"`
	ItemCount     int    `json:"item_count"`
	Status        string `json:"status"`
	ResultSummary string `json:"result_summary"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type ShipmentRecord struct {
	ID             int64                `json:"id"`
	StoreID        string               `json:"store_id"`
	ShipmentNumber string               `json:"shipment_number"`
	Status         string               `json:"status"`         // "draft", "confirmed", "shipped", "delivered", "cancelled"
	DestinationDC  string               `json:"destination_dc"` // "JHB", "CPT", "DUR", "ALL"
	TotalItems     int                  `json:"total_items"`
	TotalUnits     int                  `json:"total_units"`
	TotalValue     float64              `json:"total_value"`
	Notes          string               `json:"notes"`
	Items          []ShipmentItemRecord `json:"items,omitempty"`
	CreatedAt      string               `json:"created_at"`
	UpdatedAt      string               `json:"updated_at"`
}

type ShipmentItemRecord struct {
	ID               int64   `json:"id"`
	ShipmentID       int64   `json:"shipment_id"`
	OrderID          int64   `json:"order_id"`
	OrderItemID      int64   `json:"order_item_id"`
	OrderDate        string  `json:"order_date"`
	DueDate          string  `json:"due_date"`
	TSIN             string  `json:"tsin"`
	SKU              string  `json:"sku"`
	Title            string  `json:"title"`
	ImageURL         string  `json:"image_url"`
	SellingPrice     float64 `json:"selling_price"`
	DC               string  `json:"dc"`
	LeadtimeStock    int     `json:"leadtime_stock"`
	DemandQty        int     `json:"demand_qty"`
	ShipQty          int     `json:"ship_qty"`
	ActualWeight     float64 `json:"actual_weight"`
	VolumetricWeight float64 `json:"volumetric_weight"`
	WeighStatus      string  `json:"weigh_status"` // "pending", "done"
	CreatedAt        string  `json:"created_at"`
}

type BookingRecord struct {
	ID            int64  `json:"id"`
	StoreID       string `json:"store_id"`
	BookingNumber string `json:"booking_number"`
	ShipmentID    int64  `json:"shipment_id,omitempty"`
	ShipmentNo    string `json:"shipment_no,omitempty"`
	DestinationDC string `json:"destination_dc"`
	BookingDate   string `json:"booking_date"`
	TimeSlot      string `json:"time_slot"`
	CarrierName   string `json:"carrier_name"`
	VehicleReg    string `json:"vehicle_reg"`
	Status        string `json:"status"` // "scheduled", "completed", "cancelled"
	Notes         string `json:"notes"`
	CreatedAt     string `json:"created_at"`
}

type CachedOffer struct {
	StoreID         string `json:"store_id"`
	Key             string `json:"key"`
	TSINID          string `json:"tsin_id"`
	SKU             string `json:"sku"`
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

type DB struct {
	mu   sync.RWMutex
	conn *sql.DB
}

func columnExists(conn *sql.DB, tableName, columnName string) bool {
	rows, err := conn.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return false
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err == nil {
			if strings.EqualFold(name, columnName) {
				return true
			}
		}
	}
	return false
}

func tableExists(conn *sql.DB, tableName string) bool {
	var name string
	err := conn.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", tableName).Scan(&name)
	return err == nil && name != ""
}

func New(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite 失败: %w", err)
	}

	// 启用 WAL 模式和 busy_timeout，提升多 Goroutine 并发读写性能，彻底规避 database is locked
	_, _ = conn.Exec("PRAGMA journal_mode=WAL;")
	_, _ = conn.Exec("PRAGMA busy_timeout=5000;")

	// Set connection pool parameters
	conn.SetMaxOpenConns(1) // SQLite single writer concurrency
	conn.SetMaxIdleConns(1)
	conn.SetConnMaxLifetime(time.Hour)

	d := &DB{conn: conn}
	if err := d.migrate(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("数据库迁移失败: %w", err)
	}

	log.Printf("📦 SQLite 数据库已就绪: %s (WAL模式已开启)", dbPath)
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
	// Persistent ownership prevents our stores from repricing against each other.
	if _, err := d.conn.Exec(`CREATE TABLE IF NOT EXISTS reprice_owners (
		product_key TEXT PRIMARY KEY,
		store_id TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return err
	}
	// 1. 初始化 stores 表
	storeTableQuery := `CREATE TABLE IF NOT EXISTS stores (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		authorization TEXT NOT NULL,
		is_active INTEGER DEFAULT 1,
		price_decrease_step INTEGER DEFAULT 1,
		price_increase_step INTEGER DEFAULT 1,
		rrp_percentage REAL DEFAULT 120.0,
		interval_minutes INTEGER DEFAULT 5,
		bulk_stock INTEGER DEFAULT 1,
		max_fetch_offers INTEGER DEFAULT 1000,
		proxy_url TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := d.conn.Exec(storeTableQuery); err != nil {
		return err
	}

	// 2. 检查 reprice_targets 是否需要迁移为 (store_id, target_key) 联合主键
	if tableExists(d.conn, "reprice_targets") {
		if !columnExists(d.conn, "reprice_targets", "store_id") {
			migrationSQL := `
				CREATE TABLE reprice_targets_new (
					store_id TEXT NOT NULL DEFAULT 'default',
					target_key TEXT NOT NULL,
					selected INTEGER DEFAULT 0,
					min_price INTEGER DEFAULT 0,
					max_price INTEGER DEFAULT 0,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (store_id, target_key)
				);
				INSERT OR IGNORE INTO reprice_targets_new (store_id, target_key, selected, min_price, max_price, updated_at)
				SELECT 'default', target_key, selected, min_price, max_price, updated_at FROM reprice_targets;
				DROP TABLE reprice_targets;
				ALTER TABLE reprice_targets_new RENAME TO reprice_targets;
			`
			if _, err := d.conn.Exec(migrationSQL); err != nil {
				return fmt.Errorf("迁移 reprice_targets 失败: %w", err)
			}
		}
	} else {
		if _, err := d.conn.Exec(`CREATE TABLE IF NOT EXISTS reprice_targets (
			store_id TEXT NOT NULL DEFAULT 'default',
			target_key TEXT NOT NULL,
			selected INTEGER DEFAULT 0,
			min_price INTEGER DEFAULT 0,
			max_price INTEGER DEFAULT 0,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (store_id, target_key)
		);`); err != nil {
			return err
		}
	}
	_, _ = d.conn.Exec(`CREATE INDEX IF NOT EXISTS idx_reprice_targets_store ON reprice_targets(store_id);`)

	// 3. 检查 cached_offers 是否需要迁移为 (store_id, target_key) 联合主键
	if tableExists(d.conn, "cached_offers") {
		if !columnExists(d.conn, "cached_offers", "store_id") {
			migrationSQL := `
				CREATE TABLE cached_offers_new (
					store_id TEXT NOT NULL DEFAULT 'default',
					target_key TEXT NOT NULL,
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
					sku TEXT,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (store_id, target_key)
				);
				INSERT OR IGNORE INTO cached_offers_new (store_id, target_key, tsin_id, plid, title, selling_price, rrp, stock, date_modified, best_price, competing_offers, priority_status, price_diff, image_url, image_large_url, sku, updated_at)
				SELECT 'default', target_key, tsin_id, plid, title, selling_price, rrp, stock, date_modified, best_price, competing_offers, priority_status, price_diff, image_url, image_large_url, sku, updated_at FROM cached_offers;
				DROP TABLE cached_offers;
				ALTER TABLE cached_offers_new RENAME TO cached_offers;
			`
			if _, err := d.conn.Exec(migrationSQL); err != nil {
				return fmt.Errorf("迁移 cached_offers 失败: %w", err)
			}
		}
	} else {
		if _, err := d.conn.Exec(`CREATE TABLE IF NOT EXISTS cached_offers (
			store_id TEXT NOT NULL DEFAULT 'default',
			target_key TEXT NOT NULL,
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
			sku TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (store_id, target_key)
		);`); err != nil {
			return err
		}
	}
	_, _ = d.conn.Exec(`CREATE INDEX IF NOT EXISTS idx_cached_offers_store_tsin ON cached_offers(store_id, tsin_id);`)

	// 4. 初始化基础表
	baseTables := []string{
		`CREATE TABLE IF NOT EXISTS system_config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS reprice_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			store_id TEXT NOT NULL DEFAULT 'default',
			target_key TEXT NOT NULL,
			tsin_id TEXT,
			sku TEXT,
			image_url TEXT,
			title TEXT,
			store_name TEXT,
			action TEXT,
			old_price INTEGER,
			new_price INTEGER,
			competitor_price INTEGER,
			reason TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS follow_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			store_id TEXT NOT NULL DEFAULT 'default',
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
			store_id TEXT NOT NULL DEFAULT 'default',
			batch_id TEXT UNIQUE NOT NULL,
			action_type TEXT,
			item_count INTEGER DEFAULT 0,
			status TEXT,
			result_summary TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS shipments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			store_id TEXT NOT NULL DEFAULT 'default',
			shipment_number TEXT UNIQUE NOT NULL,
			status TEXT NOT NULL DEFAULT 'draft',
			destination_dc TEXT NOT NULL DEFAULT 'JHB',
			total_items INTEGER DEFAULT 0,
			total_units INTEGER DEFAULT 0,
			total_value REAL DEFAULT 0,
			notes TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS shipment_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			shipment_id INTEGER NOT NULL,
			order_id INTEGER DEFAULT 0,
			order_item_id INTEGER DEFAULT 0,
			order_date TEXT DEFAULT '',
			due_date TEXT DEFAULT '',
			tsin TEXT DEFAULT '',
			sku TEXT DEFAULT '',
			title TEXT DEFAULT '',
			image_url TEXT DEFAULT '',
			selling_price REAL DEFAULT 0,
			dc TEXT DEFAULT '',
			leadtime_stock INTEGER DEFAULT 0,
			demand_qty INTEGER DEFAULT 1,
			ship_qty INTEGER DEFAULT 1,
			actual_weight REAL DEFAULT 0,
			volumetric_weight REAL DEFAULT 0,
			weigh_status TEXT DEFAULT 'pending',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS dc_bookings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			store_id TEXT NOT NULL DEFAULT 'default',
			booking_number TEXT UNIQUE NOT NULL,
			shipment_id INTEGER DEFAULT 0,
			destination_dc TEXT NOT NULL,
			booking_date TEXT NOT NULL,
			time_slot TEXT DEFAULT '',
			carrier_name TEXT DEFAULT '',
			vehicle_reg TEXT DEFAULT '',
			status TEXT NOT NULL DEFAULT 'scheduled',
			notes TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
	}

	for _, q := range baseTables {
		if _, err := d.conn.Exec(q); err != nil {
			return err
		}
	}

	// 5. 增量字段添加（兼容老版本升级）
	columnMigrations := []string{
		`ALTER TABLE reprice_history ADD COLUMN store_id TEXT DEFAULT 'default';`,
		`ALTER TABLE reprice_history ADD COLUMN sku TEXT;`,
		`ALTER TABLE reprice_history ADD COLUMN image_url TEXT;`,
		`ALTER TABLE reprice_history ADD COLUMN store_name TEXT;`,
		`ALTER TABLE reprice_history ADD COLUMN action TEXT;`,
		`ALTER TABLE follow_history ADD COLUMN store_id TEXT DEFAULT 'default';`,
		`ALTER TABLE batch_jobs ADD COLUMN store_id TEXT DEFAULT 'default';`,
		`ALTER TABLE shipments ADD COLUMN store_id TEXT DEFAULT 'default';`,
		`ALTER TABLE dc_bookings ADD COLUMN store_id TEXT DEFAULT 'default';`,
		`ALTER TABLE cached_offers ADD COLUMN sku TEXT;`,
		`UPDATE shipment_items SET image_url = '' WHERE image_url LIKE '%covers_tsins%';`,
	}
	for _, m := range columnMigrations {
		_, _ = d.conn.Exec(m)
	}

	// 6. 索引优化
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_reprice_history_store ON reprice_history(store_id);`,
		`CREATE INDEX IF NOT EXISTS idx_reprice_history_created ON reprice_history(created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_follow_history_store ON follow_history(store_id);`,
		`CREATE INDEX IF NOT EXISTS idx_follow_history_created ON follow_history(created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_batch_jobs_store ON batch_jobs(store_id);`,
		`CREATE INDEX IF NOT EXISTS idx_batch_jobs_created ON batch_jobs(created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_shipments_store ON shipments(store_id);`,
		`CREATE INDEX IF NOT EXISTS idx_shipments_status ON shipments(status);`,
		`CREATE INDEX IF NOT EXISTS idx_shipment_items_shipment_id ON shipment_items(shipment_id);`,
		`CREATE INDEX IF NOT EXISTS idx_dc_bookings_store ON dc_bookings(store_id);`,
		`CREATE INDEX IF NOT EXISTS idx_dc_bookings_date ON dc_bookings(booking_date);`,
	}
	for _, idx := range indexes {
		_, _ = d.conn.Exec(idx)
	}

	return nil
}

// ----------------------------------------------------
// 店铺管理 (Stores CRUD)
// ----------------------------------------------------

func (d *DB) GetStores() ([]Store, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rows, err := d.conn.Query(`
		SELECT id, name, authorization, is_active, price_decrease_step, price_increase_step,
		       rrp_percentage, interval_minutes, bulk_stock, max_fetch_offers, COALESCE(proxy_url, ''),
		       datetime(created_at, 'localtime'), datetime(updated_at, 'localtime')
		FROM stores ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]Store, 0)
	for rows.Next() {
		var s Store
		var activeInt int
		if err := rows.Scan(&s.ID, &s.Name, &s.Authorization, &activeInt, &s.PriceDecreaseStep, &s.PriceIncreaseStep,
			&s.RRPPercentage, &s.IntervalMinutes, &s.BulkStock, &s.MaxFetchOffers, &s.ProxyURL, &s.CreatedAt, &s.UpdatedAt); err != nil {
			continue
		}
		s.IsActive = activeInt == 1
		list = append(list, s)
	}
	return list, nil
}

func (d *DB) GetStore(id string) (*Store, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if id == "" {
		id = "default"
	}

	row := d.conn.QueryRow(`
		SELECT id, name, authorization, is_active, price_decrease_step, price_increase_step,
		       rrp_percentage, interval_minutes, bulk_stock, max_fetch_offers, COALESCE(proxy_url, ''),
		       datetime(created_at, 'localtime'), datetime(updated_at, 'localtime')
		FROM stores WHERE id = ?
	`, id)

	var s Store
	var activeInt int
	if err := row.Scan(&s.ID, &s.Name, &s.Authorization, &activeInt, &s.PriceDecreaseStep, &s.PriceIncreaseStep,
		&s.RRPPercentage, &s.IntervalMinutes, &s.BulkStock, &s.MaxFetchOffers, &s.ProxyURL, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return nil, err
	}
	s.IsActive = activeInt == 1
	return &s, nil
}

func (d *DB) SaveStore(s Store) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if s.ID == "" {
		s.ID = fmt.Sprintf("store_%d", time.Now().Unix())
	}
	if s.Name == "" {
		s.Name = "未命名店铺"
	}
	if s.PriceDecreaseStep <= 0 {
		s.PriceDecreaseStep = 1
	}
	if s.PriceIncreaseStep <= 0 {
		s.PriceIncreaseStep = 1
	}
	if s.IntervalMinutes <= 0 {
		s.IntervalMinutes = 5
	}
	if s.BulkStock <= 0 {
		s.BulkStock = 1
	}
	if s.MaxFetchOffers <= 0 {
		s.MaxFetchOffers = 1000
	}
	if s.RRPPercentage < 100 {
		s.RRPPercentage = 120.0
	}

	activeInt := 0
	if s.IsActive {
		activeInt = 1
	}

	_, err := d.conn.Exec(`
		INSERT INTO stores (
			id, name, authorization, is_active, price_decrease_step, price_increase_step,
			rrp_percentage, interval_minutes, bulk_stock, max_fetch_offers, proxy_url, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now', 'localtime'), datetime('now', 'localtime'))
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			authorization = excluded.authorization,
			is_active = excluded.is_active,
			price_decrease_step = excluded.price_decrease_step,
			price_increase_step = excluded.price_increase_step,
			rrp_percentage = excluded.rrp_percentage,
			interval_minutes = excluded.interval_minutes,
			bulk_stock = excluded.bulk_stock,
			max_fetch_offers = excluded.max_fetch_offers,
			proxy_url = excluded.proxy_url,
			updated_at = datetime('now', 'localtime')
	`, s.ID, s.Name, s.Authorization, activeInt, s.PriceDecreaseStep, s.PriceIncreaseStep,
		s.RRPPercentage, s.IntervalMinutes, s.BulkStock, s.MaxFetchOffers, s.ProxyURL)
	return err
}

func (d *DB) DeleteStore(id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	var count int
	_ = d.conn.QueryRow(`SELECT COUNT(*) FROM stores`).Scan(&count)
	if count <= 1 {
		return fmt.Errorf("系统中至少保留一个店铺，无法删除最后一个店铺")
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM reprice_owners WHERE store_id = ?`, id); err != nil {
		return err
	}
	_, _ = tx.Exec(`DELETE FROM reprice_targets WHERE store_id = ?`, id)
	_, _ = tx.Exec(`DELETE FROM cached_offers WHERE store_id = ?`, id)
	_, _ = tx.Exec(`DELETE FROM reprice_history WHERE store_id = ?`, id)
	_, _ = tx.Exec(`DELETE FROM follow_history WHERE store_id = ?`, id)
	_, _ = tx.Exec(`DELETE FROM batch_jobs WHERE store_id = ?`, id)
	_, _ = tx.Exec(`DELETE FROM shipments WHERE store_id = ?`, id)
	_, _ = tx.Exec(`DELETE FROM dc_bookings WHERE store_id = ?`, id)
	_, err = tx.Exec(`DELETE FROM stores WHERE id = ?`, id)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (d *DB) EnsureDefaultStore(defaultAuth string, defaultDecrease, defaultIncrease, defaultInterval, defaultBulkStock, defaultMaxFetch int, defaultRRP float64) (*Store, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var count int
	err := d.conn.QueryRow(`SELECT COUNT(*) FROM stores`).Scan(&count)
	if err == nil && count > 0 {
		row := d.conn.QueryRow(`
			SELECT id, name, authorization, is_active, price_decrease_step, price_increase_step,
			       rrp_percentage, interval_minutes, bulk_stock, max_fetch_offers, COALESCE(proxy_url, ''),
			       datetime(created_at, 'localtime'), datetime(updated_at, 'localtime')
			FROM stores ORDER BY created_at ASC LIMIT 1
		`)
		var s Store
		var activeInt int
		if scanErr := row.Scan(&s.ID, &s.Name, &s.Authorization, &activeInt, &s.PriceDecreaseStep, &s.PriceIncreaseStep,
			&s.RRPPercentage, &s.IntervalMinutes, &s.BulkStock, &s.MaxFetchOffers, &s.ProxyURL, &s.CreatedAt, &s.UpdatedAt); scanErr == nil {
			s.IsActive = activeInt == 1
			// 如果已有店铺缺少 authorization 且传入了有效的 defaultAuth，补全
			if s.Authorization == "" && defaultAuth != "" {
				_, _ = d.conn.Exec(`UPDATE stores SET authorization = ? WHERE id = ?`, defaultAuth, s.ID)
				s.Authorization = defaultAuth
			}
			return &s, nil
		}
	}

	if defaultDecrease <= 0 {
		defaultDecrease = 1
	}
	if defaultIncrease <= 0 {
		defaultIncrease = 1
	}
	if defaultInterval <= 0 {
		defaultInterval = 5
	}
	if defaultBulkStock <= 0 {
		defaultBulkStock = 1
	}
	if defaultMaxFetch <= 0 {
		defaultMaxFetch = 1000
	}
	if defaultRRP < 100 {
		defaultRRP = 120.0
	}

	defaultStore := Store{
		ID:                "default",
		Name:              "默认店铺",
		Authorization:     defaultAuth,
		IsActive:          true,
		PriceDecreaseStep: defaultDecrease,
		PriceIncreaseStep: defaultIncrease,
		RRPPercentage:     defaultRRP,
		IntervalMinutes:   defaultInterval,
		BulkStock:         defaultBulkStock,
		MaxFetchOffers:    defaultMaxFetch,
		ProxyURL:          "",
	}

	_, err = d.conn.Exec(`
		INSERT INTO stores (
			id, name, authorization, is_active, price_decrease_step, price_increase_step,
			rrp_percentage, interval_minutes, bulk_stock, max_fetch_offers, proxy_url, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now', 'localtime'), datetime('now', 'localtime'))
		ON CONFLICT(id) DO NOTHING
	`, defaultStore.ID, defaultStore.Name, defaultStore.Authorization, 1,
		defaultStore.PriceDecreaseStep, defaultStore.PriceIncreaseStep, defaultStore.RRPPercentage,
		defaultStore.IntervalMinutes, defaultStore.BulkStock, defaultStore.MaxFetchOffers, defaultStore.ProxyURL)

	if err != nil {
		return nil, err
	}
	return &defaultStore, nil
}

// ----------------------------------------------------
// 调价监控目标管理 (Reprice Targets)
// ----------------------------------------------------

func (d *DB) SaveStoreTargets(storeID string, targets map[string]config.Target) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if storeID == "" {
		storeID = "default"
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO reprice_targets (store_id, target_key, selected, min_price, max_price, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(store_id, target_key) DO UPDATE SET
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
		if _, err := stmt.Exec(storeID, key, sel, t.MinPrice, t.MaxPrice); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (d *DB) SaveTargets(targets map[string]config.Target) error {
	return d.SaveStoreTargets("default", targets)
}

func (d *DB) LoadStoreTargets(storeID string) (map[string]config.Target, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var query string
	var args []any
	if storeID != "" && storeID != "all" {
		query = `SELECT target_key, selected, min_price, max_price, store_id FROM reprice_targets WHERE store_id = ?`
		args = append(args, storeID)
	} else {
		query = `SELECT target_key, selected, min_price, max_price, store_id FROM reprice_targets`
	}

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := make(map[string]config.Target)
	for rows.Next() {
		var key, stID string
		var selInt, minP, maxP int
		if err := rows.Scan(&key, &selInt, &minP, &maxP, &stID); err != nil {
			continue
		}
		target := config.Target{
			Selected: selInt == 1,
			MinPrice: minP,
			MaxPrice: maxP,
			StoreID:  stID,
		}
		res[key] = target
		if storeID == "all" && stID != "" {
			res[stID+":"+key] = target
		}
	}
	return res, nil
}

func (d *DB) LoadTargets() (map[string]config.Target, error) {
	return d.LoadStoreTargets("default")
}

func (d *DB) CountStoreTargets(storeID string) (int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if storeID == "" {
		storeID = "default"
	}
	var count int
	err := d.conn.QueryRow(`SELECT count(*) FROM reprice_targets WHERE store_id = ?`, storeID).Scan(&count)
	return count, err
}

func (d *DB) CountTargets() (int, error) {
	return d.CountStoreTargets("default")
}

// MigrateGJData 将旧版 GJDATA 迁移至默认店铺
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
		if err := d.SaveStoreTargets("default", targets); err != nil {
			return 0, fmt.Errorf("将迁移数据写入 SQLite reprice_targets 失败: %w", err)
		}
	}

	bakFile := filePath + ".migrated.bak"
	if err := os.Rename(filePath, bakFile); err != nil {
		_ = os.Remove(bakFile)
		_ = os.Rename(filePath, bakFile)
	}

	return len(targets), nil
}

// ----------------------------------------------------
// 调价历史日志 (Reprice History)
// ----------------------------------------------------

func (d *DB) RecordStoreReprice(storeID, targetKey, tsinID, sku, title, imageURL, storeName, action string, oldPrice, newPrice, competitorPrice int, reason string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if storeID == "" {
		storeID = "default"
	}

	_, err := d.conn.Exec(`
		INSERT INTO reprice_history (store_id, target_key, tsin_id, sku, title, image_url, store_name, action, old_price, new_price, competitor_price, reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, storeID, targetKey, tsinID, sku, title, imageURL, storeName, action, oldPrice, newPrice, competitorPrice, reason)
	return err
}

func (d *DB) RecordReprice(targetKey, tsinID, sku, title, imageURL, storeName, action string, oldPrice, newPrice, competitorPrice int, reason string) error {
	return d.RecordStoreReprice("default", targetKey, tsinID, sku, title, imageURL, storeName, action, oldPrice, newPrice, competitorPrice, reason)
}

func (d *DB) GetStoreRepriceHistory(storeID string, limit int) ([]RepriceHistoryRecord, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	var query string
	var args []any
	if storeID != "" && storeID != "all" {
		query = `
			SELECT 
				h.id, 
				h.store_id,
				h.target_key, 
				COALESCE(h.tsin_id, ''), 
				COALESCE(NULLIF(h.sku, ''), c.sku, ''), 
				COALESCE(NULLIF(h.image_url, ''), c.image_url, ''), 
				COALESCE(NULLIF(h.title, ''), c.title, ''), 
				COALESCE(NULLIF(h.store_name, ''), 'Takealot'), 
				COALESCE(NULLIF(h.action, ''), CASE WHEN h.new_price < h.old_price THEN '跟降' WHEN h.new_price > h.old_price THEN '跟涨' ELSE '调价' END), 
				h.old_price, 
				h.new_price, 
				h.competitor_price, 
				COALESCE(h.reason, ''), 
				datetime(h.created_at, 'localtime')
			FROM reprice_history h
			LEFT JOIN cached_offers c ON h.store_id = c.store_id AND ((h.tsin_id != '' AND h.tsin_id = c.tsin_id) OR (h.target_key = c.target_key))
			WHERE h.store_id = ?
			ORDER BY h.id DESC
			LIMIT ?
		`
		args = append(args, storeID, limit)
	} else {
		query = `
			SELECT 
				h.id, 
				h.store_id,
				h.target_key, 
				COALESCE(h.tsin_id, ''), 
				COALESCE(NULLIF(h.sku, ''), c.sku, ''), 
				COALESCE(NULLIF(h.image_url, ''), c.image_url, ''), 
				COALESCE(NULLIF(h.title, ''), c.title, ''), 
				COALESCE(NULLIF(h.store_name, ''), 'Takealot'), 
				COALESCE(NULLIF(h.action, ''), CASE WHEN h.new_price < h.old_price THEN '跟降' WHEN h.new_price > h.old_price THEN '跟涨' ELSE '调价' END), 
				h.old_price, 
				h.new_price, 
				h.competitor_price, 
				COALESCE(h.reason, ''), 
				datetime(h.created_at, 'localtime')
			FROM reprice_history h
			LEFT JOIN cached_offers c ON h.store_id = c.store_id AND ((h.tsin_id != '' AND h.tsin_id = c.tsin_id) OR (h.target_key = c.target_key))
			ORDER BY h.id DESC
			LIMIT ?
		`
		args = append(args, limit)
	}

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]RepriceHistoryRecord, 0)
	for rows.Next() {
		var item RepriceHistoryRecord
		if err := rows.Scan(
			&item.ID, &item.StoreID, &item.TargetKey, &item.TSINID, &item.SKU, &item.ImageURL, &item.Title,
			&item.StoreName, &item.Action, &item.OldPrice, &item.NewPrice, &item.CompetitorPrice,
			&item.Reason, &item.CreatedAt,
		); err != nil {
			continue
		}
		list = append(list, item)
	}
	return list, nil
}

func (d *DB) GetRecentRepriceHistory(limit int) ([]RepriceHistoryRecord, error) {
	return d.GetStoreRepriceHistory("", limit)
}

// ----------------------------------------------------
// 跟卖历史记录 (Follow History)
// ----------------------------------------------------

func (d *DB) RecordStoreFollow(storeID, sourceURL, tsinID, plid, title string, stock, minPrice int, status, message string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if storeID == "" {
		storeID = "default"
	}

	_, err := d.conn.Exec(`
		INSERT INTO follow_history (store_id, source_url, tsin_id, plid, title, stock, min_price, status, message, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, storeID, sourceURL, tsinID, plid, title, stock, minPrice, status, message)
	return err
}

func (d *DB) RecordFollow(sourceURL, tsinID, plid, title string, stock, minPrice int, status, message string) error {
	return d.RecordStoreFollow("default", sourceURL, tsinID, plid, title, stock, minPrice, status, message)
}

func (d *DB) GetStoreFollowHistory(storeID string, limit int) ([]FollowHistoryRecord, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	var query string
	var args []any
	if storeID != "" && storeID != "all" {
		query = `
			SELECT id, store_id, COALESCE(source_url, ''), COALESCE(tsin_id, ''), COALESCE(plid, ''), COALESCE(title, ''), stock, min_price, COALESCE(status, ''), COALESCE(message, ''), datetime(created_at, 'localtime')
			FROM follow_history
			WHERE store_id = ?
			ORDER BY id DESC
			LIMIT ?
		`
		args = append(args, storeID, limit)
	} else {
		query = `
			SELECT id, store_id, COALESCE(source_url, ''), COALESCE(tsin_id, ''), COALESCE(plid, ''), COALESCE(title, ''), stock, min_price, COALESCE(status, ''), COALESCE(message, ''), datetime(created_at, 'localtime')
			FROM follow_history
			ORDER BY id DESC
			LIMIT ?
		`
		args = append(args, limit)
	}

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]FollowHistoryRecord, 0)
	for rows.Next() {
		var item FollowHistoryRecord
		if err := rows.Scan(&item.ID, &item.StoreID, &item.SourceURL, &item.TSINID, &item.PLID, &item.Title, &item.Stock, &item.MinPrice, &item.Status, &item.Message, &item.CreatedAt); err != nil {
			continue
		}
		list = append(list, item)
	}
	return list, nil
}

func (d *DB) GetRecentFollowHistory(limit int) ([]FollowHistoryRecord, error) {
	return d.GetStoreFollowHistory("", limit)
}

// ----------------------------------------------------
// 批量任务记录 (Batch Jobs)
// ----------------------------------------------------

func (d *DB) RecordStoreBatchJob(storeID, batchID, actionType string, itemCount int, status string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if storeID == "" {
		storeID = "default"
	}

	_, err := d.conn.Exec(`
		INSERT INTO batch_jobs (store_id, batch_id, action_type, item_count, status, result_summary, created_at, updated_at)
		VALUES (?, ?, ?, ?, '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(batch_id) DO UPDATE SET
			store_id = excluded.store_id,
			status = excluded.status,
			updated_at = CURRENT_TIMESTAMP
	`, storeID, batchID, actionType, itemCount, status)
	return err
}

func (d *DB) RecordBatchJob(batchID, actionType string, itemCount int, status string) error {
	return d.RecordStoreBatchJob("default", batchID, actionType, itemCount, status)
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

func (d *DB) GetStoreBatchJobs(storeID string, limit int) ([]BatchJobRecord, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	var query string
	var args []any
	if storeID != "" && storeID != "all" {
		query = `
			SELECT id, store_id, batch_id, COALESCE(action_type, ''), item_count, COALESCE(status, ''), COALESCE(result_summary, ''), datetime(created_at, 'localtime'), datetime(updated_at, 'localtime')
			FROM batch_jobs
			WHERE store_id = ?
			ORDER BY id DESC
			LIMIT ?
		`
		args = append(args, storeID, limit)
	} else {
		query = `
			SELECT id, store_id, batch_id, COALESCE(action_type, ''), item_count, COALESCE(status, ''), COALESCE(result_summary, ''), datetime(created_at, 'localtime'), datetime(updated_at, 'localtime')
			FROM batch_jobs
			ORDER BY id DESC
			LIMIT ?
		`
		args = append(args, limit)
	}

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]BatchJobRecord, 0)
	for rows.Next() {
		var item BatchJobRecord
		if err := rows.Scan(&item.ID, &item.StoreID, &item.BatchID, &item.ActionType, &item.ItemCount, &item.Status, &item.ResultSummary, &item.CreatedAt, &item.UpdatedAt); err != nil {
			continue
		}
		list = append(list, item)
	}
	return list, nil
}

func (d *DB) GetRecentBatchJobs(limit int) ([]BatchJobRecord, error) {
	return d.GetStoreBatchJobs("", limit)
}

// ----------------------------------------------------
// 商品本地缓存 (Cached Offers)
// ----------------------------------------------------

func (d *DB) SaveStoreCachedOffers(storeID string, offers []CachedOffer) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if storeID == "" {
		storeID = "default"
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO cached_offers (
			store_id, target_key, tsin_id, sku, plid, title, selling_price, rrp, stock, date_modified,
			best_price, competing_offers, priority_status, price_diff, image_url, image_large_url, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(store_id, target_key) DO UPDATE SET
			sku = CASE WHEN excluded.sku != '' THEN excluded.sku ELSE cached_offers.sku END,
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
			storeID, item.Key, item.TSINID, item.SKU, item.PLID, item.Title, item.SellingPrice, item.RRP, item.Stock, item.DateModified,
			item.BestPrice, item.CompetingOffers, item.PriorityStatus, item.PriceDiff, item.ImageURL, item.ImageLargeURL,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (d *DB) SaveCachedOffers(offers []CachedOffer) error {
	return d.SaveStoreCachedOffers("default", offers)
}

func (d *DB) UpdateStoreSingleMPV(storeID, tsinID string, bestPrice, competing int, priorityStatus string, priceDiff int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if storeID == "" {
		storeID = "default"
	}

	_, err := d.conn.Exec(`
		UPDATE cached_offers
		SET best_price = ?, competing_offers = ?, priority_status = ?, price_diff = ?, updated_at = CURRENT_TIMESTAMP
		WHERE store_id = ? AND tsin_id = ?
	`, bestPrice, competing, priorityStatus, priceDiff, storeID, tsinID)
	return err
}

func (d *DB) UpdateSingleMPV(tsinID string, bestPrice, competing int, priorityStatus string, priceDiff int) error {
	return d.UpdateStoreSingleMPV("default", tsinID, bestPrice, competing, priorityStatus, priceDiff)
}

func (d *DB) LoadStoreCachedOffers(storeID string) ([]CachedOffer, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var query string
	var args []any
	if storeID != "" && storeID != "all" {
		query = `
			SELECT store_id, target_key, tsin_id, COALESCE(sku, ''), plid, COALESCE(title, ''), selling_price, rrp, stock,
			       COALESCE(date_modified, ''), best_price, competing_offers, COALESCE(priority_status, 'solo'),
			       price_diff, COALESCE(image_url, ''), COALESCE(image_large_url, ''),
			       datetime(updated_at, 'localtime')
			FROM cached_offers
			WHERE store_id = ?
			ORDER BY stock DESC, selling_price DESC
		`
		args = append(args, storeID)
	} else {
		query = `
			SELECT store_id, target_key, tsin_id, COALESCE(sku, ''), plid, COALESCE(title, ''), selling_price, rrp, stock,
			       COALESCE(date_modified, ''), best_price, competing_offers, COALESCE(priority_status, 'solo'),
			       price_diff, COALESCE(image_url, ''), COALESCE(image_large_url, ''),
			       datetime(updated_at, 'localtime')
			FROM cached_offers
			ORDER BY stock DESC, selling_price DESC
		`
	}

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]CachedOffer, 0)
	for rows.Next() {
		var item CachedOffer
		if err := rows.Scan(
			&item.StoreID, &item.Key, &item.TSINID, &item.SKU, &item.PLID, &item.Title, &item.SellingPrice, &item.RRP, &item.Stock,
			&item.DateModified, &item.BestPrice, &item.CompetingOffers, &item.PriorityStatus,
			&item.PriceDiff, &item.ImageURL, &item.ImageLargeURL, &item.UpdatedAt,
		); err != nil {
			continue
		}
		list = append(list, item)
	}
	return list, nil
}

func (d *DB) LoadCachedOffers() ([]CachedOffer, error) {
	return d.LoadStoreCachedOffers("default")
}

func (d *DB) GetStoreCachedOffersCount(storeID string) (int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if storeID == "" {
		storeID = "default"
	}

	var count int
	err := d.conn.QueryRow(`SELECT COUNT(*) FROM cached_offers WHERE store_id = ?`, storeID).Scan(&count)
	return count, err
}

func (d *DB) GetCachedOffersCount() (int, error) {
	return d.GetStoreCachedOffersCount("default")
}

// ----------------------------------------------------
// 发货单 (Shipments) 与 送仓预约 (Bookings) 管理方法
// ----------------------------------------------------

func (d *DB) CreateStoreShipment(storeID, shipmentNumber, status, destinationDC, notes string, items []ShipmentItemRecord) (*ShipmentRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if storeID == "" {
		storeID = "default"
	}
	if status == "" {
		status = "draft"
	}
	if destinationDC == "" {
		destinationDC = "JHB"
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var totalUnits int
	var totalValue float64
	for _, it := range items {
		qty := it.ShipQty
		if qty <= 0 {
			qty = it.DemandQty
		}
		if qty <= 0 {
			qty = 1
		}
		totalUnits += qty
		totalValue += it.SellingPrice * float64(qty)
	}
	totalItems := len(items)

	res, err := tx.Exec(`
		INSERT INTO shipments (store_id, shipment_number, status, destination_dc, total_items, total_units, total_value, notes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now', 'localtime'), datetime('now', 'localtime'))
	`, storeID, shipmentNumber, status, destinationDC, totalItems, totalUnits, totalValue, notes)
	if err != nil {
		return nil, fmt.Errorf("创建发货单失败: %w", err)
	}

	shipmentID, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	for _, it := range items {
		shipQty := it.ShipQty
		if shipQty <= 0 {
			shipQty = it.DemandQty
		}
		if shipQty <= 0 {
			shipQty = 1
		}
		weighStatus := it.WeighStatus
		if weighStatus == "" {
			weighStatus = "pending"
		}

		_, err := tx.Exec(`
			INSERT INTO shipment_items (
				shipment_id, order_id, order_item_id, order_date, due_date, tsin, sku, title,
				image_url, selling_price, dc, leadtime_stock, demand_qty, ship_qty,
				actual_weight, volumetric_weight, weigh_status, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now', 'localtime'))
		`, shipmentID, it.OrderID, it.OrderItemID, it.OrderDate, it.DueDate, it.TSIN, it.SKU, it.Title,
			it.ImageURL, it.SellingPrice, it.DC, it.LeadtimeStock, it.DemandQty, shipQty,
			it.ActualWeight, it.VolumetricWeight, weighStatus)
		if err != nil {
			return nil, fmt.Errorf("插入发货单明细失败: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &ShipmentRecord{
		ID:             shipmentID,
		StoreID:        storeID,
		ShipmentNumber: shipmentNumber,
		Status:         status,
		DestinationDC:  destinationDC,
		TotalItems:     totalItems,
		TotalUnits:     totalUnits,
		TotalValue:     totalValue,
		Notes:          notes,
	}, nil
}

func (d *DB) CreateShipment(shipmentNumber, status, destinationDC, notes string, items []ShipmentItemRecord) (*ShipmentRecord, error) {
	return d.CreateStoreShipment("default", shipmentNumber, status, destinationDC, notes, items)
}

func (d *DB) GetStoreShipments(storeID, status string) ([]ShipmentRecord, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var query string
	var args []any
	if storeID != "" && storeID != "all" {
		if status != "" && status != "all" {
			query = `SELECT id, store_id, shipment_number, status, destination_dc, total_items, total_units, total_value, COALESCE(notes, ''),
			                datetime(created_at, 'localtime'), datetime(updated_at, 'localtime')
			         FROM shipments WHERE store_id = ? AND status = ? ORDER BY id DESC`
			args = append(args, storeID, status)
		} else {
			query = `SELECT id, store_id, shipment_number, status, destination_dc, total_items, total_units, total_value, COALESCE(notes, ''),
			                datetime(created_at, 'localtime'), datetime(updated_at, 'localtime')
			         FROM shipments WHERE store_id = ? ORDER BY id DESC`
			args = append(args, storeID)
		}
	} else {
		if status != "" && status != "all" {
			query = `SELECT id, store_id, shipment_number, status, destination_dc, total_items, total_units, total_value, COALESCE(notes, ''),
			                datetime(created_at, 'localtime'), datetime(updated_at, 'localtime')
			         FROM shipments WHERE status = ? ORDER BY id DESC`
			args = append(args, status)
		} else {
			query = `SELECT id, store_id, shipment_number, status, destination_dc, total_items, total_units, total_value, COALESCE(notes, ''),
			                datetime(created_at, 'localtime'), datetime(updated_at, 'localtime')
			         FROM shipments ORDER BY id DESC`
		}
	}

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []ShipmentRecord
	for rows.Next() {
		var s ShipmentRecord
		if err := rows.Scan(
			&s.ID, &s.StoreID, &s.ShipmentNumber, &s.Status, &s.DestinationDC, &s.TotalItems, &s.TotalUnits, &s.TotalValue,
			&s.Notes, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			continue
		}
		list = append(list, s)
	}

	for i := range list {
		items, _ := d.getShipmentItemsInternal(list[i].ID)
		list[i].Items = items
	}

	return list, nil
}

func (d *DB) GetShipments(status string) ([]ShipmentRecord, error) {
	return d.GetStoreShipments("", status)
}

func (d *DB) getShipmentItemsInternal(shipmentID int64) ([]ShipmentItemRecord, error) {
	rows, err := d.conn.Query(`
		SELECT id, shipment_id, order_id, order_item_id, COALESCE(order_date, ''), COALESCE(due_date, ''),
		       COALESCE(tsin, ''), COALESCE(sku, ''), COALESCE(title, ''), COALESCE(image_url, ''),
		       selling_price, COALESCE(dc, ''), leadtime_stock, demand_qty, ship_qty,
		       actual_weight, volumetric_weight, COALESCE(weigh_status, 'pending'),
		       datetime(created_at, 'localtime')
		FROM shipment_items
		WHERE shipment_id = ?
		ORDER BY id ASC
	`, shipmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ShipmentItemRecord
	for rows.Next() {
		var it ShipmentItemRecord
		if err := rows.Scan(
			&it.ID, &it.ShipmentID, &it.OrderID, &it.OrderItemID, &it.OrderDate, &it.DueDate,
			&it.TSIN, &it.SKU, &it.Title, &it.ImageURL, &it.SellingPrice, &it.DC,
			&it.LeadtimeStock, &it.DemandQty, &it.ShipQty, &it.ActualWeight, &it.VolumetricWeight,
			&it.WeighStatus, &it.CreatedAt,
		); err != nil {
			continue
		}
		items = append(items, it)
	}
	return items, nil
}

func (d *DB) UpdateShipmentStatus(shipmentID int64, newStatus string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.conn.Exec(`
		UPDATE shipments
		SET status = ?, updated_at = datetime('now', 'localtime')
		WHERE id = ?
	`, newStatus, shipmentID)
	return err
}

func (d *DB) DeleteShipment(shipmentID int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, _ = d.conn.Exec(`DELETE FROM shipment_items WHERE shipment_id = ?`, shipmentID)
	_, err := d.conn.Exec(`DELETE FROM shipments WHERE id = ?`, shipmentID)
	return err
}

func (d *DB) UpdateShipmentItem(itemID int64, shipQty, leadtimeStock int, actualWeight, volWeight float64, weighStatus string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.conn.Exec(`
		UPDATE shipment_items
		SET ship_qty = ?, leadtime_stock = ?, actual_weight = ?, volumetric_weight = ?,
		    weigh_status = ?
		WHERE id = ?
	`, shipQty, leadtimeStock, actualWeight, volWeight, weighStatus, itemID)
	return err
}

func (d *DB) CreateStoreBooking(storeID string, shipmentID int64, dc, bookingDate, timeSlot, carrier, vehicleReg, notes string) (*BookingRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if storeID == "" {
		storeID = "default"
	}

	bookingNo := fmt.Sprintf("BK-%s-%d", dc, time.Now().Unix()%1000000)
	res, err := d.conn.Exec(`
		INSERT INTO dc_bookings (store_id, booking_number, shipment_id, destination_dc, booking_date, time_slot, carrier_name, vehicle_reg, status, notes, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'scheduled', ?, datetime('now', 'localtime'))
	`, storeID, bookingNo, shipmentID, dc, bookingDate, timeSlot, carrier, vehicleReg, notes)
	if err != nil {
		return nil, err
	}

	id, _ := res.LastInsertId()
	return &BookingRecord{
		ID:            id,
		StoreID:       storeID,
		BookingNumber: bookingNo,
		ShipmentID:    shipmentID,
		DestinationDC: dc,
		BookingDate:   bookingDate,
		TimeSlot:      timeSlot,
		CarrierName:   carrier,
		VehicleReg:    vehicleReg,
		Status:        "scheduled",
		Notes:         notes,
		CreatedAt:     time.Now().Format("2006-01-02 15:04:05"),
	}, nil
}

func (d *DB) CreateBooking(shipmentID int64, dc, bookingDate, timeSlot, carrier, vehicleReg, notes string) (*BookingRecord, error) {
	return d.CreateStoreBooking("default", shipmentID, dc, bookingDate, timeSlot, carrier, vehicleReg, notes)
}

func (d *DB) GetStoreBookings(storeID string) ([]BookingRecord, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var query string
	var args []any
	if storeID != "" && storeID != "all" {
		query = `
			SELECT b.id, b.store_id, b.booking_number, b.shipment_id, COALESCE(s.shipment_number, ''),
			       b.destination_dc, b.booking_date, COALESCE(b.time_slot, ''),
			       COALESCE(b.carrier_name, ''), COALESCE(b.vehicle_reg, ''), b.status,
			       COALESCE(b.notes, ''), datetime(b.created_at, 'localtime')
			FROM dc_bookings b
			LEFT JOIN shipments s ON b.shipment_id = s.id
			WHERE b.store_id = ?
			ORDER BY b.booking_date ASC, b.id DESC
		`
		args = append(args, storeID)
	} else {
		query = `
			SELECT b.id, b.store_id, b.booking_number, b.shipment_id, COALESCE(s.shipment_number, ''),
			       b.destination_dc, b.booking_date, COALESCE(b.time_slot, ''),
			       COALESCE(b.carrier_name, ''), COALESCE(b.vehicle_reg, ''), b.status,
			       COALESCE(b.notes, ''), datetime(b.created_at, 'localtime')
			FROM dc_bookings b
			LEFT JOIN shipments s ON b.shipment_id = s.id
			ORDER BY b.booking_date ASC, b.id DESC
		`
	}

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []BookingRecord
	for rows.Next() {
		var b BookingRecord
		if err := rows.Scan(
			&b.ID, &b.StoreID, &b.BookingNumber, &b.ShipmentID, &b.ShipmentNo, &b.DestinationDC,
			&b.BookingDate, &b.TimeSlot, &b.CarrierName, &b.VehicleReg, &b.Status,
			&b.Notes, &b.CreatedAt,
		); err != nil {
			continue
		}
		list = append(list, b)
	}
	return list, nil
}

func (d *DB) GetBookings() ([]BookingRecord, error) {
	return d.GetStoreBookings("")
}

func (d *DB) UpdateBookingStatus(id int64, status string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.conn.Exec(`UPDATE dc_bookings SET status = ? WHERE id = ?`, status, id)
	return err
}

func (d *DB) DeleteBooking(id int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.conn.Exec(`DELETE FROM dc_bookings WHERE id = ?`, id)
	return err
}

func (d *DB) GetSystemConfig(key string) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var val string
	err := d.conn.QueryRow(`SELECT value FROM system_config WHERE key = ?`, key).Scan(&val)
	if err != nil {
		return "", err
	}
	return val, nil
}

func (d *DB) SetSystemConfig(key, value string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.conn.Exec(
		`INSERT INTO system_config (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`,
		key, value,
	)
	return err
}
