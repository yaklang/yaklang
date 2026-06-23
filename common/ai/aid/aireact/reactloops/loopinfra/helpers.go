package loopinfra

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

const (
	loopInfraNodeToolCompose        = "tool_compose_progress"
	loopInfraNodeLoadCapability     = "load_capability"
	loopInfraNodeLoadSkillResources = "load_skill_resources_path"
	loopInfraNodeSingleFileWrite    = "infra-file-write"
	loopInfraNodeSingleFileModify   = "infra-file-modify"
	loopInfraNodeSingleFileInsert   = "infra-file-insert"
	loopInfraNodeSingleFileDelete   = "infra-file-delete"
	loopInfraNodeQueryMCPServers    = "query_mcp_servers"
	loopInfraNodeQueryMCPTools      = "query_mcp_tools"
)

func loopInfraStatus(loop *reactloops.ReActLoop, message string) {
	reactloops.EmitStatus(loop, message)
}

func loopInfraSystemLog(loop *reactloops.ReActLoop, nodeID, message string) {
	if loop == nil || nodeID == "" || strings.TrimSpace(message) == "" {
		return
	}
	emitter := loop.GetEmitter()
	if emitter == nil {
		return
	}
	taskID := ""
	if task := loop.GetCurrentTask(); task != nil {
		taskID = task.GetId()
	}
	_, _ = emitter.EmitDefaultSystemStreamEvent(nodeID, strings.NewReader(message), taskID)
}

func loopInfraActionStart(loop *reactloops.ReActLoop, nodeID, line, status string) {
	reactloops.EmitActionLog(loop, nodeID, line)
	loopInfraStatus(loop, status)
}

func loopInfraActionFinish(loop *reactloops.ReActLoop, nodeID, line string, reference ...string) {
	reactloops.EmitActionLog(loop, nodeID, line, reference...)
}

func loopInfraSaveReference(loop *reactloops.ReActLoop, prefix, content string, previewBytes int) (filename string, preview string) {
	return reactloops.SaveContentReference(loop, prefix, content, previewBytes)
}

func loopInfraFileReferenceSummary(title, filename, preview string) string {
	if filename == "" {
		return preview
	}
	if preview == "" {
		return fmt.Sprintf("%s: %s", title, filename)
	}
	return fmt.Sprintf("%s: %s\n\nPreview:\n%s", title, filename, preview)
}
