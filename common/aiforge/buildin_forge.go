package aiforge

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools/metadata"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/information"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//go:embed buildinforge/**
var buildInForge embed.FS

var generateMetadataPrompt = `
# AI forge 元数据生成器

你是一个专门的AI模型，负责为Yak的ai forge智能体生成准确的描述和关键词。你需要理解forge的核心功能和用途。

## 指令:
1. 分析提供的forge信息，理解这个forge的功能和目的，这个信息可能是一段yaklang代码或者json配置文件
2. **重点**：完全忽略代码中的注释内容，仅基于代码的实际功能生成描述
3. 生成一个简洁但全面的forge描述，说明forge能做什么，解决什么问题
4. 生成能够准确表达forge功能的关键词列表(最多10个)
5. 关键词应围绕forge的功能、应用场景和解决的问题
6. 每个关键词应该是单个词或短语(1-3个词)，且为小写中文
7. 如果forge没有强调ai问题，请不要包含ai相关的关键词

## 注意事项：
- 描述应当简明清晰地表达"这个forge能做什么"
- 不要在描述中包含代码注释中的信息
- 不要解释实现细节，只关注forge的实际功能 
`

func GenerateForgeMetadata(forgeContent string) (*GenerateMetadataResult, error) {
	var lfopts []LiteForgeOption
	lfopts = append(lfopts,
		WithLiteForge_Prompt(generateMetadataPrompt))
	lfopts = append(lfopts, WithLiteForge_OutputSchema(
		aitool.WithStringParam("language", aitool.WithParam_Required(true), aitool.WithParam_Description("语言，固定为chinese")),
		aitool.WithStringParam("description", aitool.WithParam_Required(true), aitool.WithParam_Description("forge功能描述")),
		aitool.WithStringArrayParam("keywords", aitool.WithParam_Required(true), aitool.WithParam_Description("关键词数组")),
	))

	lf, err := NewLiteForge("generate_metadata", lfopts...)
	if err != nil {
		return nil, err
	}
	result, err := lf.Execute(context.Background(), []*ypb.ExecParamItem{
		{
			Key:   "query",
			Value: forgeContent,
		},
	})
	if err != nil {
		return nil, err
	}

	if result.Action == nil {
		return nil, fmt.Errorf("extract action failed")
	}

	// Extract the result
	params := result.Action.GetInvokeParams("params")
	language := params.GetString("language")
	description := params.GetString("description")
	keywords := params.GetStringSlice("keywords")

	return &GenerateMetadataResult{
		Language:    language,
		Description: description,
		Keywords:    keywords,
	}, nil
}

type GenerateMetadataResult struct {
	Language    string   `json:"language"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"`
}

func init() {
	yakit.RegisterPostInitDatabaseFunction(func() error {
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
		registerBuildInForge("biography")
		registerBuildInForge("intent_recognition")
		registerBuildInForge("entity_identify")
		registerBuildInForge("log_event_formatter")
		registerBuildInForge("event_analyzer")
		registerBuildInForge("web_log_monitor")
		registerBuildInForge("vulscan")
		registerBuildInForge("hostscan")
		registerBuildInForge("ssapoc")
		registerBuildInForge("flow_report")
		registerBuildInForge("ssa_vulnerability_analyzer")
		registerBuildInForge("mock_forge")
		return nil
	}, "sync-buildin-ai-forge")
}

func BuildInForgeHash() (string, error) {
	return filesys.CreateEmbedFSHash(buildInForge)
}

func getBuildInForgeYakScript(name string) (*schema.AIForge, error) {
	var fullName string
	if !strings.HasSuffix(name, ".yak") {
		fullName = name + ".yak"
	}
	codeBytes, err := buildInForge.ReadFile(fmt.Sprintf("buildinforge/%v", fullName))
	if err != nil {
		return nil, err
	}
	return buildAIForgeFromYakCode(name, codeBytes)
}

func buildAIForgeFromYakCode(forgeName string, codeBytes []byte) (*schema.AIForge, error) {
	prog, err := static_analyzer.SSAParse(string(codeBytes), "yak")
	if err != nil {
		return nil, err
	}

	scriptMetadata, err := metadata.ParseYakScriptMetadataProg(forgeName, prog)
	if err != nil {

		return nil, utils.Errorf("parse yak script metadata failed: %v", err)
	}

	uiParamsConfig, _, err := information.GenerateParameterFromProgram(prog)
	if err != nil {
		return nil, utils.Errorf("generate yak script parameters failed: %v", err)
	}

	return &schema.AIForge{
		ForgeName:        scriptMetadata.Name,
		ForgeVerboseName: scriptMetadata.VerboseName,
		Description:      scriptMetadata.Description,
		Tags:             strings.Join(scriptMetadata.Keywords, ","),
		ForgeContent:     string(codeBytes),
		ParamsUIConfig:   uiParamsConfig,
		ForgeType:        schema.FORGE_TYPE_YAK,
	}, nil
}

