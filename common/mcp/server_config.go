package mcp

import (
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/yakcliconvert"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
)

type MCPServerConfig struct {
	enableTools      map[string]*ToolWithHandler
	disableTools     map[string]*ToolWithHandler
	enableResources  map[string]*ResourceWithHandler
	disableResources map[string]*ResourceWithHandler
	dynamicScript    []string
	// extraAITools holds aitool.Tool instances registered via WithAITools.
	// They are merged last and override globalTools entries with the same name.
	extraAITools map[string]*ToolWithHandler
	// bridgeClientClosers holds outbound MCP clients for bridged aitools; each
	// client is closed once when the Yak MCP server shuts down.
	bridgeClientClosers []io.Closer
}

func NewMCPServerConfig() *MCPServerConfig {
	return &MCPServerConfig{
		enableTools:      make(map[string]*ToolWithHandler),
		disableTools:     make(map[string]*ToolWithHandler),
		enableResources:  make(map[string]*ResourceWithHandler),
		disableResources: make(map[string]*ResourceWithHandler),
		extraAITools:     make(map[string]*ToolWithHandler),
	}
}

func (cfg *MCPServerConfig) trackBridgeClientCloser(c io.Closer) {
	if c == nil {
		return
	}
	for _, existing := range cfg.bridgeClientClosers {
		if existing == c {
			return
		}
	}
	cfg.bridgeClientClosers = append(cfg.bridgeClientClosers, c)
}

func (cfg *MCPServerConfig) ApplyConfig(s *MCPServer) {
	// Legacy MCP tools are only exposed when at least one tool set was enabled
	// via WithEnableToolSet (typically driven by StartMcpServerRequest.EnableAll).
	tools := make(map[string]*ToolWithHandler)
	if len(cfg.enableTools) > 0 {
		for name, tool := range cfg.enableTools {
			if _, disabled := cfg.disableTools[name]; disabled {
				continue
			}
			tools[name] = tool
		}
	}

	// extraAITools (registered via WithAITools) are merged last and override
	// any legacy entry with the same name.
	for name, tool := range cfg.extraAITools {
		if _, disabled := cfg.disableTools[name]; disabled {
			continue
		}
		tools[name] = tool
	}

	for _, tool := range tools {
		s.server.AddTool(tool.tool, tool.handler(s))
	}

	resources := cfg.enableResources
	if len(resources) == 0 {
		resources = nil
	}
	for name, resource := range resources {
		if _, ok := cfg.disableResources[name]; ok {
			continue
		}
		if resource.resource != nil {
			s.server.AddResource(resource.resource, resource.handler(s))
		} else if resource.resourceTemplate != nil {
			s.server.AddResourceTemplate(resource.resourceTemplate, resource.handler(s))
		}
	}

	if len(cfg.dynamicScript) > 0 {
		old := log.GetLevel()
		log.SetLevel(log.FatalLevel)
		defer log.SetLevel(old)
		for _, script := range cfg.dynamicScript {
			f, err := os.Open(script)
			if err != nil {
				log.Errorf("failed to open yak script file: %v", err)
				continue
			}
			contentBytes, err := io.ReadAll(f)
			if err != nil {
				log.Errorf("failed to read yak script file: %v", err)
				continue
			}
			defer f.Close()

			toolName := filepath.Base(script)
			content := string(contentBytes)
			ext := filepath.Ext(toolName)
			if ext != "" {
				toolName = strings.TrimSuffix(toolName, ext)
			}

			prog, err := static_analyzer.SSAParse(string(content), "yak")
			if err != nil {
				log.Errorf("failed to parse yak script: %v", err)
			}

			tool := yakcliconvert.ConvertCliParameterToTool(toolName, prog)
			s.server.AddTool(tool, s.execYakScriptWrapper(toolName, content))
		}
	}
}

type McpServerOption func(*MCPServerConfig) error

func WithEnableTool(name string) McpServerOption {
	return func(cfg *MCPServerConfig) error {
		tool, ok := globalTools[name]
		if !ok {
			return utils.Errorf("undefined tool: %s", name)
		}
		cfg.enableTools[name] = tool
		return nil
	}
}

func WithEnableToolSet(name string) McpServerOption {
	return func(cfg *MCPServerConfig) error {
		toolSet, ok := globalToolSets[name]
		if !ok {
			return utils.Errorf("undefined tool set: %s", name)
		}
		maps.Copy(cfg.enableTools, toolSet.Tools)
		return nil
	}
}

// WithEnableAllToolSets registers every legacy tool set, matching StartMcpServer EnableAll.
func WithEnableAllToolSets() McpServerOption {
	return func(cfg *MCPServerConfig) error {
		for _, name := range GlobalToolSetList() {
			if err := WithEnableToolSet(name)(cfg); err != nil {
				return err
			}
		}
		return nil
	}
}

