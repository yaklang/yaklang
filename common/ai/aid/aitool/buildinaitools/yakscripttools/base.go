package yakscripttools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools/metadata"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/yakcliconvert"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
)

//go:generate gzip-embed -cache --source ./yakscriptforai --gz yakscriptforai.tar.gz --no-embed

// FileSystemWithHash 是一个带有 GetHash 方法的文件系统接口
type FileSystemWithHash interface {
	fi.FileSystem
	GetHash() (string, error)
}

const builtinAIToolEmbedKey = "2b709ef7252a06a0c1cfbb952f77f976"

func init() {
	InitEmbedFS()
	yakit.RegisterPostInitDatabaseFunction(func() error {
		if result, ok := os.LookupEnv("SKIP_SYNC_BUILD_IN_AI_TOOL"); ok {
			r, _ := strconv.ParseBool(result)
			if r {
				return nil
			}
		}
		hash := consts.ExistedBuildInAIToolEmbedFSHash
		if consts.IsDevMode() {
			var err error
			hash, err = BuildInAIToolHash()
			if err != nil {
				return err
			}
		}
		overrideYakScriptAiToolsMu.Lock()
		err := syncBuiltinAITools(consts.GetGormProfileDatabase(), hash+":"+schema.BuiltinAIToolExtractorVersion)
		overrideYakScriptAiToolsMu.Unlock()
		if err != nil {
			return err
		}
		UpdateAIYakToolAIOutputOption()
		return nil
	}, "sync-ai-tool")
}

func UpdateAIYakToolAIOutputOption() {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return
	}
	// Only legacy user tools need the fallback; builtin tools are parsed at sync.
	db = db.Model(&schema.AIYakTool{}).Where("enable_ai_output_log = ? AND is_builtin = ?", 0, false)
	toolsChan := yakit.YieldAllAITools(context.Background(), db)
	for tool := range toolsChan {
		mode := ParseAIToolEnableAIOutputLog(tool.Content)
		if mode == 0 {
			continue
		}
		tool.EnableAIOutputLog = mode
		if _, err := yakit.UpdateAIYakToolByID(db, tool); err != nil {
			log.Errorf("failed to update AI tool[%s] AIOutput flag: %v", tool.Name, err)
		}
	}
}

func ParseAIToolEnableAIOutputLog(code string) int {
	prog, err := static_analyzer.SSAParse(code, "yak")
	if err != nil {
		log.Warnf("failed to parse AI tool content for AIOutput: %v", err)
		return 1
	}
	return aiToolOutputMode(code, prog)
}

func aiToolOutputMode(code string, prog *ssaapi.Program) int {
	if prog.SyntaxFlowChain(`AIOutput<getCallee> as $call`).Len() > 0 {
		return 2
	}
	if strings.Contains(code, "yakit.AIOutput") || strings.Contains(code, "AIOutput(") {
		return 2
	}
	return 1
}

func BuildInAIToolHash() (string, error) {
	return yakScriptFS.GetHash()
}

func GetEmbedFS() fi.FileSystem {
	InitEmbedFS()
	return yakScriptFS
}

var overrideYakScriptAiToolsMu sync.Mutex

var parseBuiltinAITool = func(name, content string) *schema.AIYakTool {
	return loadYakScriptToAiTools(name, content, false)
}

func OverrideYakScriptAiTools() error {
	overrideYakScriptAiToolsMu.Lock()
	defer overrideYakScriptAiToolsMu.Unlock()
	return overrideYakScriptAiTools(consts.GetGormProfileDatabase())
}

func overrideYakScriptAiTools(db *gorm.DB) error {
	return syncBuiltinAITools(db, "")
}

