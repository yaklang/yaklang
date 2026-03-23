package bizhelper

import (
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"
)

type paginationTestItem struct {
	gorm.Model
	Name string
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
