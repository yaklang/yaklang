package aiforge

import (
	"path/filepath"
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func newTestForgeDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	schema.AutoMigrate(db, schema.KEY_SCHEMA_PROFILE_DATABASE)
	return db
}

func assertForgeFields(t *testing.T, expected *schema.AIForge, actual *schema.AIForge) {
	require.Equal(t, expected.ForgeName, actual.ForgeName)
	require.Equal(t, expected.ForgeVerboseName, actual.ForgeVerboseName)
	require.Equal(t, expected.ForgeType, actual.ForgeType)
	require.Equal(t, expected.ForgeContent, actual.ForgeContent)
	require.Equal(t, expected.ParamsUIConfig, actual.ParamsUIConfig)
	require.Equal(t, expected.Params, actual.Params)
	require.Equal(t, expected.UserPersistentData, actual.UserPersistentData)
	require.Equal(t, expected.Description, actual.Description)
	require.Equal(t, expected.Tools, actual.Tools)
	require.Equal(t, expected.ToolKeywords, actual.ToolKeywords)
	require.Equal(t, expected.Actions, actual.Actions)
	require.Equal(t, expected.Tags, actual.Tags)
	require.Equal(t, expected.InitPrompt, actual.InitPrompt)
	require.Equal(t, expected.PersistentPrompt, actual.PersistentPrompt)
	require.Equal(t, expected.PlanPrompt, actual.PlanPrompt)
	require.Equal(t, expected.ResultPrompt, actual.ResultPrompt)
	require.Equal(t, expected.Author, actual.Author)
}

func TestExportImportYakForge_AllFieldsAndProgress(t *testing.T) {
	db := newTestForgeDB(t)
	defer db.Close()

	forge := &schema.AIForge{
		ForgeName:          "yak-" + t.Name(),
		ForgeVerboseName:   "yak-forge",
		ForgeType:          schema.FORGE_TYPE_YAK,
		ForgeContent:       "println('hello')",
		ParamsUIConfig:     `{"ui":"yak"}`,
		Params:             "--flag",
		UserPersistentData: "user-data",
		Description:        "yak desc",
		Tools:              "tool1,tool2",
		ToolKeywords:       "kw1,kw2",
		Actions:            "act1",
		Tags:               "tag1,tag2",
		InitPrompt:         "yak init",
		PersistentPrompt:   "yak persist",
		PlanPrompt:         "yak plan",
		ResultPrompt:       "yak result",
		Author:             "yak-author",
	}
	require.NoError(t, yakit.CreateAIForge(db, forge))

	var progressMsg []string
	progress := func(percent float64, msg string) {
		progressMsg = append(progressMsg, msg)
	}

	target := filepath.Join(t.TempDir(), "yak.tar.gz")
	exported, err := ExportAIForgesToTarGz(db, []string{forge.ForgeName}, target, WithForgeProgress(progress))
	require.NoError(t, err)
	require.Equal(t, target, exported)
	require.NotEmpty(t, progressMsg)

	db.Unscoped().Where("forge_name = ?", forge.ForgeName).Delete(&schema.AIForge{})

	progressMsg = nil
	imported, err := ImportAIForgesFromTarGz(db, exported, WithForgeProgress(progress))
	require.NoError(t, err)
	require.Len(t, imported, 1)
	require.NotEmpty(t, progressMsg)

	stored, err := yakit.GetAIForgeByName(db, forge.ForgeName)
	require.NoError(t, err)
	assertForgeFields(t, forge, stored)
	assertForgeFields(t, forge, imported[0])
}

func TestExportImportConfigForge_WithRenameAuthorAndOverwrite(t *testing.T) {
	db := newTestForgeDB(t)
	defer db.Close()

	forge := &schema.AIForge{
		ForgeName:          "config-" + t.Name(),
		ForgeVerboseName:   "config-forge",
		ForgeType:          schema.FORGE_TYPE_Config,
		ForgeContent:       "println('config')",
		InitPrompt:         "init prompt",
		PersistentPrompt:   "persistent prompt",
		PlanPrompt:         "plan prompt",
		ResultPrompt:       "result prompt",
		Params:             "--rule",
		ParamsUIConfig:     `{"ui":"config"}`,
		UserPersistentData: "userdata",
		Description:        "desc",
		Tools:              "tool1,tool2",
		ToolKeywords:       "kw1,kw2",
		Actions:            "act",
		Tags:               "t1,t2",
		Author:             "old-author",
	}
	require.NoError(t, yakit.CreateAIForge(db, forge))

	target := filepath.Join(t.TempDir(), "config.tar.gz")
	exported, err := ExportAIForgesToTarGz(db, []string{forge.ForgeName}, target, WithForgeAuthor("export-author"))
	require.NoError(t, err)

	db.Unscoped().Where("forge_name = ?", forge.ForgeName).Delete(&schema.AIForge{})

	newName := forge.ForgeName + "-new"
	imported, err := ImportAIForgesFromTarGz(db, exported,
		WithForgeNewName(newName),
		WithForgeAuthor("new-author"),
		WithForgeOverwrite(true),
	)
	require.NoError(t, err)
	require.Len(t, imported, 1)

	_, err = yakit.GetAIForgeByName(db, forge.ForgeName)
	require.Error(t, err, "old name should not exist after rename")

	stored, err := yakit.GetAIForgeByName(db, newName)
	require.NoError(t, err)
	forgeRenamed := *forge
	forgeRenamed.ForgeName = newName
	forgeRenamed.Author = "new-author"
	assertForgeFields(t, &forgeRenamed, stored)
	assertForgeFields(t, &forgeRenamed, imported[0])
}

func TestExportImportMultipleForges_WithDBValidation(t *testing.T) {
	db := newTestForgeDB(t)
	defer db.Close()

	yakForge := &schema.AIForge{
		ForgeName:        "yak-" + t.Name(),
		ForgeType:        schema.FORGE_TYPE_YAK,
		ForgeContent:     "println('yak')",
		ForgeVerboseName: "yak-multi",
		Author:           "yak-multi-author",
	}
	cfgForge := &schema.AIForge{
		ForgeName:        "cfg-" + t.Name(),
		ForgeType:        schema.FORGE_TYPE_Config,
		ForgeContent:     "println('cfg')",
		InitPrompt:       "init",
		PersistentPrompt: "persist",
		ForgeVerboseName: "cfg-multi",
		Author:           "cfg-multi-author",
	}
	require.NoError(t, yakit.CreateAIForge(db, yakForge))
	require.NoError(t, yakit.CreateAIForge(db, cfgForge))

	target := filepath.Join(t.TempDir(), "multi.tar.gz")
	exported, err := ExportAIForgesToTarGz(db, []string{yakForge.ForgeName, cfgForge.ForgeName}, target, WithForgeOverwrite(true))
	require.NoError(t, err)
	require.Equal(t, target, exported)

	db.Unscoped().Where("forge_name IN (?)", []string{yakForge.ForgeName, cfgForge.ForgeName}).Delete(&schema.AIForge{})

	imported, err := ImportAIForgesFromTarGz(db, exported, WithForgeOverwrite(true))
	require.NoError(t, err)
	require.Len(t, imported, 2)

	storedYak, err := yakit.GetAIForgeByName(db, yakForge.ForgeName)
	require.NoError(t, err)
	assertForgeFields(t, yakForge, storedYak)

	storedCfg, err := yakit.GetAIForgeByName(db, cfgForge.ForgeName)
	require.NoError(t, err)
	assertForgeFields(t, cfgForge, storedCfg)
}
