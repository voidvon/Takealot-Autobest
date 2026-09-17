package db

import (
	"os"
	"path/filepath"
	"testing"
	"takealot/pkg/config"
)

func TestMigrateGJData(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "takealot_db_test_*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	database, err := New(dbPath)
	if err != nil {
		t.Fatalf("初始化数据库失败: %v", err)
	}
	defer database.Close()

	// Create legacy mock GJDATA
	legacyFile := filepath.Join(tempDir, "GJDATA")
	legacyContent := "1001/2001/1/100/500#1002/2002/0/50/200"
	if err := os.WriteFile(legacyFile, []byte(legacyContent), 0644); err != nil {
		t.Fatalf("写入测试 GJDATA 失败: %v", err)
	}

	// Run migration
	count, err := database.MigrateGJData(legacyFile)
	if err != nil {
		t.Fatalf("MigrateGJData 报错: %v", err)
	}
	if count != 2 {
		t.Errorf("期望迁移 2 条，实际迁移: %d", count)
	}

	// Verify legacy file was renamed to .migrated.bak
	if _, err := os.Stat(legacyFile); !os.IsNotExist(err) {
		t.Errorf("旧文件 GJDATA 应当不存在")
	}
	bakFile := legacyFile + ".migrated.bak"
	if _, err := os.Stat(bakFile); err != nil {
		t.Errorf("未找到备份文件 %s: %v", bakFile, err)
	}

	// Verify targets loaded from database
	targets, err := database.LoadTargets()
	if err != nil {
		t.Fatalf("LoadTargets 失败: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("期望数据库中有 2 条数据，实际有: %d", len(targets))
	}

	t1, ok := targets["1001/2001"]
	if !ok || !t1.Selected || t1.MinPrice != 100 || t1.MaxPrice != 500 {
		t.Errorf("目标 1001/2001 数据不符合预期: %+v", t1)
	}

	t2, ok := targets["1002/2002"]
	if !ok || t2.Selected || t2.MinPrice != 50 || t2.MaxPrice != 200 {
		t.Errorf("目标 1002/2002 数据不符合预期: %+v", t2)
	}

	// Re-running on non-existent legacy file should return 0, nil
	count2, err := database.MigrateGJData(legacyFile)
	if err != nil || count2 != 0 {
		t.Errorf("再次运行应返回 0, nil，实际: count=%d, err=%v", count2, err)
	}
}

func TestStoreCRUDAndIsolation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "takealot_db_multistore_*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "multistore.db")
	database, err := New(dbPath)
	if err != nil {
		t.Fatalf("初始化数据库失败: %v", err)
	}
	defer database.Close()

	// 1. EnsureDefaultStore
	defStore, err := database.EnsureDefaultStore("Key test_default_key", 2, 3, 10, 5, 2000, 130.0)
	if err != nil {
		t.Fatalf("EnsureDefaultStore 失败: %v", err)
	}
	if defStore.ID != "default" || defStore.PriceDecreaseStep != 2 {
		t.Fatalf("默认店铺属性不符: %+v", defStore)
	}

	// 2. Create second store
	store2 := Store{
		ID:                "store_2",
		Name:              "二号分店",
		Authorization:     "Key test_store2_key",
		IsActive:          true,
		PriceDecreaseStep: 1,
		PriceIncreaseStep: 1,
		RRPPercentage:     120.0,
		IntervalMinutes:   5,
		BulkStock:         1,
		MaxFetchOffers:    1000,
		ProxyURL:          "http://127.0.0.1:7890",
	}
	if err := database.SaveStore(store2); err != nil {
		t.Fatalf("保存店铺 store_2 失败: %v", err)
	}

	// 3. List stores
	stores, err := database.GetStores()
	if err != nil {
		t.Fatalf("GetStores 失败: %v", err)
	}
	if len(stores) != 2 {
		t.Fatalf("期望 2 个店铺，实际: %d", len(stores))
	}

	// 4. Test target isolation
	targetsStore1 := map[string]config.Target{
		"100/200": {Selected: true, MinPrice: 100, MaxPrice: 300},
	}
	targetsStore2 := map[string]config.Target{
		"100/200": {Selected: true, MinPrice: 150, MaxPrice: 400}, // 同一个商品在店铺2有不同的底价
		"300/400": {Selected: false, MinPrice: 50, MaxPrice: 100},
	}

	if err := database.SaveStoreTargets("default", targetsStore1); err != nil {
		t.Fatalf("SaveStoreTargets default 失败: %v", err)
	}
	if err := database.SaveStoreTargets("store_2", targetsStore2); err != nil {
		t.Fatalf("SaveStoreTargets store_2 失败: %v", err)
	}

	loaded1, err := database.LoadStoreTargets("default")
	if err != nil || loaded1["100/200"].MinPrice != 100 {
		t.Fatalf("店铺1目标数据异常: %+v", loaded1)
	}

	loaded2, err := database.LoadStoreTargets("store_2")
	if err != nil || loaded2["100/200"].MinPrice != 150 || len(loaded2) != 2 {
		t.Fatalf("店铺2目标数据异常: %+v", loaded2)
	}

	// 5. Test cached offers isolation
	offersStore1 := []CachedOffer{
		{Key: "100/200", TSINID: "100", Title: "商品1 (店1)", SellingPrice: 120},
	}
	offersStore2 := []CachedOffer{
		{Key: "100/200", TSINID: "100", Title: "商品1 (店2)", SellingPrice: 180},
	}
	_ = database.SaveStoreCachedOffers("default", offersStore1)
	_ = database.SaveStoreCachedOffers("store_2", offersStore2)

	cached1, _ := database.LoadStoreCachedOffers("default")
	cached2, _ := database.LoadStoreCachedOffers("store_2")
	if len(cached1) != 1 || cached1[0].SellingPrice != 120 {
		t.Errorf("店铺1商品缓存异常: %+v", cached1)
	}
	if len(cached2) != 1 || cached2[0].SellingPrice != 180 {
		t.Errorf("店铺2商品缓存异常: %+v", cached2)
	}

	// 6. Test delete store
	if err := database.DeleteStore("store_2"); err != nil {
		t.Fatalf("删除店铺 store_2 失败: %v", err)
	}
	storesAfter, _ := database.GetStores()
	if len(storesAfter) != 1 {
		t.Errorf("删除后应剩下 1 个店铺，实际: %d", len(storesAfter))
	}
	// Try deleting the last store should fail
	if err := database.DeleteStore("default"); err == nil {
		t.Errorf("删除最后一个店铺应当报错拦截")
	}
}
