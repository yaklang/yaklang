package bizhelper

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

func TestFuzzSearchManyTermsPreservesMatches(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.Exec("CREATE TABLE search_depth_fixture (name TEXT, description TEXT)").Error)
	require.NoError(t, db.Exec("INSERT INTO search_depth_fixture VALUES (?, ?), (?, ?), (?, ?)", "first-term", "", "", "last-term", "unmatched", "").Error)
	terms := []string{"first-term"}
	for i := 0; i < 600; i++ {
		terms = append(terms, fmt.Sprintf("absent-%d", i))
	}
	terms = append(terms, "last-term")
	var count int
	err = FuzzSearchWithStringArrayOrEx(db.Table("search_depth_fixture"), []string{"name", "description"}, terms, false).Count(&count).Error
	require.NoError(t, err)
	require.Equal(t, 2, count, "both early and late keywords must match, without truncating the query")
}
