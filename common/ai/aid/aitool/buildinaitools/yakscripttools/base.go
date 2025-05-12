package yakscripttools

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools/metadata"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/yakcliconvert"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	_ "github.com/yaklang/yaklang/common/yak"
)

//go:embed yakscriptforai/**
var yakScriptFS embed.FS

var overrideYakScriptAiToolsOnce sync.Once

func OverrideYakScriptAiTools() {
	overrideYakScriptAiToolsOnce.Do(func() {
		db := consts.GetGormProfileDatabase()
		aiTools := loadAllYakScriptFromEmbedFS()
		for _, aiTool := range aiTools {
			schema.SaveAIYakTool(db, aiTool)
		}
	})
}

func loadAllYakScriptFromEmbedFS() []*schema.AIYakTool {
	aiTools := []*schema.AIYakTool{}
	efs := filesys.NewEmbedFS(yakScriptFS)
	_ = filesys.Recursive(".", filesys.WithFileSystem(efs), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		filename := info.Name()
		_, filename = efs.PathSplit(filename)
		dirname, _ := efs.PathSplit(s)
		if efs.Ext(filename) != ".yak" {
			return nil
		}
		toolname := strings.TrimSuffix(filename, ".yak")

		content, err := efs.ReadFile(s)
		if err != nil {
			return nil
		}
		aiTool := loadYakScriptToAiTools(toolname, string(content))
		if aiTool == nil {
			return nil
		}

		namePath := ""
		dirnameClean, ok := strings.CutPrefix(dirname, `yakscriptforai`)
		if ok {
			namePath = dirnameClean
		}
		namePath = strings.Trim(namePath, `/`)
		aiTool.Path = filepath.Join(namePath, toolname)

		aiTools = append(aiTools, aiTool)
		return nil
	}))
	return aiTools
}

func loadYakScriptToAiTools(name string, content string) *schema.AIYakTool {
	metadata, err := metadata.ParseYakScriptMetadata(name, string(content))
	if err != nil {
		log.Warnf("parse yak script metadata failed: %v", err)
		return nil
	}
	prog, err := static_analyzer.SSAParse(string(content), "yak")
	if err != nil {
		log.Warnf(`static_analyzer.SSAParse(string(content), "yak") error: %v`, err)
		return nil
	}
	tool := yakcliconvert.ConvertCliParameterToTool(name, prog)
	params, _ := json.Marshal(tool.InputSchema.ToMap())
	return &schema.AIYakTool{
		Name:        name,
		Description: metadata.Description,
		Keywords:    strings.Join(metadata.Keywords, ","),
		Content:     string(content),
		Params:      string(params),
	}
}

func ConvertYakScriptAiToolsToMCPTools(aiTools []*schema.AIYakTool) []*aitool.Tool {
	tools := []*aitool.Tool{}
	for _, aiTool := range aiTools {
		tool := mcp.NewTool(aiTool.Name)
		tool.Description = aiTool.Description
		dataMap := map[string]any{}
		err := json.Unmarshal([]byte(aiTool.Params), &dataMap)
		if err != nil {
			log.Errorf("unmarshal aiTool.Params failed: %v", err)
			continue
		}
		tool.InputSchema.FromMap(dataMap)
		at, err := aitool.NewFromMCPTool(
			tool,
			aitool.WithDescription(aiTool.Description),
			aitool.WithKeywords(strings.Split(aiTool.Keywords, ",")),
			aitool.WithCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				runtimeId := params.GetString("runtime_id")
				if runtimeId == "" {
					runtimeId = uuid.New().String()
				}
				yakitClient := yaklib.NewVirtualYakitClientWithRuntimeID(func(i *ypb.ExecResult) error {
					if i.IsMessage {
						stdout.Write([]byte(yaklib.ConvertExecResultIntoLog(i)))
						stdout.Write([]byte("\n"))
					}
					return nil
				}, runtimeId)
				engine := yak.NewYakitVirtualClientScriptEngine(yakitClient)

				var args []string
				for k, v := range params {
					args = append(args, "--"+k, fmt.Sprint(v))
				}
				cliApp := yak.GetHookCliApp(args)
				engine.RegisterEngineHooks(func(ae *antlr4yak.Engine) error {
					yak.BindYakitPluginContextToEngine(
						ae,
						yak.CreateYakitPluginContext(
							runtimeId,
						).WithContext(
							ctx,
						).WithContextCancel(
							cancel,
						).WithCliApp(
							cliApp,
						).WithYakitClient(
							yakitClient,
						),
					)
					return nil
				})

				_, err = engine.ExecuteExWithContext(ctx, aiTool.Content, map[string]interface{}{
					"RUNTIME_ID":   runtimeId,
					"CTX":          ctx,
					"PLUGIN_NAME":  runtimeId + ".yak",
					"YAK_FILENAME": runtimeId + ".yak",
				})
				if err != nil {
					log.Errorf("execute ex with context failed: %v", err)
					stderr.Write([]byte(err.Error()))
					return nil, err
				}
				return "", nil
			}))
		if err != nil {
			log.Errorf(`at.NewFromMCPTool(tool): %v`, err)
			return nil
		}
		tools = append(tools, at)
	}
	return tools
}

func GetAllYakScriptAiTools() []*aitool.Tool {
	OverrideYakScriptAiTools()
	db := consts.GetGormProfileDatabase()
	allAiTools, err := schema.SearchAIYakTool(db, "")
	if err != nil {
		log.Errorf("search ai yak tool failed: %v", err)
		return nil
	}
	return ConvertYakScriptAiToolsToMCPTools(allAiTools)
}
func GetYakScriptAiTools(names ...string) []*aitool.Tool {
	OverrideYakScriptAiTools()
	db := consts.GetGormProfileDatabase()
	tools := []*schema.AIYakTool{}
	toolsNameMap := map[string]struct{}{}
	for _, name := range names {
		dbAiTools, err := schema.SearchAIYakToolByPath(db, name)
		if err != nil {
			log.Errorf("search ai yak tool failed: %v", err)
			continue
		}
		for _, tool := range dbAiTools {
			if _, ok := toolsNameMap[tool.Name]; ok {
				continue
			}
			toolsNameMap[tool.Name] = struct{}{}
			tools = append(tools, tool)
		}
	}
	return ConvertYakScriptAiToolsToMCPTools(tools)
}
