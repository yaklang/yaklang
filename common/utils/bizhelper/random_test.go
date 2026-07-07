package bizhelper

import (
	"fmt"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Item struct {
	ID   uint `gorm:"primaryKey"`
	Name string
}

func setupDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite in-memory db: %v", err)
	}
	if err := db.AutoMigrate(&Item{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	return db
}
func closeGormDB(db *gorm.DB) {
	sqlDB, _ := db.DB()
	if sqlDB != nil {
		sqlDB.Close()
	}
}

func seedItems(t *testing.T, db *gorm.DB, n int) {
	for i := 0; i < n; i++ {
		it := Item{Name: fmt.Sprintf("item-%d", i)}
		if err := db.Create(&it).Error; err != nil {
			t.Fatalf("failed to seed item: %v", err)
		}
	}
}

func TestRandomQuery_EdgeCases(t *testing.T) {
	t.Run("empty table", func(t *testing.T) {
		db := setupDB(t)
		defer closeGormDB(db)

		var out []Item
		total, err := RandomQuery(db.Model(&Item{}), 5, &out)
		if err != nil {
			t.Fatalf("RandomQuery error: %v", err)
		}
		if total != 0 {
			t.Fatalf("expected total 0, got %d", total)
		}
		if len(out) != 0 {
			t.Fatalf("expected 0 results, got %d", len(out))
		}
	})

	t.Run("total less than limit", func(t *testing.T) {
		db := setupDB(t)
		defer closeGormDB(db)
		seedItems(t, db, 3)

		var out []Item
		total, err := RandomQuery(db.Model(&Item{}), 5, &out)
		if err != nil {
			t.Fatalf("RandomQuery error: %v", err)
		}
		if total != 3 {
			t.Fatalf("expected total 3, got %d", total)
		}
		if len(out) != 3 {
			t.Fatalf("expected 3 results, got %d", len(out))
		}
	})

	t.Run("total equals limit", func(t *testing.T) {
		db := setupDB(t)
		defer closeGormDB(db)
		seedItems(t, db, 5)

		var out []Item
		total, err := RandomQuery(db.Model(&Item{}), 5, &out)
		if err != nil {
			t.Fatalf("RandomQuery error: %v", err)
		}
		if total != 5 {
			t.Fatalf("expected total 5, got %d", total)
		}
		if len(out) != 5 {
			t.Fatalf("expected 5 results, got %d", len(out))
		}
	})

	t.Run("total greater than limit", func(t *testing.T) {
		db := setupDB(t)
		defer closeGormDB(db)
		seedItems(t, db, 10)

		var out []Item
		total, err := RandomQuery(db.Model(&Item{}), 5, &out)
		if err != nil {
			t.Fatalf("RandomQuery error: %v", err)
		}
		if total != 10 {
			t.Fatalf("expected total 10, got %d", total)
		}
		if len(out) != 5 {
			t.Fatalf("expected 5 results, got %d", len(out))
		}
	})

	t.Run("limit zero", func(t *testing.T) {
		db := setupDB(t)
		defer closeGormDB(db)
		seedItems(t, db, 5)

		var out []Item
		total, err := RandomQuery(db.Model(&Item{}), 0, &out)
		if err != nil {
			t.Fatalf("RandomQuery error: %v", err)
		}
		if total != 5 {
			t.Fatalf("expected total 5, got %d", total)
		}
		if len(out) != 0 {
			t.Fatalf("expected 0 results for limit 0, got %d", len(out))
		}
	})
}