func syncBuiltinAITools(db *gorm.DB, resourceHash string) error {
	if db == nil {
		return fmt.Errorf("AI tool profile database is not initialized")
	}
	if resourceHash != "" && yakit.GetKey(db, builtinAIToolEmbedKey) == resourceHash {
		return nil
	}
	start := time.Now()
	aiTools, err := loadAllYakScriptFromEmbedFS()
	if err != nil {
		return fmt.Errorf("load builtin AI tools: %w", err)
	}
	// Include colliding user names in the same query so an edited builtin is
	// never overwritten or recreated (Author remains builtin after an edit).
	names := make([]string, 0, len(aiTools))
	for _, tool := range aiTools {
		names = append(names, tool.Name)
	}
	var stored []schema.AIYakTool
	if err := db.Select("id, name, hash, is_builtin, enable_ai_output_log").
		Where("is_builtin = ? OR name IN (?)", true, names).Find(&stored).Error; err != nil {
		return err
	}
	existing := make(map[string]schema.AIYakTool, len(stored))
	for _, tool := range stored {
		existing[tool.Name] = tool
	}
	changed := make([]*schema.AIYakTool, 0)
	for _, tool := range aiTools {
		old, ok := existing[tool.Name]
		delete(existing, tool.Name)
		if ok && (!old.IsBuiltin || (old.Hash == tool.CalcHash() && old.EnableAIOutputLog != 0)) {
			continue
		}
		tool.ID = old.ID
		changed = append(changed, tool)
	}
	group := new(errgroup.Group)
	group.SetLimit(2)
	for i, tool := range changed {
		group.Go(func() error {
			parsed := parseBuiltinAITool(tool.Name, tool.Content)
			if parsed == nil {
				return fmt.Errorf("parse builtin AI tool %q.yak", tool.Path)
			}
			parsed.ID, parsed.Author, parsed.IsBuiltin, parsed.Path = tool.ID, tool.Author, true, tool.Path
			changed[i] = parsed
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}
	deleted := 0
	for _, old := range existing {
		if old.IsBuiltin {
			deleted++
		}
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := removeRetiredBuiltInAITools(tx, names...); err != nil {
			return err
		}
		for _, tool := range changed {
			if tool.ID == 0 {
				err = tx.Create(tool).Error
			} else {
				updates := tool.ToUpdateMap()
				delete(updates, "is_favorite")
				err = tx.Model(tool).Where("is_builtin = ?", true).Updates(updates).Error
			}
			if err != nil {
				return fmt.Errorf("save builtin AI tool %q: %w", tool.Name, err)
			}
		}
		if resourceHash != "" {
			return yakit.SetKey(tx, builtinAIToolEmbedKey, resourceHash)
		}
		return nil
	})
	if err == nil {
		log.Infof("ai-tool sync: total=%d changed=%d deleted=%d elapsed=%s", len(aiTools), len(changed), deleted, time.Since(start))
	}
	return err
}

// 清理升级前写入 profile 数据库的内置工具，保留同名的用户自定义工具。
func removeRetiredBuiltInAITools(db *gorm.DB, currentNames ...string) error {
	// Editing a builtin clears IsBuiltin but preserves Author. Author alone
	// cannot distinguish a retired builtin from user-owned content.
	db = db.Where("is_builtin = ?", true)
	if len(currentNames) > 0 {
		db = db.Where("name NOT IN (?)", currentNames)
	}
	return db.Unscoped().Delete(&schema.AIYakTool{}).Error
}

func loadAllYakScriptFromEmbedFS() ([]*schema.AIYakTool, error) {
	aiTools := []*schema.AIYakTool{}
	efs := yakScriptFS
	err := fs.WalkDir(efs, ".", func(s string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		filename := entry.Name()
		_, filename = efs.PathSplit(filename)
		dirname, _ := efs.PathSplit(s)
		if efs.Ext(filename) != ".yak" {
			return nil
		}
		toolname := strings.TrimSuffix(filename, ".yak")
		if schema.AIToolDeprecationError(toolname) != nil {
			return nil
		}

		content, err := efs.ReadFile(s)
		if err != nil {
			return fmt.Errorf("read builtin AI tool %q: %w", s, err)
		}
		aiTool := &schema.AIYakTool{Name: toolname, Content: string(content), Author: schema.AIResourceAuthorBuiltin, IsBuiltin: true}

		namePath := strings.Trim(strings.TrimPrefix(dirname, "yakscriptforai"), "/")
		aiTool.Path = filepath.Join(namePath, toolname)

		aiTools = append(aiTools, aiTool)
		return nil
	})
	return aiTools, err
}

// LoadYakScriptToAiTools 将一段 Yak 插件源码解析为 AI 工具描述（导出名为 aiagent.ParseYakScriptToAiTools）
// 会从源码中提取元数据（名称、描述、关键词等）以及 CLI 参数并转换为工具输入 schema
// 参数:
//   - name: 工具名称
//   - content: Yak 插件源码
//
// 返回值:
//   - AI 工具描述对象（解析失败时返回 nil）
//
// Example:
// ```
// code = `cli.String("target"); cli.check()`
// tool = aiagent.ParseYakScriptToAiTools("demo", code)
// dump(tool)
// ```
func LoadYakScriptToAiTools(name string, content string) *schema.AIYakTool {
	return loadYakScriptToAiTools(name, content, true)
}

func loadYakScriptToAiTools(name, content string, enableCache bool) *schema.AIYakTool {
	// Parse once and reuse the program for both metadata and CLI parameter conversion
	// Cached programs are mutable; each sync worker owns its program.
	prog, err := static_analyzer.SSAParse(content, "yak", ssaapi.WithEnableCache(enableCache))
	if err != nil {
		log.Warnf("static_analyzer.SSAParse content[%q] failed: %v", name, err)
		return nil
	}

	ins, err := metadata.ParseYakScriptMetadataProg(name, prog)
	if err != nil {
		log.Warnf("parse yak script metadata failed: %v", err)
		return nil
	}

	tool := yakcliconvert.ConvertCliParameterToTool(name, prog)
	if ins.InputConstraints != "" {
		var constraints map[string]any
		if err := json.Unmarshal([]byte(ins.InputConstraints), &constraints); err != nil || constraints == nil {
			log.Errorf("invalid input constraints for Yak tool %q: %v", name, err)
			return nil
		}
		tool.InputSchema.AllOf = append(tool.InputSchema.AllOf, constraints)
	}
	params, _ := json.Marshal(tool.InputSchema.ToMap())
	return &schema.AIYakTool{
		EnableAIOutputLog: aiToolOutputMode(content, prog),
		Name:              name,
		VerboseName:       ins.VerboseName,
		VerboseNameZh:     ins.VerboseNameZh,
		Description:       ins.Description,
		Keywords:          strings.Join(ins.Keywords, ","),
		Usage:             ins.Usage,
		DefaultAsync:      ins.DefaultAsync,
		Content:           content,
		Params:            string(params),
	}
}

var toolCovertHandle func(aitools []*schema.AIYakTool) []*aitool.Tool

func RegisterYakScriptAiToolsCovertHandle(handle func(aitools []*schema.AIYakTool) []*aitool.Tool) {
	toolCovertHandle = handle
}

var yakScriptPluginConvertHandle func(script *schema.YakScript) (*aitool.Tool, error)

func RegisterYakScriptPluginConvertHandle(handle func(script *schema.YakScript) (*aitool.Tool, error)) {
	yakScriptPluginConvertHandle = handle
}

func ConvertYakScriptPlugin(script *schema.YakScript) (*aitool.Tool, error) {
	if yakScriptPluginConvertHandle == nil {
		return nil, nil
	}
	return yakScriptPluginConvertHandle(script)
}

func covertTools(tools []*schema.AIYakTool) []*aitool.Tool {
	if toolCovertHandle == nil {
		return nil
	}
	active := make([]*schema.AIYakTool, 0, len(tools))
	for _, tool := range tools {
		if tool != nil && schema.AIToolDeprecationError(tool.Name) == nil {
			active = append(active, tool)
		}
	}
	return toolCovertHandle(active)
}

// ConvertTools 将 AIYakTool 转换为 aitool.Tool (公开接口)
func ConvertTools(tools []*schema.AIYakTool) []*aitool.Tool {
	return covertTools(tools)
}

// GetAllYakScriptAiTools 获取数据库中保存的全部 Yak 脚本 AI 工具（导出名为 aiagent.AllYakScriptAiTools）
// 参数:
//   - 无
//
// 返回值:
//   - AI 工具列表
//
// Example:
// ```
// // 依赖本地 profile 数据库（示意性示例）
// tools = aiagent.AllYakScriptAiTools()
// dump(tools)
// ```
func GetAllYakScriptAiTools() []*aitool.Tool {
	db := consts.GetGormProfileDatabase()
	allAiTools, err := yakit.SearchAIYakTool(db, "")
	if err != nil {
		log.Errorf("search ai yak tool failed: %v", err)
		return nil
	}
	return covertTools(allAiTools)
}

func GetAllYakScriptAiToolsByDB(db *gorm.DB) []*aitool.Tool {
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
