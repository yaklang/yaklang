package yakit

import (
	"sort"
	"strings"
	"sync"

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

		count, err := countEnabledCustomContextMenuPluginsByScene(tx, scene)
		if err != nil {
			return err
		}
		if count > contextmenu.MaxCustomPluginsPerScene {
			return utils.Errorf(
				"at most %d custom context-menu plugins can be enabled in scene %s",
				contextmenu.MaxCustomPluginsPerScene,
				scene,
			)
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
	sort.SliceStable(bindings, func(i, j int) bool {
		if bindings[i].Sort != bindings[j].Sort {
			return bindings[i].Sort < bindings[j].Sort
		}
		if bindings[i].PluginUUID != bindings[j].PluginUUID {
			return bindings[i].PluginUUID < bindings[j].PluginUUID
		}
		return bindings[i].ActionID < bindings[j].ActionID
	})
	return bindings, nil
}

func countEnabledCustomContextMenuPluginsByScene(db *gorm.DB, scene string) (int, error) {
	if !contextmenu.IsKnownScene(scene) {
		return 0, utils.Errorf("unknown context-menu scene: %s", scene)
	}
	var bindings []*schema.ContextMenuBinding
	if result := db.Model(&schema.ContextMenuBinding{}).Where("enabled = ?", true).Find(&bindings); result.Error != nil {
		return 0, result.Error
	}

	seen := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
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
	return len(seen), nil
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
