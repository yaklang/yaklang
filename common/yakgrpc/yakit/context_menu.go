package yakit

import (
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/contextmenu"
)

var contextMenuBindingLock sync.Mutex

func SetContextMenuBinding(db *gorm.DB, binding *schema.ContextMenuBinding) (*schema.ContextMenuBinding, error) {
	if binding == nil {
		return nil, utils.Error("context-menu binding is nil")
	}
	binding.PluginUUID = strings.TrimSpace(binding.PluginUUID)
	binding.ActionID = strings.TrimSpace(binding.ActionID)
	binding.ResultMode = contextmenu.NormalizeResultMode(binding.ResultMode)
	if !contextmenu.IsKnownBindingAction(binding.ActionID) {
		return nil, utils.Errorf("unknown context-menu action: %s", binding.ActionID)
	}
	if !contextmenu.IsValidResultMode(binding.ResultMode) {
		return nil, utils.Errorf("unknown context-menu result mode: %s", binding.ResultMode)
	}

	script, err := GetYakScriptByUUID(db, binding.PluginUUID)
	if err != nil {
		return nil, err
	}
	if !isContextMenuManagedScript(script, binding.ActionID) {
		return nil, utils.Errorf("plugin %s does not provide context-menu action %s", binding.PluginUUID, binding.ActionID)
	}
	if script.IsCorePlugin && !binding.Enabled {
		return nil, utils.Errorf("core context-menu action %s cannot be disabled", binding.PluginUUID)
	}
	scene, ok := contextmenu.SceneForAction(binding.ActionID)
	if !ok {
		return nil, utils.Errorf("unknown context-menu scene for action: %s", binding.ActionID)
	}

	contextMenuBindingLock.Lock()
	defer contextMenuBindingLock.Unlock()

	err = utils.GormTransaction(db, func(tx *gorm.DB) error {
		if binding.Enabled && !script.IsCorePlugin {
			enabledPlugins, err := queryEnabledCustomContextMenuPluginUUIDsByScene(tx, scene)
			if err != nil {
				return err
			}
			if _, alreadyEnabled := enabledPlugins[binding.PluginUUID]; !alreadyEnabled && len(enabledPlugins) >= contextmenu.MaxCustomPluginsPerScene {
				return utils.Errorf(
					"at most %d custom context-menu plugins can be enabled in scene %s",
					contextmenu.MaxCustomPluginsPerScene,
					scene,
				)
			}
		}

		binding.Hash = binding.CalcHash()
		assign := map[string]any{
			"plugin_uuid":    binding.PluginUUID,
			"action_id":      binding.ActionID,
			"enabled":        binding.Enabled,
			"sort":           binding.Sort,
			"shortcut":       binding.Shortcut,
			"result_mode":    binding.ResultMode,
			"ask_before_run": binding.AskBeforeRun,
		}
		if result := tx.Model(&schema.ContextMenuBinding{}).
			Where("hash = ?", binding.Hash).
			Assign(assign).
			FirstOrCreate(binding); result.Error != nil {
			return utils.Errorf("save context-menu binding failed: %s", result.Error)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return GetContextMenuBinding(db, binding.PluginUUID, binding.ActionID)
}

func GetContextMenuBinding(db *gorm.DB, pluginUUID, actionID string) (*schema.ContextMenuBinding, error) {
	var binding schema.ContextMenuBinding
	result := db.Model(&schema.ContextMenuBinding{}).
		Where("hash = ?", utils.CalcSha1(strings.TrimSpace(pluginUUID), strings.TrimSpace(actionID))).
		First(&binding)
	if result.Error != nil {
		return nil, result.Error
	}
	return &binding, nil
}

func QueryContextMenuBindings(db *gorm.DB) ([]*schema.ContextMenuBinding, error) {
	var bindings []*schema.ContextMenuBinding
	result := db.Model(&schema.ContextMenuBinding{}).Find(&bindings)
	if result.Error != nil {
		return nil, result.Error
	}
	sortContextMenuBindings(bindings)
	return bindings, nil
}

// QueryEffectiveContextMenuBindings returns explicit user bindings plus the
// legacy CODEC actions that the pre-refactor frontend displayed by default.
// An explicit binding, including Enabled=false, always overrides the default.
func QueryEffectiveContextMenuBindings(db *gorm.DB, scene string) ([]*schema.ContextMenuBinding, error) {
	explicit, err := QueryContextMenuBindings(db)
	if err != nil {
		return nil, err
	}
	defaults, err := queryDefaultLegacyContextMenuBindings(db, scene, explicit)
	if err != nil {
		return nil, err
	}

	bindingsByKey := make(map[string]*schema.ContextMenuBinding, len(defaults)+len(explicit))
	for _, binding := range defaults {
		bindingsByKey[contextMenuBindingKey(binding.PluginUUID, binding.ActionID)] = binding
	}
	for _, binding := range explicit {
		bindingsByKey[contextMenuBindingKey(binding.PluginUUID, binding.ActionID)] = binding
	}

	bindings := make([]*schema.ContextMenuBinding, 0, len(bindingsByKey))
	for _, binding := range bindingsByKey {
		bindingScene, ok := contextmenu.SceneForAction(binding.ActionID)
		if !ok || (scene != "" && bindingScene != scene) {
			continue
		}
		bindings = append(bindings, binding)
	}
	sortContextMenuBindings(bindings)
	return bindings, nil
}

func queryDefaultLegacyContextMenuBindings(
	db *gorm.DB,
	scene string,
	explicit []*schema.ContextMenuBinding,
) ([]*schema.ContextMenuBinding, error) {
	if scene != "" && !contextmenu.IsKnownScene(scene) {
		return nil, utils.Errorf("unknown context-menu scene: %s", scene)
	}
	explicitByKey := make(map[string]struct{}, len(explicit))
	explicitUUIDs := make([]string, 0, len(explicit))
	explicitUUIDSet := make(map[string]struct{}, len(explicit))
	for _, binding := range explicit {
		explicitByKey[contextMenuBindingKey(binding.PluginUUID, binding.ActionID)] = struct{}{}
		if !binding.Enabled {
			continue
		}
		if _, exists := explicitUUIDSet[binding.PluginUUID]; !exists {
			explicitUUIDSet[binding.PluginUUID] = struct{}{}
			explicitUUIDs = append(explicitUUIDs, binding.PluginUUID)
		}
	}

	selectedByScene := make(map[string]map[string]struct{})
	if len(explicitUUIDs) > 0 {
		var explicitScripts []*schema.YakScript
		result := db.Model(&schema.YakScript{}).Where("uuid IN (?)", explicitUUIDs).Find(&explicitScripts)
		if result.Error != nil {
			return nil, result.Error
		}
		explicitScriptByUUID := make(map[string]*schema.YakScript, len(explicitScripts))
		for _, script := range explicitScripts {
			explicitScriptByUUID[script.Uuid] = script
		}
		for _, binding := range explicit {
			if !binding.Enabled {
				continue
			}
			bindingScene, ok := contextmenu.SceneForAction(binding.ActionID)
			if !ok || (scene != "" && bindingScene != scene) {
				continue
			}
			script := explicitScriptByUUID[binding.PluginUUID]
			if script == nil || script.IsCorePlugin || !isContextMenuManagedScript(script, binding.ActionID) {
				continue
			}
			if selectedByScene[bindingScene] == nil {
				selectedByScene[bindingScene] = make(map[string]struct{})
			}
			selectedByScene[bindingScene][binding.PluginUUID] = struct{}{}
		}
	}

	actionIDs := []string{
		contextmenu.ActionLegacyHistorySingle,
		contextmenu.ActionLegacyHistoryMulti,
		contextmenu.ActionLegacyPacketContext,
		contextmenu.ActionLegacyPacketMutate,
	}
	bindings := make([]*schema.ContextMenuBinding, 0, len(actionIDs)*contextmenu.LegacyDefaultPluginLimit)
	for _, actionID := range actionIDs {
		actionScene, _ := contextmenu.SceneForAction(actionID)
		if scene != "" && actionScene != scene {
			continue
		}
		tag, _ := contextmenu.LegacyTagForAction(actionID)
		var scripts []*schema.YakScript
		result := db.Model(&schema.YakScript{}).
			Where("ignored = ?", false).
			Where("type = ?", contextmenu.LegacyPluginType).
			Where("tags LIKE ?", "%"+tag+"%").
			Order("updated_at desc").
			Limit(contextmenu.LegacyDefaultPluginLimit).
			Find(&scripts)
		if result.Error != nil {
			return nil, result.Error
		}
		if selectedByScene[actionScene] == nil {
			selectedByScene[actionScene] = make(map[string]struct{})
		}
		selected := selectedByScene[actionScene]
		for index, script := range scripts {
			if err := EnsureContextMenuScriptUUID(db, script); err != nil {
				return nil, err
			}
			if _, configured := explicitByKey[contextMenuBindingKey(script.Uuid, actionID)]; configured {
				continue
			}
			if !script.IsCorePlugin {
				if _, exists := selected[script.Uuid]; !exists {
					if len(selected) >= contextmenu.MaxCustomPluginsPerScene {
						continue
					}
					selected[script.Uuid] = struct{}{}
				}
			}
			bindings = append(bindings, &schema.ContextMenuBinding{
				PluginUUID: script.Uuid,
				ActionID:   actionID,
				Enabled:    true,
				Sort:       int64(index + 1),
				ResultMode: contextmenu.ResultModeAuto,
			})
		}
	}
	return bindings, nil
}

func EnsureContextMenuScriptUUID(db *gorm.DB, script *schema.YakScript) error {
	if script == nil || script.Uuid != "" {
		return nil
	}
	generated := uuid.NewString()
	result := db.Model(&schema.YakScript{}).
		Where("id = ? AND (uuid = '' OR uuid IS NULL)", script.ID).
		UpdateColumn("uuid", generated)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		script.Uuid = generated
		return nil
	}
	refreshed, err := GetYakScript(db, int64(script.ID))
	if err != nil {
		return err
	}
	script.Uuid = refreshed.Uuid
	return nil
}

func contextMenuBindingKey(pluginUUID, actionID string) string {
	return strings.TrimSpace(pluginUUID) + "\x00" + strings.TrimSpace(actionID)
}

func sortContextMenuBindings(bindings []*schema.ContextMenuBinding) {
	sort.SliceStable(bindings, func(i, j int) bool {
		if bindings[i].Sort != bindings[j].Sort {
			return bindings[i].Sort < bindings[j].Sort
		}
		if bindings[i].PluginUUID != bindings[j].PluginUUID {
			return bindings[i].PluginUUID < bindings[j].PluginUUID
		}
		return bindings[i].ActionID < bindings[j].ActionID
	})
}

func countEnabledCustomContextMenuPluginsByScene(db *gorm.DB, scene string) (int, error) {
	plugins, err := queryEnabledCustomContextMenuPluginUUIDsByScene(db, scene)
	if err != nil {
		return 0, err
	}
	return len(plugins), nil
}

func queryEnabledCustomContextMenuPluginUUIDsByScene(db *gorm.DB, scene string) (map[string]struct{}, error) {
	if !contextmenu.IsKnownScene(scene) {
		return nil, utils.Errorf("unknown context-menu scene: %s", scene)
	}
	bindings, err := QueryEffectiveContextMenuBindings(db, scene)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		if !binding.Enabled {
			continue
		}
		bindingScene, ok := contextmenu.SceneForAction(binding.ActionID)
		if !ok || bindingScene != scene {
			continue
		}
		if _, ok := seen[binding.PluginUUID]; ok {
			continue
		}
		script, err := GetYakScriptByUUID(db, binding.PluginUUID)
		if err != nil {
			continue
		}
		if isContextMenuManagedScript(script, binding.ActionID) && !script.IsCorePlugin {
			seen[binding.PluginUUID] = struct{}{}
		}
	}
	return seen, nil
}

func isContextMenuManagedScript(script *schema.YakScript, actionID string) bool {
	if script == nil {
		return false
	}
	switch script.Type {
	case contextmenu.PluginType:
		return contextmenu.IsKnownAction(actionID)
	case contextmenu.LegacyPluginType:
		return contextmenu.LegacyScriptImplements(script.Tags, actionID)
	default:
		return false
	}
}
