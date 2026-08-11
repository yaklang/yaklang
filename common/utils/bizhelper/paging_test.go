package bizhelper

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

type paginationTestItem struct {
	gorm.Model
	Name string
}

func TestCreateTempTestDatabaseIsIsolated(t *testing.T) {
	firstDB, err := createTempTestDatabase()
	require.NoError(t, err)
	defer firstDB.Close()
	secondDB, err := createTempTestDatabase()
	require.NoError(t, err)
	defer secondDB.Close()

	require.NoError(t, firstDB.AutoMigrate(&paginationTestItem{}).Error)
	require.NoError(t, secondDB.AutoMigrate(&paginationTestItem{}).Error)
	require.NoError(t, firstDB.Create(&paginationTestItem{Name: "first-only"}).Error)

	var count int
	require.NoError(t, secondDB.Model(&paginationTestItem{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestNewPaginationReturnsQueryError(t *testing.T) {
	db, err := createTempTestDatabase()
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, db.AutoMigrate(&paginationTestItem{}).Error)
	require.NoError(t, db.Create(&paginationTestItem{Name: "alpha"}).Error)

	var items []paginationTestItem
	_, queryDB := NewPagination(&Param{
		DB:    db.Model(&paginationTestItem{}).Order("missing_column DESC"),
		Page:  1,
		Limit: 10,
	}, &items)

	require.Error(t, queryDB.Error)
	require.Contains(t, queryDB.Error.Error(), "missing_column")
}

func TestNewPaginationRecordsCountAndDataQueryDurations(t *testing.T) {
	db, err := createTempTestDatabase()
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, db.AutoMigrate(&paginationTestItem{}).Error)
	require.NoError(t, db.Create(&paginationTestItem{Name: "alpha"}).Error)

	var items []paginationTestItem
	paginator, queryDB := NewPagination(&Param{
		DB:                 db.Model(&paginationTestItem{}),
		Page:               1,
		Limit:              10,
		DisableTransaction: true,
	}, &items)

	require.NoError(t, queryDB.Error)
	require.True(t, paginator.CountExecuted)
	require.Positive(t, paginator.CountQueryDuration)
	require.Positive(t, paginator.DataQueryDuration)
	require.Len(t, items, 1)

	items = nil
	paginator, queryDB = NewPagination(&Param{
		DB:                 db.Model(&paginationTestItem{}),
		Page:               1,
		Limit:              10,
		SkipCount:          true,
		DisableTransaction: true,
	}, &items)
	require.NoError(t, queryDB.Error)
	require.False(t, paginator.CountExecuted)
	require.Zero(t, paginator.CountQueryDuration)
	require.Positive(t, paginator.DataQueryDuration)
	require.Zero(t, paginator.TotalRecord)
	require.Len(t, items, 1)
}

func TestYakitPagingQueryAcceptsNilPaging(t *testing.T) {
	db, err := createTempTestDatabase()
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, db.AutoMigrate(&paginationTestItem{}).Error)
	require.NoError(t, db.Create(&paginationTestItem{Name: "alpha"}).Error)

	var items []paginationTestItem
	paginator, queryDB := YakitPagingQuery(
		db.Model(&paginationTestItem{}),
		nil,
		&items,
	)

	require.NoError(t, queryDB.Error)
	require.Equal(t, 1, paginator.Page)
	require.Equal(t, 10, paginator.Limit)
	require.Len(t, items, 1)
}
