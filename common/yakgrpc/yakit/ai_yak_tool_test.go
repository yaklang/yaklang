package yakit

import (
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func newAIYakToolTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, db.AutoMigrate(&schema.AIYakTool{}).Error)
	return db
}

func newAIYakToolTestName(prefix string) string {
	return prefix + "-" + ksuid.New().String()
}

func TestSaveAIYakTool_CreatePreservesAuthor(t *testing.T) {
	db := newAIYakToolTestDB(t)

	toolName := newAIYakToolTestName("create-preserves-author")
	require.NoError(t, func() error {
		_, err := SaveAIYakTool(db, &schema.AIYakTool{
			Name:        toolName,
			Content:     "print('created')",
			Description: "created-desc",
			Author:      "alice",
		})
		return err
	}())

	got, err := GetAIYakTool(db, toolName)
	require.NoError(t, err)
	require.Equal(t, "alice", got.Author)
}

func TestSaveAIYakTool_PreservesAuthorOnUpdateAndZeroValues(t *testing.T) {
	db := newAIYakToolTestDB(t)

	toolName := newAIYakToolTestName("update-preserves-author")
	require.NoError(t, func() error {
		_, err := SaveAIYakTool(db, &schema.AIYakTool{
			Name:              toolName,
			Content:           "print('before')",
			Description:       "before-desc",
			Keywords:          "keyword-a,keyword-b",
			Usage:             "usage-before",
			Params:            `{"type":"object","properties":{"arg":{"type":"string"}}}`,
			Author:            "alice",
			EnableAIOutputLog: 2,
		})
		return err
	}())

	updateTool := &schema.AIYakTool{
		Name:              toolName,
		Content:           "",
		Description:       "",
		Keywords:          "",
		Usage:             "",
		Params:            "",
		Author:            "bob",
		EnableAIOutputLog: 0,
	}
	require.NoError(t, func() error {
		_, err := SaveAIYakTool(db, updateTool)
		return err
	}())
	require.Equal(t, "alice", updateTool.Author)

	got, err := GetAIYakTool(db, toolName)
	require.NoError(t, err)
	require.Equal(t, "", got.Content)
	require.Equal(t, "", got.Description)
	require.Equal(t, "", got.Keywords)
	require.Equal(t, "", got.Usage)
	require.Equal(t, "", got.Params)
	require.Equal(t, "alice", got.Author)
	require.Equal(t, 0, got.EnableAIOutputLog)
}
