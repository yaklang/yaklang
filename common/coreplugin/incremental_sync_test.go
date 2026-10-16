package coreplugin

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/contextmenu"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func newCorePluginSyncDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.YakScript{}, &schema.GeneralStorage{}).Error)
	return db
}

func corePluginFixture(name string) *builtinCorePlugin {
	return &builtinCorePlugin{YakScript: &schema.YakScript{
		ScriptName: name, Content: `cli.String("target"); cli.check()`,
		Type: "yak", IsCorePlugin: true, Uuid: "uuid-" + name, Author: "yaklang.io",
	}, generateParam: true}
}

func countCorePluginParses(t *testing.T) *atomic.Int32 {
	t.Helper()
	var count atomic.Int32
	previous := parseCorePlugin
	t.Cleanup(func() { parseCorePlugin = previous })
	parseCorePlugin = func(code, typ string, opts ...ssaconfig.Option) (*ssaapi.Program, error) {
		count.Add(1)
		return previous(code, typ, opts...)
	}
	return &count
}

func TestCorePluginIncrementalMigration(t *testing.T) {
	db := newCorePluginSyncDB(t)
	plugins := map[string]*builtinCorePlugin{"first": corePluginFixture("first"), "second": corePluginFixture("second")}
	legacy := *plugins["first"].YakScript
	legacy.Uuid = "legacy-action-uuid"
	legacy.Type, legacy.Tags = schema.SCRIPT_TYPE_CODEC, contextmenu.LegacyTagHistorySingle
	legacy.Level, legacy.EnablePluginSelector, legacy.SkipUpdate = "user-level", true, true
	require.NoError(t, db.Create(&legacy).Error)
	custom := &schema.YakScript{ScriptName: "custom", Content: "user content", Author: "yaklang.io"}
	require.NoError(t, db.Create(custom).Error)
	plugins["custom"] = corePluginFixture("custom")
	count := countCorePluginParses(t)
	require.NoError(t, syncCorePlugins(db, plugins, "archive:1", true))
	require.EqualValues(t, 2, count.Swap(0), "legacy rows reparse once; colliding user rows are skipped")
	first, err := yakit.GetYakScriptByName(db, "first")
	require.NoError(t, err)
	require.Equal(t, legacy.ID, first.ID)
	require.Equal(t, legacy.Uuid, first.Uuid)
	require.Equal(t, legacy.Type, first.Type)
	require.Equal(t, legacy.Level, first.Level)
	require.True(t, first.EnablePluginSelector)
	require.True(t, first.SkipUpdate)
	require.True(t, contextmenu.HasTag(first.Tags, contextmenu.LegacyTagHistorySingle))
	require.NotEmpty(t, first.Params)
	second, err := yakit.GetYakScriptByName(db, "second")
	require.NoError(t, err)

	require.NoError(t, syncCorePlugins(db, plugins, "repacked:1", true))
	require.Zero(t, count.Swap(0))
	plugins["first"].Content = `cli.String("TARGET"); cli.check()`
	start := time.Now()
	require.NoError(t, syncCorePlugins(db, plugins, "changed:1", true))
	t.Logf("one changed item: %s", time.Since(start))
	require.EqualValues(t, 1, count.Swap(0))
	first, err = yakit.GetYakScriptByName(db, "first")
	require.NoError(t, err)
	require.Contains(t, first.Params, "TARGET")
	require.Equal(t, legacy.ID, first.ID)
	require.Equal(t, legacy.Uuid, first.Uuid)
	untouched, err := yakit.GetYakScriptByName(db, "second")
	require.NoError(t, err)
	require.Equal(t, second.UpdatedAt.UnixNano(), untouched.UpdatedAt.UnixNano())

	plugins["first"].Help = "changed Go metadata"
	require.NoError(t, syncCorePlugins(db, plugins, "metadata:1", true))
	require.EqualValues(t, 1, count.Swap(0))
	// Changing the recorded extractor invalidates every item, even with identical code.
	require.NoError(t, yakit.SetKey(db, corePluginItemHashesKey, `{"first":"old-extractor","second":"old-extractor"}`))
	require.NoError(t, syncCorePlugins(db, plugins, "archive:2", true))
	require.EqualValues(t, 2, count.Swap(0))

	delete(plugins, "second")
	delete(plugins, "custom")
	require.NoError(t, syncCorePlugins(db, plugins, "removed:2", true))
	require.Zero(t, count.Swap(0))
	_, err = yakit.GetYakScriptByName(db, "second")
	require.Error(t, err)
	preserved, err := yakit.GetYakScriptByName(db, "custom")
	require.NoError(t, err)
	require.Equal(t, custom.Content, preserved.Content)
}

