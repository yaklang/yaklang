package bizhelper

import (
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

type testData struct {
	gorm.Model
	Key   string
	Value int
}

func TestGroupColumn(t *testing.T) {
	db, err := createTempTestDatabase()
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Close()
	})

	db = db.Debug().AutoMigrate(&testData{}).Model(&testData{})

	token1, token2, token3 := utils.RandStringBytes(10), utils.RandStringBytes(10), utils.RandStringBytes(10)
	for i := 1; i < 6; i++ {
		db.Save(&testData{Key: token1, Value: i})
		db.Save(&testData{Key: token2, Value: i})
		db.Save(&testData{Key: token3, Value: i})
	}

	// test string
	data, err := GroupColumn(db, "test_data", "Key")
	require.NoError(t, err)
	require.Len(t, data, 3)
	for _, datum := range data {
		require.NotEmpty(t, datum)
	}

	fieldGroup := GroupCount(db, "test_data", "Key")
	require.Len(t, fieldGroup, 3)
	for _, group := range fieldGroup {
		require.Equal(t, int(group.Total), 5)
	}

	// test int
	data, err = GroupColumn(db, "test_data", "Value")
	require.NoError(t, err)
	require.Len(t, data, 5)
	for _, datum := range data {
		require.NotEmpty(t, datum)
	}

	fieldGroup = GroupCount(db, "test_data", "Value")
	require.Len(t, fieldGroup, 5)
	for _, group := range fieldGroup {
		require.Equal(t, int(group.Total), 3)
	}
}
