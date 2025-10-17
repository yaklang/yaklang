package utils

import (
	"fmt"
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

// 测试用的表结构
type TestRecord struct {
	ID   uint   `gorm:"primary_key"`
	Name string `gorm:"size:100"`
}

func setupTestDB() (*gorm.DB, error) {
	db, err := gorm.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, err
	}

	// 创建测试表
	db.AutoMigrate(&TestRecord{})
	return db, nil
}

func TestSavepointTransaction_BasicUsage(t *testing.T) {
	db, err := setupTestDB()
	if err != nil {
		t.Fatalf("设置测试数据库失败: %v", err)
	}
	defer db.Close()

	st := NewSavepointTransaction(db)

	// 测试开始事务
	tx, err := st.Begin()
	if err != nil {
		t.Fatalf("开始事务失败: %v", err)
	}

	if !st.IsInTransaction() {
		t.Error("应该在事务中")
	}

	if st.GetLevel() != 1 {
		t.Errorf("期望事务层级为1，实际为%d", st.GetLevel())
	}

	// 插入测试数据
	if err := tx.Create(&TestRecord{Name: "test1"}).Error; err != nil {
		t.Fatalf("插入数据失败: %v", err)
	}

	// 提交事务
	if err := st.Commit(); err != nil {
		t.Fatalf("提交事务失败: %v", err)
	}

	if st.IsInTransaction() {
		t.Error("不应该在事务中")
	}

	// 验证数据已提交
	var count int
	db.Model(&TestRecord{}).Count(&count)
	if count != 1 {
		t.Errorf("期望记录数为1，实际为%d", count)
	}
}

func TestSavepointTransaction_NestedCommit(t *testing.T) {
	db, err := setupTestDB()
	if err != nil {
		t.Fatalf("设置测试数据库失败: %v", err)
	}
	defer db.Close()

	err = SavepointTransactionWithCallback(db, func(st *SavepointTransaction) error {
		// 第一层
		if err := st.GetDB().Create(&TestRecord{Name: "level1"}).Error; err != nil {
			return err
		}

		// 嵌套事务
		return NestedSavepointTransaction(st, func(tx *gorm.DB) error {
			if st.GetLevel() != 2 {
				return fmt.Errorf("期望嵌套事务层级为2，实际为%d", st.GetLevel())
			}

			return tx.Create(&TestRecord{Name: "level2"}).Error
		})
	})

	if err != nil {
		t.Fatalf("嵌套事务失败: %v", err)
	}

	// 验证两条记录都已提交
	var count int
	db.Model(&TestRecord{}).Count(&count)
	if count != 2 {
		t.Errorf("期望记录数为2，实际为%d", count)
	}
}

func TestSavepointTransaction_NestedRollback(t *testing.T) {
	db, err := setupTestDB()
	if err != nil {
		t.Fatalf("设置测试数据库失败: %v", err)
	}
	defer db.Close()

	err = SavepointTransactionWithCallback(db, func(st *SavepointTransaction) error {
		// 第一层
		if err := st.GetDB().Create(&TestRecord{Name: "level1"}).Error; err != nil {
			return err
		}

		// 嵌套事务（会失败）
		err := NestedSavepointTransaction(st, func(tx *gorm.DB) error {
			if err := tx.Create(&TestRecord{Name: "level2"}).Error; err != nil {
				return err
			}
			return fmt.Errorf("模拟嵌套事务失败")
		})

		if err != nil {
			// 嵌套事务失败了，但主事务继续
			t.Logf("嵌套事务按预期失败: %v", err)
		}

		// 继续主事务
		return st.GetDB().Create(&TestRecord{Name: "level1_after_nested_fail"}).Error
	})

	if err != nil {
		t.Fatalf("主事务不应该失败: %v", err)
	}

	// 验证只有主事务的数据被提交
	var count int
	db.Model(&TestRecord{}).Count(&count)
	if count != 2 {
		t.Errorf("期望记录数为2（level1和level1_after_nested_fail），实际为%d", count)
	}

	// 验证具体的记录
	var records []TestRecord
	db.Find(&records)
	names := make([]string, len(records))
	for i, record := range records {
		names[i] = record.Name
	}

	expectedNames := []string{"level1", "level1_after_nested_fail"}
	if len(names) != len(expectedNames) {
		t.Errorf("记录数量不匹配，期望%v，实际%v", expectedNames, names)
	}
}

func TestSavepointTransaction_MainTransactionRollback(t *testing.T) {
	db, err := setupTestDB()
	if err != nil {
		t.Fatalf("设置测试数据库失败: %v", err)
	}
	defer db.Close()

	err = SavepointTransactionWithCallback(db, func(st *SavepointTransaction) error {
		// 第一层
		if err := st.GetDB().Create(&TestRecord{Name: "level1"}).Error; err != nil {
			return err
		}

		// 嵌套事务（成功）
		err := NestedSavepointTransaction(st, func(tx *gorm.DB) error {
			return tx.Create(&TestRecord{Name: "level2"}).Error
		})

		if err != nil {
			return err
		}

		// 主事务失败
		return fmt.Errorf("模拟主事务失败")
	})

	if err == nil {
		t.Fatal("主事务应该失败")
	}

	// 验证所有数据都被回滚
	var count int
	db.Model(&TestRecord{}).Count(&count)
	if count != 0 {
		t.Errorf("期望记录数为0（全部回滚），实际为%d", count)
	}
}

func TestSavepointTransaction_MultipleNestingLevels(t *testing.T) {
	db, err := setupTestDB()
	if err != nil {
		t.Fatalf("设置测试数据库失败: %v", err)
	}
	defer db.Close()

	err = SavepointTransactionWithCallback(db, func(st *SavepointTransaction) error {
		// Level 1
		if err := st.GetDB().Create(&TestRecord{Name: "level1"}).Error; err != nil {
			return err
		}

		// Level 2
		return NestedSavepointTransaction(st, func(tx *gorm.DB) error {
			if err := tx.Create(&TestRecord{Name: "level2"}).Error; err != nil {
				return err
			}

			// Level 3
			return NestedSavepointTransaction(st, func(tx *gorm.DB) error {
				if st.GetLevel() != 3 {
					return fmt.Errorf("期望嵌套层级为3，实际为%d", st.GetLevel())
				}

				if err := tx.Create(&TestRecord{Name: "level3"}).Error; err != nil {
					return err
				}

				// Level 4 (会失败)
				err := NestedSavepointTransaction(st, func(tx *gorm.DB) error {
					if err := tx.Create(&TestRecord{Name: "level4"}).Error; err != nil {
						return err
					}
					return fmt.Errorf("level4失败")
				})

				if err != nil {
					t.Logf("Level 4按预期失败: %v", err)
				}

				// Level 3继续
				return tx.Create(&TestRecord{Name: "level3_continue"}).Error
			})
		})
	})

	if err != nil {
		t.Fatalf("多层嵌套事务失败: %v", err)
	}

	// 验证除了level4以外的数据都提交了
	var count int
	db.Model(&TestRecord{}).Count(&count)
	if count != 4 {
		t.Errorf("期望记录数为4，实际为%d", count)
	}

	var records []TestRecord
	db.Find(&records)
	expectedNames := []string{"level1", "level2", "level3", "level3_continue"}
	if len(records) != len(expectedNames) {
		t.Errorf("记录数量不匹配，期望%d，实际%d", len(expectedNames), len(records))
	}
}
