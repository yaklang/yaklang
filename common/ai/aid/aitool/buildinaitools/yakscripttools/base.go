package yakscripttools

import (
	"embed"
	"encoding/json"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools/metadata"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/yakcliconvert"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
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



var toolCovertHandle func(aitools []*schema.AIYakTool) []*aitool.Tool

func RegisterYakScriptAiToolsCovertHandle(handle func(aitools []*schema.AIYakTool) []*aitool.Tool) {
	toolCovertHandle = handle
}


func covertTools(tools []*schema.AIYakTool) []*aitool.Tool {
	if toolCovertHandle == nil {
		return nil
	}
	return toolCovertHandle(tools)
}

func GetAllYakScriptAiTools() []*aitool.Tool {
	OverrideYakScriptAiTools()
	db := consts.GetGormProfileDatabase()
	allAiTools, err := schema.SearchAIYakTool(db, "")
	if err != nil {
		log.Errorf("search ai yak tool failed: %v", err)
		return nil
	}
	return covertTools(allAiTools)
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
	return covertTools(tools)
}
