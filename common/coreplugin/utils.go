package coreplugin

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"

	"github.com/yaklang/yaklang/common/log"
)

type PlugInfo struct {
	PlugName    string
	BinDataPath string
}

// FileSystemWithHash 是一个带有 GetHash 方法的文件系统接口
type FileSystemWithHash interface {
	fi.FileSystem
	GetHash() (string, error)
}

var basePluginFS interface {
	GetHash() (string, error)
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
	codeBytes, err := getBasePlugin().ReadFile(fmt.Sprintf("base-yak-plugin/%v.yak", name))
	if err != nil {
		log.Errorf("%v不是core plugin", name)
		return nil
	}
	return codeBytes
}

func GetAllCorePluginName() []string {
	var corePluginNames []string
	dir, err := getBasePlugin().ReadDir("base-yak-plugin")
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