func WithDisableTool(name string) McpServerOption {
	return func(cfg *MCPServerConfig) error {
		tool, ok := globalTools[name]
		if !ok {
			return utils.Errorf("undefined tool: %s", name)
		}
		cfg.disableTools[name] = tool
		return nil
	}
}

func WithDisableToolSet(name string) McpServerOption {
	return func(cfg *MCPServerConfig) error {
		toolSet, ok := globalToolSets[name]
		if !ok {
			return utils.Errorf("undefined tool set: %s", name)
		}
		maps.Copy(cfg.disableTools, toolSet.Tools)
		return nil
	}
}

func WithEnableResource(name string) McpServerOption {
	return func(cfg *MCPServerConfig) error {
		resource, ok := globalResources[name]
		if !ok {
			return utils.Errorf("undefined resource: %s", name)
		}
		cfg.enableResources[name] = resource
		return nil
	}
}

func WithEnableResourceSet(name string) McpServerOption {
	return func(cfg *MCPServerConfig) error {
		resourceSet, ok := globalResourceSets[name]
		if !ok {
			return utils.Errorf("undefined resource set: %s", name)
		}
		maps.Copy(cfg.enableResources, resourceSet.Resources)
		return nil
	}
}

func WithDisableResource(name string) McpServerOption {
	return func(cfg *MCPServerConfig) error {
		resource, ok := globalResources[name]
		if !ok {
			return utils.Errorf("undefined resource: %s", name)
		}
		cfg.disableResources[name] = resource
		return nil
	}
}

func WithDisableResourceSet(name string) McpServerOption {
	return func(cfg *MCPServerConfig) error {
		resourceSet, ok := globalResourceSets[name]
		if !ok {
			return utils.Errorf("undefined resource set: %s", name)
		}
		maps.Copy(cfg.disableResources, resourceSet.Resources)
		return nil
	}
}

func WithDynamicScript(script []string) McpServerOption {
	return func(cfg *MCPServerConfig) error {
		for _, s := range script {
			if _, err := utils.GetFirstExistedFileE(s); err != nil {
				return err
			}
		}
		cfg.dynamicScript = script
		return nil
	}
}

func WithEnableCodecToolSet() McpServerOption {
	return WithEnableToolSet("codec")
}

func WithDisableCodecToolSet() McpServerOption {
	return WithDisableToolSet("codec")
}

func WithEnableCVEToolSet() McpServerOption {
	return WithEnableToolSet("cve")
}

func WithDisableCVEToolSet() McpServerOption {
	return WithDisableToolSet("cve")
}

func WithEnableHTTPFlowToolSet() McpServerOption {
	return WithEnableToolSet("httpflow")
}

func WithDisableHTTPFlowToolSet() McpServerOption {
	return WithDisableToolSet("httpflow")
}

func WithEnableHybridScanToolSet() McpServerOption {
	return WithEnableToolSet("hybrid_scan")
}
func WithDisableHybridScanToolSet() McpServerOption {
	return WithDisableToolSet("hybrid_scan")
}

func WithEnablePayloadToolSet() McpServerOption {
	return WithEnableToolSet("payload")
}

func WithDisablePayloadToolSet() McpServerOption {
	return WithDisableToolSet("payload")
}

func WithEnablePortScanToolSet() McpServerOption {
	return WithEnableToolSet("port_scan")
}

func WithDisablePortScanToolSet() McpServerOption {
	return WithDisableToolSet("port_scan")
}

func WithEnableYakDocumentToolSet() McpServerOption {
	return WithEnableToolSet("yak_document")
}

func WithDisableYakDocumentToolSet() McpServerOption {
	return WithDisableToolSet("yak_document")
}

func WithEnableYakScriptToolSet() McpServerOption {
	return WithEnableToolSet("yak_script")
}

func WithDisableYakScriptToolSet() McpServerOption {
	return WithDisableToolSet("yak_script")
}

func WithEnableSSAToolSet() McpServerOption {
	return WithEnableToolSet("ssa")
}

func WithDisableSSAToolSet() McpServerOption {
	return WithDisableToolSet("ssa")
}

// WithDisabledToolNames registers a set of tool names that should be excluded
// from the MCP server even if they are part of an enabled tool set. This is
// used to apply per-tool enable/disable state stored in the profile DB.
func WithDisabledToolNames(names map[string]struct{}) McpServerOption {
	return func(cfg *MCPServerConfig) error {
		for name := range names {
			// We use a sentinel ToolWithHandler (nil tool) — ApplyConfig only
			// checks key presence in disableTools to decide whether to skip.
			cfg.disableTools[name] = &ToolWithHandler{}
		}
		return nil
	}
}
