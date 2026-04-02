package yakit

import (
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func newYakScriptTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, db.AutoMigrate(&schema.YakScript{}).Error)
	return db
}

func TestCreateOrUpdateYakScriptByName_CreatesAndUpdatesSingleRecord(t *testing.T) {
	db := newYakScriptTestDB(t)

	const scriptName = "create-or-update-by-name"
	require.NoError(t, CreateOrUpdateYakScriptByName(db, scriptName, &schema.YakScript{
		ScriptName: scriptName,
		Type:       "yak",
		Content:    "print('v1')",
		Help:       "first version",
	}))

	require.NoError(t, CreateOrUpdateYakScriptByName(db, scriptName, &schema.YakScript{
		ScriptName: scriptName,
		Type:       "yak",
		Content:    "print('v2')",
		Help:       "second version",
	}))

	got, err := GetYakScriptByName(db, scriptName)
	require.NoError(t, err)
	require.Equal(t, "print('v2')", got.Content)
	require.Equal(t, "second version", got.Help)

	var count int
	require.NoError(t, db.Model(&schema.YakScript{}).Where("script_name = ?", scriptName).Count(&count).Error)
	require.Equal(t, 1, count)
}

func TestCreateOrUpdateYakScriptByName_PersistsZeroValues(t *testing.T) {
	db := newYakScriptTestDB(t)

	const scriptName = "zero-value-update-by-name"
	require.NoError(t, CreateOrUpdateYakScriptByName(db, scriptName, &schema.YakScript{
		ScriptName:           scriptName,
		Type:                 "yak",
		Content:              "print('before')",
		Params:               "\"[{\\\"Field\\\":\\\"target\\\"}]\"",
		EnablePluginSelector: true,
		PluginSelectorTypes:  "mitm",
		EnableForAI:          true,
		AIDesc:               "desc",
		AIKeywords:           "k1,k2",
		AIUsage:              "usage",
		OnlineId:             123,
		OnlineIsPrivate:      true,
		SkipUpdate:           true,
		ForceInteractive:     true,
		IsGeneralModule:      true,
		GeneralModuleVerbose: "verbose",
		GeneralModuleKey:     "module-key",
	}))

	require.NoError(t, CreateOrUpdateYakScriptByName(db, scriptName, &schema.YakScript{
		ScriptName:           scriptName,
		Type:                 "yak",
		Content:              "",
		Params:               "",
		EnablePluginSelector: false,
		PluginSelectorTypes:  "",
		EnableForAI:          false,
		AIDesc:               "",
		AIKeywords:           "",
		AIUsage:              "",
		OnlineId:             0,
		OnlineIsPrivate:      false,
		SkipUpdate:           false,
		ForceInteractive:     false,
		IsGeneralModule:      false,
		GeneralModuleVerbose: "",
		GeneralModuleKey:     "",
	}))

	got, err := GetYakScriptByName(db, scriptName)
	require.NoError(t, err)
	require.Equal(t, "", got.Content)
	require.Equal(t, "", got.Params)
	require.False(t, got.EnablePluginSelector)
	require.Equal(t, "", got.PluginSelectorTypes)
	require.False(t, got.EnableForAI)
	require.Equal(t, "", got.AIDesc)
	require.Equal(t, "", got.AIKeywords)
	require.Equal(t, "", got.AIUsage)
	require.EqualValues(t, 0, got.OnlineId)
	require.False(t, got.OnlineIsPrivate)
	require.False(t, got.SkipUpdate)
	require.False(t, got.ForceInteractive)
	require.False(t, got.IsGeneralModule)
	require.Equal(t, "", got.GeneralModuleVerbose)
	require.Equal(t, "", got.GeneralModuleKey)
}

func TestCreateOrUpdateYakScriptByName_MapUpdateOnlyTouchesSpecifiedFields(t *testing.T) {
	db := newYakScriptTestDB(t)

	const scriptName = "map-update-by-name"
	require.NoError(t, CreateOrUpdateYakScriptByName(db, scriptName, &schema.YakScript{
		ScriptName:  scriptName,
		Type:        "yak",
		Content:     "print('before')",
		EnableForAI: true,
		AIDesc:      "desc",
	}))

	require.NoError(t, CreateOrUpdateYakScriptByName(db, scriptName, map[string]interface{}{
		"enable_for_ai": false,
	}))

	got, err := GetYakScriptByName(db, scriptName)
	require.NoError(t, err)
	require.Equal(t, "print('before')", got.Content)
	require.False(t, got.EnableForAI)
	require.Equal(t, "desc", got.AIDesc)
}

func TestCreateOrUpdateYakScript_CreatesNewRecordWhenIDMissing(t *testing.T) {
	db := newYakScriptTestDB(t)

	const scriptName = "create-or-update-by-id"
	require.NoError(t, CreateOrUpdateYakScriptByID(db, 0, &schema.YakScript{
		ScriptName: scriptName,
		Type:       "yak",
		Content:    "print('created')",
	}))

	got, err := GetYakScriptByName(db, scriptName)
	require.NoError(t, err)
	require.NotZero(t, got.ID)
	require.Equal(t, "print('created')", got.Content)
}

func TestCreateOrUpdateYakScriptByID_PersistsZeroValues(t *testing.T) {
	db := newYakScriptTestDB(t)

	const scriptName = "zero-value-update-by-id"
	require.NoError(t, CreateOrUpdateYakScriptByName(db, scriptName, &schema.YakScript{
		ScriptName:  scriptName,
		Type:        "yak",
		Content:     "print('before')",
		EnableForAI: true,
		AIDesc:      "desc",
		AIKeywords:  "k1,k2",
		AIUsage:     "usage",
	}))

	existing, err := GetYakScriptByName(db, scriptName)
	require.NoError(t, err)

	require.NoError(t, CreateOrUpdateYakScriptByID(db, int64(existing.ID), &schema.YakScript{
		ScriptName:  scriptName,
		Type:        "yak",
		Content:     "",
		EnableForAI: false,
		AIDesc:      "",
		AIKeywords:  "",
		AIUsage:     "",
	}))

	got, err := GetYakScript(db, int64(existing.ID))
	require.NoError(t, err)
	require.Equal(t, "", got.Content)
	require.False(t, got.EnableForAI)
	require.Equal(t, "", got.AIDesc)
	require.Equal(t, "", got.AIKeywords)
	require.Equal(t, "", got.AIUsage)
}

func TestCreateOrUpdateYakScript_MapUpdateOnlyTouchesSpecifiedFields(t *testing.T) {
	db := newYakScriptTestDB(t)

	const scriptName = "map-update-by-id"
	require.NoError(t, CreateOrUpdateYakScriptByName(db, scriptName, &schema.YakScript{
		ScriptName: scriptName,
		Type:       "yak",
		Content:    "print('before')",
		Ignored:    false,
		Help:       "keep me",
	}))

	existing, err := GetYakScriptByName(db, scriptName)
	require.NoError(t, err)

	require.NoError(t, CreateOrUpdateYakScriptByID(db, int64(existing.ID), map[string]interface{}{
		"ignored": true,
	}))

	got, err := GetYakScript(db, int64(existing.ID))
	require.NoError(t, err)
	require.Equal(t, "print('before')", got.Content)
	require.Equal(t, "keep me", got.Help)
	require.True(t, got.Ignored)
}
