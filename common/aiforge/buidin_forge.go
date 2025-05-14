package aiforge

import (
	"embed"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

//go:embed **
var basePlugin embed.FS

func init() {
	yakit.RegisterPostInitDatabaseFunction(func() error {
		if consts.IsDevMode() {
			//const key = "cd336beba498c97738c275f6771efca3"
			//if yakit.Get(key) == consts.ExistedCorePluginEmbedFSHash {
			//	return nil
			//}
			//log.Debug("start to load core plugin")
			//defer func() {
			//	hash, _ := buildInForgeHash()
			//	yakit.Set(key, hash)
			//}()
			registerBuildInForge("fragment_summarizer")
			registerBuildInForge("long_text_summarizer")
		}
		return nil
	})
}

func buildInForgeHash() (string, error) {
	return filesys.CreateEmbedFSHash(basePlugin)
}

func getBuildInForge(name string) []byte {
	codeBytes, err := basePlugin.ReadFile(fmt.Sprintf("buildinforge/%v.yak", name))
	if err != nil {
		log.Errorf("%v不是build-in forge", name)
		return nil
	}
	return codeBytes
}

func registerBuildInForge(name string) {
	code := string(getBuildInForge(name))
	if len(code) <= 0 {
		return
	}

	err := yakit.CreateOrUpdateAIForge(consts.GetGormProfileDatabase(), name, &schema.AIForge{
		ForgeName:    name,
		ForgeContent: code,
	})
	if err != nil {
		log.Errorf("create or update forge %v failed: %v", name, err)
		return
	}
}
