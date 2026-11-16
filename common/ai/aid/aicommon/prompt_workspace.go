package aicommon

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var codingPromptTemplate = promptloader.MustLoad("mainloop/coding.txt")

var workspacePromptTemplate = promptloader.MustLoad("mainloop/workspace.txt")

// WorkspaceContext renders configured coordinates without inspecting the file system.
func (m *PromptMaterials) WorkspaceContext() string {
	if m == nil || !m.Workspace || strings.TrimSpace(m.OSArch+m.WorkingDir+m.AIArtifactsDir) == "" {
		return ""
	}
	content, err := RenderPromptTemplate("workspace", workspacePromptTemplate, m)
	if err != nil {
		return ""
	}
	if m.AgentMode == ypb.AgentModeCoding {
		codingContent, codingErr := RenderPromptTemplate("coding", codingPromptTemplate, m)
		if codingErr == nil {
			content += "\n" + codingContent
		}
	}
	return strings.TrimSpace(content)
}
