package db

import (
	"os"
	"path/filepath"
	"testing"
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
