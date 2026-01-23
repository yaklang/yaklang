package coreplugin

import (
	"embed"
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/utils/filesys"

	"github.com/yaklang/yaklang/common/log"
)

//go:embed base-yak-plugin
var basePlugin embed.FS

type PlugInfo struct {
	PlugName    string
	BinDataPath string
}

var initDB = sync.Once{}
var LoadCorePluginHooks = []func(name string, source string) string{}

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
	codeBytes, err := basePlugin.ReadFile(fmt.Sprintf("base-yak-plugin/%v.yak", name))
	if err != nil {
		log.Errorf("%v不是core plugin", name)
		return nil
	}
	return codeBytes
}

func GetAllCorePluginName() []string {
	var corePluginNames []string
	dir, err := basePlugin.ReadDir("base-yak-plugin")
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

func CorePluginHash() (string, error) {
	// Only calculate hash for .yak files to ensure stability
	// This prevents hash changes when non-plugin files (like .gitkeep, .DS_Store) are added/removed
	hash, err := filesys.CreateEmbedFSHash(basePlugin, filesys.WithIncludeExts(".yak"))
	if err != nil {
		// Check if error is due to no .yak files found
		if errors.Is(err, filesys.ErrNoFileFound) {
			return "", fmt.Errorf("no .yak file found")
		}
		return "", err
	}
	return hash, nil
}
