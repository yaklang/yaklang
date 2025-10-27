package yakscripttools

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools/metadata"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/yakcliconvert"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	utils "github.com/yaklang/yaklang/common/utils/resources_monitor"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
)

//go:generate gzip-embed -cache --source ./yakscriptforai --gz yakscriptforai.tar.gz --no-embed

// FileSystemWithHash 是一个带有 GetHash 方法的文件系统接口
type FileSystemWithHash interface {
	fi.FileSystem
	GetHash() (string, error)
}

func init() {
	InitEmbedFS()
	yakit.RegisterPostInitDatabaseFunction(func() error {
		if result, ok := os.LookupEnv("SKIP_SYNC_BUILD_IN_AI_TOOL"); ok {
			r, _ := strconv.ParseBool(result)
			if r {
				return nil
			}
		}
		const key = "2b709ef7252a06a0c1cfbb952f77f976"
		return utils.NewEmbedResourcesMonitor(key, consts.ExistedBuildInAIToolEmbedFSHash).MonitorModifiedWithAction(func() string {
			buildinHash, _ := BuildInAIToolHash()
			return buildinHash
		}, func() error {
			OverrideYakScriptAiTools()
			return nil
		})
	}, "sync-ai-tool")
}

func BuildInAIToolHash() (string, error) {
	return yakScriptFS.GetHash()
}

var overrideYakScriptAiToolsOnce sync.Once

func OverrideYakScriptAiTools() {
	overrideYakScriptAiToolsOnce.Do(func() {
		db := consts.GetGormProfileDatabase()
		aiTools, err := loadAllYakScriptFromEmbedFS()
		if err != nil {
			log.Errorf("load all yak script from embed fs failed: %v", err)
			return
		}
		for _, aiTool := range aiTools {
			yakit.SaveAIYakTool(db, aiTool)
		}
	})
}

func loadAllYakScriptFromEmbedFS() ([]*schema.AIYakTool, error) {
	aiTools := []*schema.AIYakTool{}
	efs := yakScriptFS
	err := filesys.Recursive(".", filesys.WithFileSystem(yakScriptFS), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
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
		aiTool := LoadYakScriptToAiTools(toolname, string(content))
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
	return aiTools, err
}

func LoadYakScriptToAiTools(name string, content string) *schema.AIYakTool {
	ins, err := metadata.ParseYakScriptMetadata(name, string(content))
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
		VerboseName: ins.VerboseName,
		Description: ins.Description,
		Keywords:    strings.Join(ins.Keywords, ","),
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
	db := consts.GetGormProfileDatabase()
	allAiTools, err := yakit.SearchAIYakTool(db, "")
	if err != nil {
		log.Errorf("search ai yak tool failed: %v", err)
		return nil
	}
	return covertTools(allAiTools)
}
func GetYakScriptAiTools(names ...string) []*aitool.Tool {
	db := consts.GetGormProfileDatabase()
	tools := []*schema.AIYakTool{}
	toolsNameMap := map[string]struct{}{}
	for _, name := range names {
		dbAiTools, err := yakit.SearchAIYakToolByPath(db, name)
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
