package loopinfra

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	loopInfraNodeToolCompose        = "tool_compose_progress"
	loopInfraNodeLoadCapability     = "load_capability"
	loopInfraNodeLoadSkillResources = "load_skill_resources_path"
	loopInfraNodeSingleFileWrite    = "write_code"
	loopInfraNodeSingleFileModify   = "code_modified"
	loopInfraNodeSingleFileInsert   = "code_modified"
	loopInfraNodeSingleFileDelete   = "code_modified"
	loopInfraNodeQueryMCPServers    = "query_mcp_servers"
	loopInfraNodeQueryMCPTools      = "query_mcp_tools"
)

const loopInfraReferencePreviewBytes = 1200

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
	content = strings.TrimSpace(content)
	if content == "" || loop == nil {
		return "", ""
	}
	if previewBytes <= 0 {
		previewBytes = loopInfraReferencePreviewBytes
	}
	preview = utils.ShrinkTextBlock(content, previewBytes)
	if len(content) <= previewBytes {
		return "", preview
	}
	dataDir := loop.GetLoopContentDir("data")
	if dataDir == "" {
		return "", preview
	}
	filename = filepath.Join(dataDir, fmt.Sprintf("%s_%d_%s.txt", prefix, loop.GetCurrentIterationIndex(), utils.DatetimePretty2()))
	if err := reactloops.SaveAndPinFile(loop, filename, []byte(content)); err != nil {
		log.Warnf("loopinfra: failed to save %s reference: %v", prefix, err)
		return "", preview
	}
	return filename, preview
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
