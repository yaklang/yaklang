package buildinaitools

import (
	"io"
	"time"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/fstools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/searchtools"
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
		aitool.WithCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
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

// GetAllTools returns all built-in AI tools, including generated ones
func GetAllTools() []*aitool.Tool {
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

	// Add ai tools search from searchtools package
	aiSearchTools, err := searchtools.CreateAiToolsSearchTools(GetAllTools)
	if err != nil {
		log.Errorf("create ai tools search tools: %v", err)
	} else {
		tools = append(tools, aiSearchTools...)
	}

	// Add generated tools (added by code-gen when run)
	// These functions will be generated based on aitools.tools by the code generator
	// Example:
	// tools = append(tools, GetSystemTools()...)  // From system_tools.go
	// tools = append(tools, GetFilesystemTools()...)  // From filesystem_tools.go
	// tools = append(tools, GetExampleTools()...)  // From example_tools.go

	return lo.Filter(tools, func(item *aitool.Tool, index int) bool {
		if utils.IsNil(item) {
			log.Errorf("tool is nil")
			return false
		}
		return true
	})
}
