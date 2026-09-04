package buildinaitools

import (
	"io"
	"sync"
	"time"

	"github.com/samber/lo"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/codeaudittools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/fstools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/notifytools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/scheduletools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/ssatools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func GetBasicBuildInTools() []*aitool.Tool {
	nowTime, err := aitool.New(
		"now",
		aitool.WithDescription("get current time"),
		aitool.WithStringParam(
			"timezone",
			aitool.WithParam_Required(false),
			aitool.WithParam_Description("timezone for now, like 'Asia/Shanghai' or 'UTC' ... "),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			return time.Now().String(), nil
		}),
	)
	if err != nil {
		log.Errorf("create now tool: %v", err)
	}

	tools := []*aitool.Tool{nowTime}
	tools = append(tools, scheduletools.CreateScheduleTools()...)
	return lo.Filter(tools, func(item *aitool.Tool, index int) bool {
		if utils.IsNil(item) {
			log.Errorf("tool is nil")
			return false
		}
		return true
	})
}

var (
	allAiToolsMu      sync.RWMutex
	allAiTools        []*aitool.Tool
	doGetAllToolsOnce sync.Once
)

func publishAllAITools(tools []*aitool.Tool) []*aitool.Tool {
	// Store a private backing array and return a different copy. Neither the
	// producer nor the caller can mutate the globally published slice header or
	// its entries through an alias.
	published := append([]*aitool.Tool(nil), tools...)
	allAiToolsMu.Lock()
	allAiTools = published
	allAiToolsMu.Unlock()
	return append([]*aitool.Tool(nil), published...)
}

func getAllAIToolsSnapshot() []*aitool.Tool {
	allAiToolsMu.RLock()
	snapshot := append([]*aitool.Tool(nil), allAiTools...)
	allAiToolsMu.RUnlock()
	return snapshot
}

// GetAllToolsDynamically returns all built-in AI tools, dynamically get from the database
func GetAllToolsDynamically(db *gorm.DB) []*aitool.Tool {
	var tools []*aitool.Tool

	// Add basic tools
	tools = append(tools, GetBasicBuildInTools()...)

	// Add filesystem tools from fstools package
	fsTools, err := fstools.CreateSystemFSTools()
	if err != nil {
		log.Errorf("create fs tools: %v", err)
	} else {
		tools = append(tools, fsTools...)
	}

	// Add yakscripttools from yakscripttools package
	yakscriptTools := yakscripttools.GetAllYakScriptAiToolsByDB(db)
	tools = append(tools, yakscriptTools...)

	// Add SSA + SyntaxFlow tools from ssatools package
	ssaToolsList, err := ssatools.CreateSSATools()
	if err != nil {
		log.Errorf("create ssa tools: %v", err)
	} else {
		tools = append(tools, ssaToolsList...)
	}

	// Add code audit tools from codeaudittools package (Java static security audit)
	tools = append(tools, codeaudittools.CreateCodeAuditTools()...)

	// Add language-generic code audit tools (project_probe etc., with required
	// `language` param dispatching java/python/go/php/node content packs)
	tools = append(tools, codeaudittools.CreateGenericCodeAuditTools()...)

	// Add IM notify tools (send_im_message / configure_im_credentials) from notifytools package
	tools = append(tools, notifytools.CreateNotifySendTools()...)

	// Add generated tools (added by code-gen when run)
	// These functions will be generated based on aitools.tools by the code generator
	// Example:
	// tools = append(tools, GetSystemTools()...)  // From system_tools.go
	// tools = append(tools, GetFilesystemTools()...)  // From filesystem_tools.go
	// tools = append(tools, GetExampleTools()...)  // From example_tools.go

	// Deduplicate tools by name: later entries (yakscript) override earlier ones (go builtin).
	seen := make(map[string]int) // name -> index of last occurrence
	for i, t := range tools {
		if utils.IsNil(t) {
			continue
		}
		if prevIdx, exists := seen[t.Name]; exists {
			log.Warnf("tool %q has duplicate registration (index %d and %d), later version will be used, earlier version is dropped", t.Name, prevIdx, i)
		}
		seen[t.Name] = i
	}
	var deduped []*aitool.Tool
	for i, t := range tools {
		if utils.IsNil(t) {
			continue
		}
		if seen[t.Name] == i {
			deduped = append(deduped, t)
		}
	}
	tools = deduped

	tools = lo.Filter(tools, func(item *aitool.Tool, index int) bool {
		if utils.IsNil(item) {
			log.Errorf("tool is nil")
			return false
		}
		return true
	})
	return publishAllAITools(tools)
}

// GetAllTools returns all built-in AI tools, including generated ones
func GetAllTools() []*aitool.Tool {
	doGetAllToolsOnce.Do(func() {
		GetAllToolsDynamically(consts.GetGormProfileDatabase())
	})
	return getAllAIToolsSnapshot()
}
