package bizhelper

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"gorm.io/gorm"
)

type paginationTestItem struct {
	gorm.Model
	Name string
}

func TestNewPaginationReturnsQueryError(t *testing.T) {
	db, err := createTempTestDatabase()
	require.NoError(t, err)
	defer consts.CloseGormDB(db)

	require.NoError(t, db.AutoMigrate(&paginationTestItem{}))
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