func getBuildInForgeConfig(name string) (string, *schema.AIForge, error) {
	p := fmt.Sprintf("buildinforge/%v", name)
	loadDefaultPrompt := func(promptName string) string {
		promptBytes, _ := buildInForge.ReadFile(fmt.Sprintf("%v/%v.txt", p, promptName))
		return string(promptBytes)
	}
	codeContent, _ := buildInForge.ReadFile(fmt.Sprintf("%v/%v.yak", p, name))
	configBytes, _ := buildInForge.ReadFile(fmt.Sprintf("%v/forge_cfg.json", p))
	return buildAIForgeFromConfig(name, configBytes, codeContent, loadDefaultPrompt)
}

func buildAIForgeFromConfig(name string, configBytes []byte, codeContent []byte, loadDefaultPrompt func(string) string) (string, *schema.AIForge, error) {
	forge := &schema.AIForge{
		ForgeType: schema.FORGE_TYPE_Config,
	}
	if len(configBytes) <= 0 {
		// If config file doesn't exist, try to read prompt files directly
		initPrompt := loadDefaultPrompt("init")
		persistentPrompt := loadDefaultPrompt("persistent")
		planPrompt := loadDefaultPrompt("plan")
		resultPrompt := loadDefaultPrompt("result")

		if len(initPrompt) == 0 && len(persistentPrompt) == 0 && len(planPrompt) == 0 && len(resultPrompt) == 0 {
			return "", nil, utils.Errorf("forge configuration failed for %v", name)
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
		err := json.Unmarshal(configBytes, &cfg)
		if err != nil {
			return "", nil, utils.Errorf("parse forge config failed: %v", err)
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
		forge.ForgeVerboseName = cfg.VerboseName
		forge.ToolKeywords = cfg.ToolKeywords
		forge.Tools = cfg.Tools
		forge.Description = cfg.Description
		forge.InitPrompt = cfg.InitPrompt
		forge.PersistentPrompt = cfg.PersistentPrompt
		forge.PlanPrompt = cfg.PlanPrompt
		forge.ResultPrompt = cfg.ResultPrompt
		forge.Actions = cfg.Actions
		forge.ForgeContent = cfg.ForgeContent
		forge.Tags = cfg.Tags
		forge.Params = cfg.CLIParameterRuleYaklangCode
	}
	return string(configBytes), forge, nil
}

func getBuildInForgeFromFS(name string) (*schema.AIForge, error) {
	var forge *schema.AIForge
	// First try to get forge code
	forge, err := getBuildInForgeYakScript(name)
	if err != nil {
		// Try to get forge config
		_, forge, err = getBuildInForgeConfig(name)
		if err != nil {
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
		//todo
		//err := os.CopyFS(outputDir, os.DirFS(inputDir))
		//if err != nil {
		//	return err
		//}
		//currentDir = outputDir
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
				err := updateSystemFSForgeMetaData(fileInfo, forceUpdate)
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

func updateSystemFSForgeMetaData(fileinfo *utils.FileInfo, forceUpdate bool) error {
	if fileinfo.IsDir {
		codeContent, _ := os.ReadFile(fmt.Sprintf("%v/%v.yak", fileinfo.Path, fileinfo.Name))
		configBytes, _ := os.ReadFile(fmt.Sprintf("%v/forge_cfg.json", fileinfo.Path))
		loadDefaultPrompt := func(promptName string) string {
			promptBytes, _ := os.ReadFile(fmt.Sprintf("%v/%v.txt", fileinfo.Path, promptName))
			return string(promptBytes)
		}
		simpleCfg, forge, err := buildAIForgeFromConfig(fileinfo.Name, configBytes, codeContent, loadDefaultPrompt)
		if err != nil {
			return fmt.Errorf(`failed to find forge config for "%v"`, fileinfo.Path)
		}
		if forceUpdate || forge.Tags == "" || forge.Description == "" {
			completeForgeBytes, err2 := json.Marshal(forge)
			if err2 != nil {
				return err2
			}
			metaData, err := GenerateForgeMetadata(string(completeForgeBytes))
			if err != nil {
				return err
			}

			// update forge content
			var cfg YakForgeBlueprintConfig
			err = json.Unmarshal([]byte(simpleCfg), &cfg)
			cfg.Tags = strings.Join(metaData.Keywords, ",")
			cfg.Description = metaData.Description

			newCfg, err := json.MarshalIndent(cfg, "", "\t")
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
		codeBytes, err := os.ReadFile(fileinfo.Path)
		if err != nil {
			return nil
		}

		forge, err := buildAIForgeFromYakCode(strings.TrimSuffix(fileinfo.Name, ".yak"), codeBytes)
		if err != nil {
			return fmt.Errorf(`failed to get yak script forge for "%v"`, fileinfo.Path)
		}
		if forceUpdate || forge.Tags == "" || forge.Description == "" {
			metaData, err := GenerateForgeMetadata(forge.ForgeContent)
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
