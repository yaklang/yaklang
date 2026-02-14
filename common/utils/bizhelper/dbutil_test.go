package bizhelper

import (
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/stretchr/testify/assert"
)

type TestModelUInt64 struct {
	ID uint `gorm:"primary_key"`
	A  uint64
	B  uint64
}

func TestExactQueryMultipleUInt64ArrayOr(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	assert.NoError(t, err)
	defer db.Close()

	db.AutoMigrate(&TestModelUInt64{})

	db.Create(&TestModelUInt64{ID: 1, A: 100, B: 200})
	db.Create(&TestModelUInt64{ID: 2, A: 300, B: 400})
	db.Create(&TestModelUInt64{ID: 3, A: 500, B: 600})

	t.Run("Single match A", func(t *testing.T) {
		var results []TestModelUInt64
		ExactQueryMultipleUInt64ArrayOr(db, []string{"A", "B"}, []uint64{100}).Find(&results)
		assert.Equal(t, 1, len(results))
		assert.Equal(t, uint(1), results[0].ID)
	})

	t.Run("Single match B", func(t *testing.T) {
		var results []TestModelUInt64
		ExactQueryMultipleUInt64ArrayOr(db, []string{"A", "B"}, []uint64{400}).Find(&results)
		assert.Equal(t, 1, len(results))
		assert.Equal(t, uint(2), results[0].ID)
	})

	t.Run("Multiple match", func(t *testing.T) {
		var results []TestModelUInt64
		ExactQueryMultipleUInt64ArrayOr(db, []string{"A", "B"}, []uint64{100, 400}).Find(&results)
		assert.Equal(t, 2, len(results))
	})

	t.Run("No match", func(t *testing.T) {
		var results []TestModelUInt64
		ExactQueryMultipleUInt64ArrayOr(db, []string{"A", "B"}, []uint64{999}).Find(&results)
		assert.Equal(t, 0, len(results))
	})
}
