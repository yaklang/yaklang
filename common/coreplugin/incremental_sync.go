package coreplugin

import (
	"fmt"
	"sync"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/information"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"golang.org/x/sync/errgroup"
)

const corePluginEmbedKey = "cd336beba498c97738c275f6771efca3"
const corePluginItemHashesKey = "core-plugin-item-hashes"

// Bump for changes to metadata in Go, parameter generation or SSA extraction.
const corePluginExtractorVersion = "1"

type builtinCorePlugin struct {
	*schema.YakScript
	generateParam bool
}

var corePluginSyncMu sync.Mutex

var parseCorePlugin = static_analyzer.SSAParse

var BlackListCorePlugin = []string{"启发式SQL注入检测"}

func ClearBlackListPlugin(blackList []string) {
	if err := yakit.DeleteYakScriptByNames(consts.GetGormProfileDatabase().Where("is_core_plugin = ?", true), blackList); err != nil {
		log.Errorf("delete black list plugin failed: %s", err)
	}
}

func corePluginResourceHash() (string, error) {
	hash := consts.ExistedCorePluginEmbedFSHash
	if consts.IsDevMode() {
		var err error
		hash, err = CorePluginHash()
		if err != nil {
			return "", err
		}
	}
	return hash + ":" + corePluginExtractorVersion, nil
}

func syncCorePluginEmbed(force bool) error {
	corePluginSyncMu.Lock()
	defer corePluginSyncMu.Unlock()
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return fmt.Errorf("core plugin profile database is not initialized")
	}
	hash, err := corePluginResourceHash()
	if err != nil {
		return err
	}
	if !force && yakit.GetKey(db, corePluginEmbedKey) == hash {
		return nil
	}
	plugins, err := loadCorePluginDefinitions()
	if err != nil {
		return err
	}
	if err := syncCorePlugins(db, plugins, hash, true); err != nil {
		return err
	}
	buildInPlugin = make(map[string]*schema.YakScript, len(plugins))
	for name, plugin := range plugins {
		buildInPlugin[name] = plugin.YakScript
	}
	return nil
}

// ForceSyncCorePlugin bypasses the package gate, retaining per-item checks.
func ForceSyncCorePlugin() error { return syncCorePluginEmbed(true) }

func init() {
	yakit.RegisterPostInitDatabaseFunction(func() error {
		return syncCorePluginEmbed(false)
	}, "sync-core-plugin-for-yakit")
}

func corePluginItemHash(plugin *schema.YakScript, generateParam bool) string {
	return utils.CalcSha1(corePluginExtractorVersion, plugin.ScriptName, plugin.Content,
		plugin.Type, plugin.Help, plugin.Author, plugin.Tags, plugin.OnlineContributors,
		plugin.HeadImg, plugin.Ignored, plugin.EnableForAI, plugin.AIDesc, plugin.AIKeywords,
		plugin.AIUsage, generateParam)
}

