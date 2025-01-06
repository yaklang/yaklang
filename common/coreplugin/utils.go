package coreplugin

import (
	"embed"
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
	return filesys.CreateEmbedFSHash(basePlugin)
}
