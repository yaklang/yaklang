package coreplugin

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/resources_monitor"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/yaklang/yaklang/common/log"
)

type PlugInfo struct {
	PlugName    string
	BinDataPath string
}

var basePluginFS resources_monitor.ResourceMonitor

var initDB = sync.Once{}
var LoadCorePluginHooks = []func(name string, source string) string{}

// InitDBForTest initializes yakit database for tests.
// It is safe to call multiple times; underlying init only runs once.
func InitDBForTest() {
	initDB.Do(func() {
		yakit.InitialDatabase()
	})
}

func RegisterLoadCorePluginHook(hook func(name string, source string) string) {
	LoadCorePluginHooks = append(LoadCorePluginHooks, hook)
}

func GetCorePluginDataWithHook(name string) []byte {
	codeBytes := GetCorePluginData(name)
	for _, hook := range LoadCorePluginHooks {
		codeBytes = []byte(hook(name, string(codeBytes)))
	}
	return codeBytes
}
func GetCorePluginData(name string) []byte {
	codeBytes, err := basePluginFS.ReadFile(fmt.Sprintf("base-yak-plugin/%v.yak", name))
	if err != nil {
		log.Errorf("%v不是core plugin", name)
		return nil
	}
	return codeBytes
}

func GetAllCorePluginName() []string {
	var corePluginNames []string
	dir, err := basePluginFS.ReadDir("base-yak-plugin")
	if err != nil {
		log.Errorf("读取core plugin目录失败")
		return nil
	}
	for _, file := range dir {
		if !file.IsDir() && path.Ext(file.Name()) == ".yak" {
			corePluginNames = append(corePluginNames, strings.TrimSuffix(file.Name(), ".yak"))
		}
	}
	return corePluginNames
}

// CorePluginInfo contains the essential information of a core plugin for testing purposes
type CorePluginInfo struct {
	ScriptName string
	Type       string
	Content    string
}

// GetAllCorePluginWithType returns all registered core plugins with their type and content
// This is exported for use in external test packages (coreplugin_test)
func GetAllCorePluginWithType() []CorePluginInfo {
	var plugins []CorePluginInfo
	for _, plugin := range buildInPlugin {
		plugins = append(plugins, CorePluginInfo{
			ScriptName: plugin.ScriptName,
			Type:       plugin.Type,
			Content:    plugin.Content,
		})
	}
	return plugins
}

// getBasePlugin returns the basePlugin embed.FS (defined in embed.go or gzip_embed.go)
// This function is implemented in embed.go and gzip_embed.go

func CorePluginHash() (string, error) {
	// Only calculate hash for .yak files to ensure stability
	// This prevents hash changes when non-plugin files (like .gitkeep, .DS_Store) are added/removed
	hash, err := basePluginFS.GetHash()
	if err != nil {
		// Check if error is due to no .yak files found
		if errors.Is(err, filesys.ErrNoFileFound) {
			return "", fmt.Errorf("no .yak file found")
		}
		return "", err
	}
	return hash, nil
}
