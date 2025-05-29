package aiforge

import (
	"embed"
	"encoding/json"
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
			registerBuildInForge("xss")
			registerBuildInForge("sqlinject")
			registerBuildInForge("travelmaster")
			registerBuildInForge("pimatrix")
			registerBuildInForge("netscan")
			registerBuildInForge("recon")
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

func getForgeCode(name string) (string, bool) {
	codeBytes, err := basePlugin.ReadFile(fmt.Sprintf("buildinforge/%v.yak", name))
	if err != nil {
		return "", false
	}
	return string(codeBytes), true
}

func getForgeConfig(name string) (string, *schema.AIForge, bool) {
	forge := &schema.AIForge{}
	p := fmt.Sprintf("buildinforge/%v", name)
	loadDefaultPrompt := func(promptName string) string {
		promptBytes, _ := basePlugin.ReadFile(fmt.Sprintf("%v/%v.txt", p, promptName))
		return string(promptBytes)
	}
	codeContent, _ := basePlugin.ReadFile(fmt.Sprintf("%v/%v.yak", p, name))
	configBytes, err := basePlugin.ReadFile(fmt.Sprintf("%v/forge_cfg.json", p))
	if err != nil {
		// If config file doesn't exist, try to read prompt files directly
		initPrompt := loadDefaultPrompt("init")
		persistentPrompt := loadDefaultPrompt("persistent")
		planPrompt := loadDefaultPrompt("plan")
		resultPrompt := loadDefaultPrompt("result")

		if len(initPrompt) == 0 && len(persistentPrompt) == 0 && len(planPrompt) == 0 && len(resultPrompt) == 0 {
			return "", nil, false
		}
		forge.ForgeName = name
		forge.InitPrompt = string(initPrompt)
		forge.PersistentPrompt = string(persistentPrompt)
		forge.PlanPrompt = string(planPrompt)
		forge.ResultPrompt = string(resultPrompt)
		forge.ForgeContent = string(codeContent)
	} else {
		// 使用结构体解析forge_cfg.json
		var cfg YakForgeBlueprintConfig
		err = json.Unmarshal(configBytes, &cfg)
		if err != nil {
			log.Errorf("parse forge config failed: %v", err)
			return "", nil, false
		}

		if cfg.InitPrompt == "" {
			cfg.InitPrompt = loadDefaultPrompt("init")
		}
		if cfg.PersistentPrompt == "" {
			cfg.PersistentPrompt = loadDefaultPrompt("persistent")
		}
		if cfg.PlanPrompt == "" {
			cfg.PlanPrompt = loadDefaultPrompt("plan")
		}
		if cfg.ResultPrompt == "" {
			cfg.ResultPrompt = loadDefaultPrompt("result")
		}
		if cfg.ForgeContent == "" {
			cfg.ForgeContent = string(codeContent)
		}
		forge.ForgeName = cfg.Name
		forge.ToolKeywords = cfg.ToolKeywords
		forge.Tools = cfg.Tools
		forge.Description = cfg.Description
		forge.InitPrompt = cfg.InitPrompt
		forge.PersistentPrompt = cfg.PersistentPrompt
		forge.PlanPrompt = cfg.PlanPrompt
		forge.ResultPrompt = cfg.ResultPrompt
		forge.Actions = cfg.Actions
		forge.ForgeContent = cfg.ForgeContent
	}
	return string(configBytes), forge, true
}

func registerBuildInForge(name string) {
	var forge *schema.AIForge
	// First try to get forge code
	code, ok := getForgeCode(name)
	if ok {
		forge = &schema.AIForge{
			ForgeName:    name,
			ForgeContent: code,
		}
	} else {
		// Try to get forge config
		_, forge, ok = getForgeConfig(name)
		if !ok {
			log.Errorf("%v不是build-in forge", name)
			return
		}
		forge.ForgeName = name
	}

	err := yakit.CreateOrUpdateAIForgeByName(consts.GetGormProfileDatabase(), forge.ForgeName, forge)
	if err != nil {
		log.Errorf("create or update forge %v failed: %v", name, err)
		return
	}
}
