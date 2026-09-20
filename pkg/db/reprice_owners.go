package db

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// ClaimRepriceProduct atomically reserves both identifiers for one store. Claims
// survive cycles, pauses and restarts, and are released only on store deletion.
// A conflicting alias blocks the entire claim, including when old data has
// different owners for the TSIN and PLID. Errors must never allow repricing.
func (d *DB) ClaimRepriceProduct(storeID, tsinID, plid string) (string, error) {
	keys := make([]string, 0, 2)
	for i, raw := range []string{tsinID, plid} {
		id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err == nil && id > 0 {
			prefix := "tsin:"
			if i == 1 {
				prefix = "plid:"
			}
			keys = append(keys, prefix+strconv.FormatInt(id, 10))
		}
	}
	if storeID == "" || len(keys) == 0 {
		return "", fmt.Errorf("缺少有效的店铺或商品标识")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	tx, err := d.conn.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	for _, key := range keys {
		// Insert first so competing database connections serialize as writers.
		if _, err := tx.Exec(`INSERT INTO reprice_owners (product_key, store_id)
			VALUES (?, ?) ON CONFLICT(product_key) DO NOTHING`, key, storeID); err != nil {
			return "", err
		}
		var owner string
		if err := tx.QueryRow(`SELECT store_id FROM reprice_owners WHERE product_key = ?`, key).Scan(&owner); err != nil {
			return "", err
		}
		if owner != storeID {
			return owner, nil // Roll back any newly inserted aliases.
		}
	}
	// Do not leave claims for a store deleted while its worker was stopping.
	var exists int
	if err := tx.QueryRow(`SELECT 1 FROM stores WHERE id = ?`, storeID).Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("店铺不存在: %s", storeID)
		}
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return storeID, nil
}
