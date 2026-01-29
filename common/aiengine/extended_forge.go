package aiengine

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type ExtendedForgeZip struct {
	ZipPath  string
	Password string
}

func loadExtendedForgeFromZip(zipPaths []*ExtendedForgeZip) ([]aicommon.ConfigOption, error) {
	var forges []*schema.AIForge
	var tools []*schema.AIYakTool
	for _, zip := range zipPaths {
		if exist, _ := utils.PathExists(zip.ZipPath); !exist {
			log.Errorf("zip path not exists: %s", zip.ZipPath)
			continue
		}
		archiveInfo, err := aiforge.LoadAIForgesFromZip(zip.ZipPath, aiforge.WithImportPassword(zip.Password))
		if err != nil {
			return nil, err
		}
		forges = append(forges, archiveInfo.AIForges...)
		tools = append(tools, archiveInfo.AIYakTools...)
	}

	log.Infof("loaded %d forges and %d tools", len(forges), len(tools))
	aitools := yakscripttools.ConvertTools(tools)
	return []aicommon.ConfigOption{
		aicommon.WithForges(forges...),
		aicommon.WithTools(aitools...),
	}, nil
}
