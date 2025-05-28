package aiforge

import (
	"embed"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools/metadata"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"os"
	"strings"
)

//go:embed buildinforge/**
var basePlugin embed.FS

func init() {
	yakit.RegisterPostInitDatabaseFunction(func() error {
		if consts.IsDevMode() {
			if !consts.IsDevMode() {
				const key = "6ef3c850244a2b26ed0b163d1fda9600"
				if yakit.Get(key) == consts.ExistedBuildInForgeEmbedFSHash {
					return nil
				}
				log.Debug("start to load core plugin")
				defer func() {
					hash, _ := BuildInForgeHash()
					yakit.Set(key, hash)
				}()
			}
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

func BuildInForgeHash() (string, error) {
	return filesys.CreateEmbedFSHash(basePlugin)
}

func getForgeYakScript(name string) (*schema.AIForge, bool) {
	if !strings.HasSuffix(name, ".yak") {
		name = name + ".yak"
	}
	codeBytes, err := basePlugin.ReadFile(fmt.Sprintf("buildinforge/%v", name))
	if err != nil {
		return nil, false
	}

	scriptMetadata, err := metadata.ParseYakScriptMetadata(name, string(codeBytes))
	if err != nil {
		return nil, false
	}
	return &schema.AIForge{
		ForgeName:    scriptMetadata.Name,
		Description:  scriptMetadata.Description,
		Tags:         strings.Join(scriptMetadata.Keywords, ","),
		ForgeContent: string(codeBytes),
	}, true
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

func getBuildInForgeFromFS(name string) (*schema.AIForge, error) {
	var forge *schema.AIForge
	// First try to get forge code
	forge, ok := getForgeYakScript(name)
	if !ok {
		// Try to get forge config
		_, forge, ok = getForgeConfig(name)
		if !ok {
			return nil, fmt.Errorf(`failed to find forge config for "%v"`, name)
		}
		forge.ForgeName = name
	}
	return forge, nil
}

func registerBuildInForge(name string) {
	forge, err := getBuildInForgeFromFS(name)
	if err != nil {
		log.Error(err)
		return
	}
	err = yakit.CreateOrUpdateAIForgeByName(consts.GetGormProfileDatabase(), forge.ForgeName, forge)
	if err != nil {
		log.Errorf("create or update forge %v failed: %v", name, err)
		return
	}
}

func UpdateForgesMetaData(inputDir, outputDir string, concurrency int, forceUpdate bool) error {
	currentDir := inputDir
	if outputDir != "" {
		err := os.CopyFS(outputDir, os.DirFS(inputDir))
		if err != nil {
			return err
		}
		currentDir = outputDir
	}

	fileInfos, err := utils.ReadDir(currentDir)
	if err != nil {
		return err
	}

	log.Infof("Found %d Yak script files to process with concurrency %d", len(fileInfos), concurrency)

	errorChan := make(chan error, len(fileInfos))
	var swg = utils.NewSizedWaitGroup(concurrency)
	for _, fileInfo := range fileInfos {
		if fileInfo.IsDir || strings.HasSuffix(fileInfo.Name, ".yak") {
			swg.Add(1)
			go func() {
				defer swg.Done()
				err := updateForgeMetaData(fileInfo, forceUpdate)
				if err != nil {
					errorChan <- fmt.Errorf("error processing %s: %v", fileInfo.Path, err)
				}
			}()
		} else {
			log.Warnf("Skipping non-forge file: %s", fileInfo.Path)
			continue
		}
	}

	swg.Wait()
	close(errorChan)

	var errors []error
	for err := range errorChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		log.Errorf("Encountered %d errors during processing", len(errors))
		for _, err := range errors {
			log.Error(err)
		}
		return fmt.Errorf("encountered %d errors during processing", len(errors))
	}

	return nil
}

func updateForgeMetaData(fileinfo *utils.FileInfo, forceUpdate bool) error {
	if fileinfo.IsDir {
		simpleCfg, forge, ok := getForgeConfig(fileinfo.Path)
		if !ok {
			return fmt.Errorf(`failed to find forge config for "%v"`, fileinfo.Path)
		}
		if forceUpdate || forge.Tags == "" || forge.Description == "" {
			completeForgeBytes, err2 := json.Marshal(forge)
			if err2 != nil {
				return err2
			}
			metaData, err := metadata.GenerateForgeMetadata(string(completeForgeBytes))
			if err != nil {
				return err
			}

			// update forge content
			var cfg YakForgeBlueprintConfig
			err = json.Unmarshal([]byte(simpleCfg), &cfg)
			cfg.Tags = strings.Join(metaData.Keywords, ",")
			cfg.Description = metaData.Description

			newCfg, err := json.Marshal(cfg)
			if err != nil {
				return fmt.Errorf("failed to marshal updated forge config: %v", err)
			}

			err = os.WriteFile(fileinfo.Path+"/forge_cfg.json", newCfg, 0o755)
			if err != nil {
				return fmt.Errorf("failed to write updated forge config: %v", err)
			}
		}
		return nil
	} else {
		if !strings.HasSuffix(fileinfo.Name, ".yak") {
			return nil
		}

		forge, ok := getForgeYakScript(fileinfo.Name)
		if !ok {
			return fmt.Errorf(`failed to get yak script forge for "%v"`, fileinfo.Path)
		}
		if forceUpdate || forge.Tags == "" || forge.Description == "" {
			metaData, err := metadata.GenerateForgeMetadata(forge.ForgeContent)
			if err != nil {
				return fmt.Errorf("failed to generate metadata for %s: %v", fileinfo.Path, err)
			}
			newYakContent := metadata.GenerateScriptWithMetadata(forge.ForgeContent, metaData.Description, metaData.Keywords)
			err = os.WriteFile(fileinfo.Path, []byte(newYakContent), 0o755)
			if err != nil {
				return fmt.Errorf("failed to write updated yak script: %v", err)
			}
		}
		return nil
	}
}