// Parse outside the write transaction. Two workers own separate, uncached SSA
// programs; persistence and both sync markers are committed by one writer.
func syncCorePlugins(db *gorm.DB, plugins map[string]*builtinCorePlugin, resourceHash string, prune bool) error {
	start := time.Now()
	if db == nil {
		return fmt.Errorf("core plugin profile database is not initialized")
	}
	hashes := make(map[string]string)
	if raw := yakit.GetKey(db, corePluginItemHashesKey); raw != "" {
		parsed := utils.MustUnmarshalJson[map[string]string]([]byte(raw))
		if parsed == nil {
			return fmt.Errorf("read core plugin item hashes: invalid JSON string map")
		}
		if *parsed != nil {
			hashes = *parsed
		}
	}
	names := make([]string, 0, len(plugins))
	for name := range plugins {
		names = append(names, name)
	}
	var stored []schema.YakScript
	if err := db.Where("is_core_plugin = ? OR script_name IN (?)", true, names).Find(&stored).Error; err != nil {
		return err
	}
	existing := make(map[string]schema.YakScript, len(stored))
	for _, plugin := range stored {
		existing[plugin.ScriptName] = plugin
	}
	changed := make([]*builtinCorePlugin, 0)
	for name, definition := range plugins {
		old, found := existing[name]
		delete(existing, name)
		if found && !old.IsCorePlugin {
			continue
		}
		plugin := *definition.YakScript
		if found {
			plugin.Model = old.Model
			if old.Uuid != "" {
				plugin.Uuid = old.Uuid
			}
			yakit.PreserveCoreContextMenuFields(&plugin, &old)
		}
		hash := corePluginItemHash(&plugin, definition.generateParam)
		if found && hashes[name] == hash && corePluginItemHash(&old, definition.generateParam) == hash {
			continue
		}
		hashes[name] = hash
		changed = append(changed, &builtinCorePlugin{YakScript: &plugin, generateParam: definition.generateParam})
	}
	group := new(errgroup.Group)
	group.SetLimit(2)
	for _, plugin := range changed {
		group.Go(func() error {
			if !plugin.generateParam {
				return nil
			}
			prog, err := parseCorePlugin(plugin.Content, plugin.Type, ssaapi.WithEnableCache(false))
			if err != nil {
				return fmt.Errorf("parse core plugin %q: %w", plugin.ScriptName, err)
			}
			plugin.Params, plugin.PluginEnvKey, err = information.GenerateParameterFromProgram(prog)
			return err
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}
	retired := make([]uint, 0)
	if prune {
		for name, plugin := range existing {
			if plugin.IsCorePlugin {
				retired = append(retired, plugin.ID)
			}
			delete(hashes, name)
		}
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		if len(retired) > 0 {
			if err := tx.Where("id IN (?) AND is_core_plugin = ?", retired, true).Unscoped().Delete(&schema.YakScript{}).Error; err != nil {
				return err
			}
		}
		for _, plugin := range changed {
			var err error
			if plugin.ID == 0 {
				err = tx.Create(plugin.YakScript).Error
			} else {
				if err := plugin.BeforeSave(); err != nil {
					return err
				}
				// Only builtin-owned outputs change; retain local selectors, history,
				// module settings and other user fields on the existing row.
				updates := map[string]interface{}{
					"type": plugin.Type, "content": plugin.Content, "params": plugin.Params,
					"help": plugin.Help, "tags": plugin.Tags, "ignored": plugin.Ignored,
					"author": plugin.Author, "head_img": plugin.HeadImg, "uuid": plugin.Uuid,
					"is_core_plugin": true, "is_batch_script": plugin.IsBatchScript,
					"online_contributors": plugin.OnlineContributors, "online_official": plugin.OnlineOfficial,
					"plugin_env_key": plugin.PluginEnvKey, "enable_for_ai": plugin.EnableForAI,
					"ai_desc": plugin.AIDesc, "ai_keywords": plugin.AIKeywords, "ai_usage": plugin.AIUsage,
				}
				err = tx.Model(plugin.YakScript).Where("is_core_plugin = ?", true).Updates(updates).Error
			}
			if err != nil {
				return fmt.Errorf("save core plugin %q: %w", plugin.ScriptName, err)
			}
		}
		if err := yakit.SetKey(tx, corePluginItemHashesKey, hashes); err != nil {
			return err
		}
		if resourceHash != "" {
			return yakit.SetKey(tx, corePluginEmbedKey, resourceHash)
		}
		return nil
	})
	if err == nil {
		log.Infof("core-plugin sync: total=%d changed=%d deleted=%d elapsed=%s", len(plugins), len(changed), len(retired), time.Since(start))
	}
	return err
}

// only use for test
func OverWriteCorePluginToLocal() {
	if err := ForceSyncCorePlugin(); err != nil {
		log.Error(err)
	}
}

func OverWriteYakPlugin(name string, scriptData *schema.YakScript, enableGenerateParam bool) {
	corePluginSyncMu.Lock()
	defer corePluginSyncMu.Unlock()
	if scriptData == nil {
		return
	}
	plugin := *scriptData
	code := GetCorePluginData(name)
	if code == nil {
		return
	}
	plugin.ScriptName, plugin.Content, plugin.IsCorePlugin = name, string(code), true
	if err := syncCorePlugins(consts.GetGormProfileDatabase(), map[string]*builtinCorePlugin{
		name: {YakScript: &plugin, generateParam: enableGenerateParam},
	}, "", false); err != nil {
		log.Error(err)
	}
}