func TestCorePluginSyncRollbackAndRetry(t *testing.T) {
	db := newCorePluginSyncDB(t)
	plugins := map[string]*builtinCorePlugin{"first": corePluginFixture("first"), "retired": corePluginFixture("retired")}
	require.NoError(t, syncCorePlugins(db, plugins, "legacy", true))
	oldHashes := yakit.GetKey(db, corePluginItemHashesKey)
	old, err := yakit.GetYakScriptByName(db, "first")
	require.NoError(t, err)
	plugins["first"].Content = `cli.Int("number"); cli.check()`
	plugins["new"] = corePluginFixture("new")
	delete(plugins, "retired")
	require.NoError(t, db.Exec(fmt.Sprintf(`CREATE TRIGGER fail_core_marker BEFORE UPDATE ON general_storages WHEN NEW.key = '%s' BEGIN SELECT RAISE(FAIL, 'marker failure'); END`, `"`+corePluginEmbedKey+`"`)).Error)
	require.ErrorContains(t, syncCorePlugins(db, plugins, "new:1", true), "marker failure")
	require.Equal(t, "legacy", yakit.GetKey(db, corePluginEmbedKey))
	require.Equal(t, oldHashes, yakit.GetKey(db, corePluginItemHashesKey))
	preserved, err := yakit.GetYakScriptByName(db, "first")
	require.NoError(t, err)
	require.Equal(t, old.Content, preserved.Content)
	require.Equal(t, old.UpdatedAt.UnixNano(), preserved.UpdatedAt.UnixNano())
	_, err = yakit.GetYakScriptByName(db, "retired")
	require.NoError(t, err)
	_, err = yakit.GetYakScriptByName(db, "new")
	require.Error(t, err)
	require.NoError(t, db.Exec(`DROP TRIGGER fail_core_marker`).Error)
	require.NoError(t, syncCorePlugins(db, plugins, "new:1", true))
	require.Equal(t, "new:1", yakit.GetKey(db, corePluginEmbedKey))

	plugins["first"].Content = "func {"
	require.ErrorContains(t, syncCorePlugins(db, plugins, "broken:1", true), "parse core plugin")
	require.Equal(t, "new:1", yakit.GetKey(db, corePluginEmbedKey))
}

func TestCorePluginItemHashStorage(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		invalid   bool
	}{
		{name: "empty"},
		{name: "null", raw: "null"},
		{name: "null-hash", raw: `{"first":null}`},
		{name: "legacy-json", raw: `{"first":"old-extractor"}`},
		{name: "whitespace", raw: " \n\t", invalid: true},
		{name: "malformed", raw: "{", invalid: true},
		{name: "array", raw: "[]", invalid: true},
		{name: "scalar", raw: "true", invalid: true},
		{name: "non-string-hash", raw: `{"first":1}`, invalid: true},
		{name: "nested-hash", raw: `{"first":{"nested":"value"}}`, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := newCorePluginSyncDB(t)
			require.NoError(t, yakit.SetKey(db, corePluginItemHashesKey, tc.raw))
			require.NoError(t, yakit.SetKey(db, corePluginEmbedKey, "previous"))
			plugins := map[string]*builtinCorePlugin{"first": corePluginFixture("first")}
			count := countCorePluginParses(t)
			if tc.invalid {
				require.ErrorContains(t, syncCorePlugins(db, plugins, "archive:1", true), "read core plugin item hashes")
				require.Zero(t, count.Load())
				require.Equal(t, "previous", yakit.GetKey(db, corePluginEmbedKey))
				require.Equal(t, tc.raw, yakit.GetKey(db, corePluginItemHashesKey))
				var rows int
				require.NoError(t, db.Model(&schema.YakScript{}).Count(&rows).Error)
				require.Zero(t, rows)
				require.NoError(t, yakit.SetKey(db, corePluginItemHashesKey, "{}"))
			}
			require.NoError(t, syncCorePlugins(db, plugins, "archive:1", true))
			require.EqualValues(t, 1, count.Swap(0))
			// The shared serializer must keep the existing KV format readable.
			require.NoError(t, syncCorePlugins(db, plugins, "repacked:1", true))
			require.Zero(t, count.Load())
		})
	}
}

func TestCorePluginSyncRejectsNilDefinitions(t *testing.T) {
	db := newCorePluginSyncDB(t)
	previous := corePluginFixture("first").YakScript
	require.NoError(t, db.Create(previous).Error)
	require.NoError(t, yakit.SetKey(db, corePluginEmbedKey, "previous"))
	require.NoError(t, yakit.SetKey(db, corePluginItemHashesKey, "{}"))
	count := countCorePluginParses(t)
	for _, tc := range []struct {
		name    string
		plugins map[string]*builtinCorePlugin
	}{
		{name: "nil-set"},
		{name: "nil-entry", plugins: map[string]*builtinCorePlugin{"invalid": nil}},
		{name: "nil-script", plugins: map[string]*builtinCorePlugin{"invalid": {}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NotPanics(t, func() {
				require.Error(t, syncCorePlugins(db, tc.plugins, "new:1", true))
			})
			preserved, err := yakit.GetYakScriptByName(db, "first")
			require.NoError(t, err)
			require.Equal(t, previous.ID, preserved.ID)
			require.Equal(t, previous.Content, preserved.Content)
			require.Equal(t, previous.UpdatedAt.UnixNano(), preserved.UpdatedAt.UnixNano())
			require.Equal(t, "previous", yakit.GetKey(db, corePluginEmbedKey))
			require.Equal(t, "{}", yakit.GetKey(db, corePluginItemHashesKey))
			require.Zero(t, count.Load())
		})
	}
}

func TestCorePluginEmbeddedMigrationAndHotSync(t *testing.T) {
	db := newCorePluginSyncDB(t)
	previousDB, previousPath := consts.GetGormProfileDatabase(), consts.GetCurrentProfileDatabasePath()
	previousPlugins := buildInPlugin
	consts.BindProfileDatabase(db, "")
	t.Cleanup(func() {
		consts.BindProfileDatabase(previousDB, previousPath)
		buildInPlugin = previousPlugins
	})
	count := countCorePluginParses(t)
	require.NoError(t, syncCorePluginEmbed(false))
	require.Positive(t, count.Swap(0))
	require.NotEmpty(t, buildInPlugin)
	start := time.Now()
	require.NoError(t, syncCorePluginEmbed(false))
	t.Logf("hot sync: %s", time.Since(start))
	require.Zero(t, count.Swap(0))
}
