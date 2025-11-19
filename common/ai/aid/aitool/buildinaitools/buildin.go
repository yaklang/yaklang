package buildinaitools

import (
	"io"
	"sync"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/fstools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/searchtools"
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
	return lo.Filter(tools, func(item *aitool.Tool, index int) bool {
		if utils.IsNil(item) {
			log.Errorf("tool is nil")
			return false
		}
		return true
	})
}

var allAiTools []*aitool.Tool
var doGetAllToolsOnce sync.Once

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

	// Add search tools from searchtools package
	searchTools, err := searchtools.CreateOmniSearchTools()
	if err != nil {
		log.Errorf("create search tools: %v", err)
	} else {
		tools = append(tools, searchTools...)
	}

	// Add yakscripttools from yakscripttools package
	yakscriptTools := yakscripttools.GetAllYakScriptAiToolsByDB(db)
	tools = append(tools, yakscriptTools...)

	// Add generated tools (added by code-gen when run)
	// These functions will be generated based on aitools.tools by the code generator
	// Example:
	// tools = append(tools, GetSystemTools()...)  // From system_tools.go
	// tools = append(tools, GetFilesystemTools()...)  // From filesystem_tools.go
	// tools = append(tools, GetExampleTools()...)  // From example_tools.go

	allAiTools = lo.Filter(tools, func(item *aitool.Tool, index int) bool {
		if utils.IsNil(item) {
			log.Errorf("tool is nil")
			return false
		}
		return true
	})
	return allAiTools
}

// GetAllTools returns all built-in AI tools, including generated ones
func GetAllTools() []*aitool.Tool {
	doGetAllToolsOnce.Do(func() {
		GetAllToolsDynamically(consts.GetGormProfileDatabase())
	})
	return allAiTools
}
