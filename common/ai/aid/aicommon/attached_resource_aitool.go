package aicommon

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func init() {
	RegisterAttachedResourceDataFactory(
		CONTEXT_PROVIDER_TYPE_AITOOL,
		func() AttachedResourceData { return &AttachedAIToolResourceData{} },
	)
}

type AttachedAIToolResourceData struct {
	Key       string
	Name      string
	tool      *aitool.Tool
	lookupErr error
}

func (d *AttachedAIToolResourceData) Type() string {
	return CONTEXT_PROVIDER_TYPE_AITOOL
}

func (d *AttachedAIToolResourceData) Unmarshal(resource *AttachedResource) error {
	d.Key = resource.Key
	d.Name = strings.TrimSpace(resource.Value)
	if d.Name == "" {
		return fmt.Errorf("attached AI tool name is empty")
	}
	return nil
}

func (d *AttachedAIToolResourceData) BindLoopData(loop ReActLoopIF) error {
	d.tool = nil
	d.lookupErr = nil
	if d.Key != "" && d.Key != CONTEXT_PROVIDER_KEY_NAME {
		d.lookupErr = fmt.Errorf("unsupported attached AI tool key %q", d.Key)
		return nil
	}
	if loop == nil || loop.GetConfig() == nil || loop.GetConfig().GetAiToolManager() == nil {
		d.lookupErr = fmt.Errorf("AI tool manager is unavailable")
		return nil
	}
	manager := loop.GetConfig().GetAiToolManager()
	tool, err := manager.GetToolByName(d.Name)
	if err != nil || tool == nil {
		if err == nil {
			err = fmt.Errorf("AI tool %q was not found", d.Name)
		}
		d.lookupErr = err
		return nil
	}
	manager.EnableTool(tool.GetName())
	d.tool = tool
	loop.GetConfig().RecordRecentlyUsedTool(tool)
	return nil
}

func (d *AttachedAIToolResourceData) ToAttachData(ReActLoopIF) string {
	var b strings.Builder
	b.WriteString("## Attached AI Tool\n\n")
	b.WriteString(fmt.Sprintf("- Name: `%s`\n", d.Name))
	if d.lookupErr != nil {
		b.WriteString(fmt.Sprintf("- User Marked Name: `%s`\n", d.Name))
		b.WriteString(fmt.Sprintf("- Lookup Result: not found (%v)\n", d.lookupErr))
		b.WriteString("\nThe user explicitly marked this tool. It was not resolved by its stable name. Use search_capabilities to find the intended tool, ask for a corrected name if necessary, and do not silently substitute an unrelated tool.\n")
		return strings.TrimSpace(b.String())
	}
	if d.tool == nil {
		b.WriteString("- Status: unavailable: tool was not resolved\n")
		return strings.TrimSpace(b.String())
	}
	if description := strings.TrimSpace(d.tool.GetDescription()); description != "" {
		b.WriteString(fmt.Sprintf("- Description: %s\n", description))
	}
	if usage := strings.TrimSpace(d.tool.GetUsage()); usage != "" {
		b.WriteString(fmt.Sprintf("- Usage: %s\n", usage))
	}
	b.WriteString("\n### Parameters\n```json\n")
	b.WriteString(d.tool.ParamsJsonSchemaString())
	b.WriteString("\n```\n")
	b.WriteString("\nThe user explicitly selected this tool. Prioritize it for the task and use the parameter schema above. Other tools may only supplement it when necessary.\n")
	return strings.TrimSpace(b.String())
}
