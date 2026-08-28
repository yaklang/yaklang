package yakit

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/contextmenu"
)

func newContextMenuTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "profile.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.YakScript{}, &schema.ContextMenuBinding{}).Error)
	return db
}

func createContextMenuTestPlugin(t *testing.T, db *gorm.DB, name string, core bool) *schema.YakScript {
	t.Helper()
	script := &schema.YakScript{
		ScriptName: name,
		Type:       contextmenu.PluginType,
		Content: `
handleOneHTTPFlow = func(ctx, flow) {}
handleMultiHTTPFlows = func(ctx, flows) {}
handleHTTPPacket = func(ctx, request, response) {}
`,
		Uuid:         uuid.NewString(),
		IsCorePlugin: core,
	}
	require.NoError(t, db.Create(script).Error)
	return script
}

func createLegacyContextMenuTestPlugin(t *testing.T, db *gorm.DB, name, tags string, core bool) *schema.YakScript {
	t.Helper()
	script := &schema.YakScript{
		ScriptName:   name,
		Type:         contextmenu.LegacyPluginType,
		Content:      `handle = func(input) { return input }`,
		Tags:         tags,
		Uuid:         uuid.NewString(),
		IsCorePlugin: core,
	}
	require.NoError(t, db.Create(script).Error)
	return script
}

func TestSetContextMenuBindingCountsDistinctCustomPluginsPerScene(t *testing.T) {
	db := newContextMenuTestDB(t)
	first := createContextMenuTestPlugin(t, db, "plugin-0", false)

	for i := 0; i < contextmenu.MaxCustomPluginsPerScene; i++ {
		plugin := first
		if i > 0 {
			plugin = createContextMenuTestPlugin(t, db, fmt.Sprintf("plugin-%d", i), false)
		}
		_, err := SetContextMenuBinding(db, &schema.ContextMenuBinding{
			PluginUUID: plugin.Uuid,
			ActionID:   contextmenu.ActionHistorySingle,
			Enabled:    true,
		})
		require.NoError(t, err)
	}

	historyCount, err := countEnabledCustomContextMenuPluginsByScene(db, contextmenu.ActionHistorySingle)
	require.NoError(t, err)
	require.Equal(t, contextmenu.MaxCustomPluginsPerScene, historyCount)

	// A full History-single scene does not consume the independent HTTP-packet quota.
	_, err = SetContextMenuBinding(db, &schema.ContextMenuBinding{
		PluginUUID: first.Uuid,
		ActionID:   contextmenu.ActionHTTPPacket,
		Enabled:    true,
	})
	require.NoError(t, err)
	packetCount, err := countEnabledCustomContextMenuPluginsByScene(db, contextmenu.ActionHTTPPacket)
	require.NoError(t, err)
	require.Equal(t, 1, packetCount)

	overLimit := createContextMenuTestPlugin(t, db, "plugin-over-limit", false)
	_, err = SetContextMenuBinding(db, &schema.ContextMenuBinding{
		PluginUUID: overLimit.Uuid,
		ActionID:   contextmenu.ActionHistorySingle,
		Enabled:    true,
	})
	require.ErrorContains(t, err, "at most 15")
	_, err = GetContextMenuBinding(db, overLimit.Uuid, contextmenu.ActionHistorySingle)
	require.Error(t, err, "over-limit binding must be rolled back")

	// The same plugin can still be enabled in another scene.
	_, err = SetContextMenuBinding(db, &schema.ContextMenuBinding{
		PluginUUID: overLimit.Uuid,
		ActionID:   contextmenu.ActionHistoryMulti,
		Enabled:    true,
	})
	require.NoError(t, err)
	multiCount, err := countEnabledCustomContextMenuPluginsByScene(db, contextmenu.ActionHistoryMulti)
	require.NoError(t, err)
	require.Equal(t, 1, multiCount)
}

