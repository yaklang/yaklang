package buildinaitools

import (
	"context"
	"io"
	"slices"
	"sync"
	"time"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/bashtools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/fstools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/searchtools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
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

// GetAllTools returns all built-in AI tools, including generated ones
func GetAllTools() []*aitool.Tool {
	doGetAllToolsOnce.Do(func() {
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
		yakscriptTools := yakscripttools.GetAllYakScriptAiTools()
		tools = append(tools, yakscriptTools...)

		allAiTools = lo.Filter(tools, func(item *aitool.Tool, index int) bool {
			if utils.IsNil(item) {
				log.Errorf("tool is nil")
				return false
			}
			return true
		})
	})
	return allAiTools
}

func GetAllToolsWithContext(ctx context.Context) ([]*aitool.Tool, error) {
	allTools := GetAllTools()
	copiedAllTools := slices.Clone(allTools)
	bashCtx := bashtools.NewBashSessionContext(ctx)
	bashTools, err := bashtools.CreateBashTools(bashCtx)
	if err != nil {
		log.Errorf("create bash tools: %v", err)
	}
	copiedAllTools = append(copiedAllTools, bashTools...)
	return copiedAllTools, nil
}
