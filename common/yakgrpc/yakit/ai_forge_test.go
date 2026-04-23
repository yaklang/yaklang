package yakit

import (
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func newAIForgeTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, db.AutoMigrate(&schema.AIForge{}).Error)
	return db
}

func newAIForgeTestName(prefix string) string {
	return prefix + "-" + ksuid.New().String()
}

func TestCreateOrUpdateAIForgeByName_CreatePreservesAuthor(t *testing.T) {
	db := newAIForgeTestDB(t)

	forgeName := newAIForgeTestName("create-preserves-author")
	require.NoError(t, CreateOrUpdateAIForgeByName(db, forgeName, &schema.AIForge{
		ForgeName:    forgeName,
		ForgeType:    schema.FORGE_TYPE_YAK,
		ForgeContent: "print('created')",
		Author:       "alice",
	}))

	got, err := GetAIForgeByName(db, forgeName)
	require.NoError(t, err)
	require.Equal(t, "alice", got.Author)
}

func TestCreateOrUpdateAIForgeByName_EmptyAuthorDefaultsToAnonymous(t *testing.T) {
	db := newAIForgeTestDB(t)

	forgeName := newAIForgeTestName("upsert-empty-author-defaults")
	require.NoError(t, CreateOrUpdateAIForgeByName(db, forgeName, &schema.AIForge{
		ForgeName:    forgeName,
		ForgeType:    schema.FORGE_TYPE_YAK,
		ForgeContent: "print('created')",
	}))

	got, err := GetAIForgeByName(db, forgeName)
	require.NoError(t, err)
	require.Equal(t, schema.AIResourceAuthorAnonymous, got.Author)
	require.False(t, got.IsBuiltin)
}

func TestCreateAIForge_EmptyAuthorDefaultsToAnonymous(t *testing.T) {
	db := newAIForgeTestDB(t)

	forgeName := newAIForgeTestName("create-empty-author-defaults")
	require.NoError(t, CreateAIForge(db, &schema.AIForge{
		ForgeName:    forgeName,
		ForgeType:    schema.FORGE_TYPE_YAK,
		ForgeContent: "print('created')",
	}))

	got, err := GetAIForgeByName(db, forgeName)
	require.NoError(t, err)
	require.Equal(t, schema.AIResourceAuthorAnonymous, got.Author)
	require.False(t, got.IsBuiltin)
}

func TestCreateOrUpdateAIForgeByName_BuiltinFlagUsesDirectOverwrite(t *testing.T) {
	db := newAIForgeTestDB(t)

	forgeName := newAIForgeTestName("builtin-flag-direct-overwrite")
	require.NoError(t, CreateAIForge(db, &schema.AIForge{
		ForgeName:    forgeName,
		ForgeType:    schema.FORGE_TYPE_YAK,
		ForgeContent: "print('created')",
		IsBuiltin:    true,
	}))

	require.NoError(t, CreateOrUpdateAIForgeByName(db, forgeName, &schema.AIForge{
		ForgeName:    forgeName,
		ForgeType:    schema.FORGE_TYPE_YAK,
		ForgeContent: "print('updated')",
		IsBuiltin:    false,
	}))

	got, err := GetAIForgeByName(db, forgeName)
	require.NoError(t, err)
	require.False(t, got.IsBuiltin)
}

func TestCreateOrUpdateAIForgeByName_PreservesAuthorOnUpdateAndZeroValues(t *testing.T) {
	db := newAIForgeTestDB(t)

	forgeName := newAIForgeTestName("update-preserves-author")
	require.NoError(t, CreateOrUpdateAIForgeByName(db, forgeName, &schema.AIForge{
		ForgeName:        forgeName,
		ForgeType:        schema.FORGE_TYPE_YAK,
		ForgeContent:     "print('before')",
		Description:      "before-desc",
		Tags:             "tag-a,tag-b",
		PersistentPrompt: "keep-me",
		Author:           "alice",
		IsBuiltin:        true,
	}))

	updateForge := &schema.AIForge{
		ForgeName:        forgeName,
		ForgeType:        schema.FORGE_TYPE_YAK,
		ForgeContent:     "",
		Description:      "",
		Tags:             "",
		PersistentPrompt: "",
		Author:           "bob",
	}
	require.NoError(t, CreateOrUpdateAIForgeByName(db, forgeName, updateForge))
	require.Equal(t, "alice", updateForge.Author)

	got, err := GetAIForgeByName(db, forgeName)
	require.NoError(t, err)
	require.Equal(t, "", got.ForgeContent)
	require.Equal(t, "", got.Description)
	require.Equal(t, "", got.Tags)
	require.Equal(t, "", got.PersistentPrompt)
	require.Equal(t, "alice", got.Author)
	require.True(t, got.IsBuiltin)
}