func TestSetContextMenuBindingCountsLegacyActionsOnceWithinScene(t *testing.T) {
	db := newContextMenuTestDB(t)
	legacy := createLegacyContextMenuTestPlugin(t, db, "legacy-packet", strings.Join([]string{
		contextmenu.LegacyTagPacketContext,
		contextmenu.LegacyTagPacketMutate,
	}, ","), false)

	for _, actionID := range []string{
		contextmenu.ActionLegacyPacketContext,
		contextmenu.ActionLegacyPacketMutate,
	} {
		_, err := SetContextMenuBinding(db, &schema.ContextMenuBinding{
			PluginUUID: legacy.Uuid,
			ActionID:   actionID,
			Enabled:    true,
		})
		require.NoError(t, err)
	}

	count, err := countEnabledCustomContextMenuPluginsByScene(db, contextmenu.ActionHTTPPacket)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestEffectiveContextMenuBindingsUseLegacyFrontendDefaults(t *testing.T) {
	db := newContextMenuTestDB(t)
	plugins := make([]*schema.YakScript, 0, contextmenu.LegacyDefaultPluginLimit+1)
	for i := 0; i < contextmenu.LegacyDefaultPluginLimit+1; i++ {
		plugin := createLegacyContextMenuTestPlugin(
			t,
			db,
			fmt.Sprintf("legacy-history-%02d", i),
			contextmenu.LegacyTagHistorySingle,
			false,
		)
		require.NoError(t, db.Model(plugin).UpdateColumn("updated_at", time.Unix(int64(i+1), 0)).Error)
		plugins = append(plugins, plugin)
	}

	bindings, err := QueryEffectiveContextMenuBindings(db, contextmenu.ActionHistorySingle)
	require.NoError(t, err)
	require.Len(t, bindings, contextmenu.LegacyDefaultPluginLimit)

	byUUID := make(map[string]*schema.ContextMenuBinding, len(bindings))
	for _, binding := range bindings {
		byUUID[binding.PluginUUID] = binding
		require.True(t, binding.Enabled)
		require.Equal(t, contextmenu.ActionLegacyHistorySingle, binding.ActionID)
	}
	require.NotContains(t, byUUID, plugins[0].Uuid, "the oldest plugin should remain outside the old frontend's top ten")
	require.EqualValues(t, 1, byUUID[plugins[len(plugins)-1].Uuid].Sort)
	require.EqualValues(t, contextmenu.LegacyDefaultPluginLimit, byUUID[plugins[1].Uuid].Sort)

	explicit, err := QueryContextMenuBindings(db)
	require.NoError(t, err)
	require.Empty(t, explicit, "compatibility defaults must not create migration records")

	_, err = SetContextMenuBinding(db, &schema.ContextMenuBinding{
		PluginUUID: plugins[len(plugins)-1].Uuid,
		ActionID:   contextmenu.ActionLegacyHistorySingle,
		Enabled:    false,
	})
	require.NoError(t, err)
	bindings, err = QueryEffectiveContextMenuBindings(db, contextmenu.ActionHistorySingle)
	require.NoError(t, err)
	require.Len(t, bindings, contextmenu.LegacyDefaultPluginLimit)
	require.False(t, findContextMenuTestBinding(t, bindings, plugins[len(plugins)-1].Uuid).Enabled)

	count, err := countEnabledCustomContextMenuPluginsByScene(db, contextmenu.ActionHistorySingle)
	require.NoError(t, err)
	require.Equal(t, contextmenu.LegacyDefaultPluginLimit-1, count)
}

func TestLegacyFrontendDefaultsCountTowardSceneLimit(t *testing.T) {
	db := newContextMenuTestDB(t)
	for i := 0; i < contextmenu.LegacyDefaultPluginLimit; i++ {
		createLegacyContextMenuTestPlugin(
			t,
			db,
			fmt.Sprintf("legacy-default-%02d", i),
			contextmenu.LegacyTagHistorySingle,
			false,
		)
	}

	remaining := contextmenu.MaxCustomPluginsPerScene - contextmenu.LegacyDefaultPluginLimit
	for i := 0; i < remaining; i++ {
		plugin := createContextMenuTestPlugin(t, db, fmt.Sprintf("native-context-menu-%02d", i), false)
		_, err := SetContextMenuBinding(db, &schema.ContextMenuBinding{
			PluginUUID: plugin.Uuid,
			ActionID:   contextmenu.ActionHistorySingle,
			Enabled:    true,
		})
		require.NoError(t, err)
	}

	overLimit := createContextMenuTestPlugin(t, db, "native-context-menu-over-limit", false)
	_, err := SetContextMenuBinding(db, &schema.ContextMenuBinding{
		PluginUUID: overLimit.Uuid,
		ActionID:   contextmenu.ActionHistorySingle,
		Enabled:    true,
	})
	require.ErrorContains(t, err, "at most 15")
}

func TestLegacyFrontendDefaultsRespectExistingSceneCapacity(t *testing.T) {
	db := newContextMenuTestDB(t)
	for i := 0; i < 10; i++ {
		plugin := createContextMenuTestPlugin(t, db, fmt.Sprintf("existing-native-%02d", i), false)
		_, err := SetContextMenuBinding(db, &schema.ContextMenuBinding{
			PluginUUID: plugin.Uuid,
			ActionID:   contextmenu.ActionHistorySingle,
			Enabled:    true,
		})
		require.NoError(t, err)
	}
	for i := 0; i < contextmenu.LegacyDefaultPluginLimit; i++ {
		createLegacyContextMenuTestPlugin(
			t,
			db,
			fmt.Sprintf("later-legacy-default-%02d", i),
			contextmenu.LegacyTagHistorySingle,
			false,
		)
	}

	bindings, err := QueryEffectiveContextMenuBindings(db, contextmenu.ActionHistorySingle)
	require.NoError(t, err)
	enabledUUIDs := make(map[string]struct{})
	legacyCount := 0
	for _, binding := range bindings {
		if !binding.Enabled {
			continue
		}
		enabledUUIDs[binding.PluginUUID] = struct{}{}
		if binding.ActionID == contextmenu.ActionLegacyHistorySingle {
			legacyCount++
		}
	}
	require.Len(t, enabledUUIDs, contextmenu.MaxCustomPluginsPerScene)
	require.Equal(t, contextmenu.MaxCustomPluginsPerScene-10, legacyCount)
}

func findContextMenuTestBinding(t *testing.T, bindings []*schema.ContextMenuBinding, pluginUUID string) *schema.ContextMenuBinding {
	t.Helper()
	for _, binding := range bindings {
		if binding.PluginUUID == pluginUUID {
			return binding
		}
	}
	require.FailNow(t, "context-menu binding not found", pluginUUID)
	return nil
}

func TestCoreContextMenuBindingCannotBeDisabledAndDoesNotConsumeLimit(t *testing.T) {
	db := newContextMenuTestDB(t)
	core := createContextMenuTestPlugin(t, db, "core-plugin", true)

	_, err := SetContextMenuBinding(db, &schema.ContextMenuBinding{
		PluginUUID: core.Uuid,
		ActionID:   contextmenu.ActionHistorySingle,
		Enabled:    false,
	})
	require.ErrorContains(t, err, "cannot be disabled")

	_, err = SetContextMenuBinding(db, &schema.ContextMenuBinding{
		PluginUUID: core.Uuid,
		ActionID:   contextmenu.ActionHistorySingle,
		Enabled:    true,
	})
	require.NoError(t, err)
	count, err := countEnabledCustomContextMenuPluginsByScene(db, contextmenu.ActionHistorySingle)
	require.NoError(t, err)
	require.Zero(t, count)

	err = DeleteYakScriptByName(db, core.ScriptName)
	require.ErrorContains(t, err, "cannot be deleted")
	require.ErrorContains(t, DeleteYakScriptByIDs(db, int64(core.ID)), "cannot be deleted")
	_, err = GetYakScriptByName(db, core.ScriptName)
	require.NoError(t, err)

	custom := createContextMenuTestPlugin(t, db, "deletable-custom-plugin", false)
	require.NoError(t, DeleteYakScriptByName(db, custom.ScriptName))
	_, err = GetYakScriptByName(db, custom.ScriptName)
	require.Error(t, err)
}

func TestCoreContextMenuIdentitySurvivesGenericNameUpdate(t *testing.T) {
	db := newContextMenuTestDB(t)
	core := createContextMenuTestPlugin(t, db, "core-update-protection", true)

	require.NoError(t, CreateOrUpdateYakScriptByName(db, core.ScriptName, &schema.YakScript{
		ScriptName:   core.ScriptName,
		Type:         schema.SCRIPT_TYPE_YAK,
		Content:      `handleHTTPPacket = func(ctx, request, response) { return nil }`,
		Uuid:         uuid.NewString(),
		IsCorePlugin: false,
	}))

	updated, err := GetYakScriptByName(db, core.ScriptName)
	require.NoError(t, err)
	require.Equal(t, schema.SCRIPT_TYPE_CONTEXT_MENU, updated.Type)
	require.True(t, updated.IsCorePlugin)
	require.Equal(t, core.Uuid, updated.Uuid)
}

func TestCoreLegacyContextMenuCapabilityCannotBeDisabledOrDeleted(t *testing.T) {
	db := newContextMenuTestDB(t)
	core := createLegacyContextMenuTestPlugin(
		t,
		db,
		"core-legacy-context-menu",
		contextmenu.LegacyTagPacketContext,
		true,
	)

	_, err := SetContextMenuBinding(db, &schema.ContextMenuBinding{
		PluginUUID: core.Uuid,
		ActionID:   contextmenu.ActionLegacyPacketContext,
		Enabled:    false,
	})
	require.ErrorContains(t, err, "cannot be disabled")
	require.ErrorContains(t, DeleteYakScriptByName(db, core.ScriptName), "cannot be deleted")
	require.ErrorContains(t, DeleteYakScriptByIDs(db, int64(core.ID)), "cannot be deleted")

	require.NoError(t, CreateOrUpdateYakScriptByName(db, core.ScriptName, &schema.YakScript{
		ScriptName: core.ScriptName,
		Type:       schema.SCRIPT_TYPE_YAK,
		Content:    `a = 1`,
		Tags:       "updated-tag",
	}))
	updated, err := GetYakScriptByName(db, core.ScriptName)
	require.NoError(t, err)
	require.Equal(t, schema.SCRIPT_TYPE_CODEC, updated.Type)
	require.True(t, updated.IsCorePlugin)
	require.Equal(t, core.Uuid, updated.Uuid)
	require.True(t, contextmenu.HasTag(updated.Tags, contextmenu.LegacyTagPacketContext))
}
