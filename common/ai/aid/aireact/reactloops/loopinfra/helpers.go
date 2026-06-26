package loopinfra

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils/yakgit/yakdiff"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/utils"
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

const singleFileTimelinePreviewBytes = 800

type loopInfraFileOpTimeline struct {
	Op         string // write, modify, insert, delete
	Filename   string
	OldSegment string
	NewSegment string
	StartLine  int
	EndLine    int
	InsertLine int
	Deferred   bool
}

func loopInfraExtractLineRange(content string, startLine, endLine int) string {
	if content == "" || startLine < 1 {
		return ""
	}
	lines := utils.ParseStringToRawLines(content)
	if startLine > len(lines) {
		return ""
	}
	if endLine < startLine {
		endLine = startLine
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	return strings.Join(lines[startLine-1:endLine], "\n")
}

func loopInfraFormatSegmentDiff(oldText, newText string) string {
	const maxSeg = 600
	oldText = strings.TrimRight(oldText, "\n")
	newText = strings.TrimRight(newText, "\n")

	diffResult, err := yakdiff.Diff(oldText, newText)
	if err != nil {
		// If diff fails for any reason, fall back to simple old vs new display
		var parts []string
		parts = append(parts, "--- removed ---\n"+utils.PrefixLines(utils.ShrinkTextBlock(oldText, maxSeg), "- "))
		parts = append(parts, "+++ added ---\n"+utils.PrefixLines(utils.ShrinkTextBlock(newText, maxSeg), "+ "))
		if len(parts) == 0 {
			return "(no visible change)"
		}
		return strings.Join(parts, "\n")
	} else {
		return diffResult
	}
}

func loopInfraFormatFileOpTimeline(spec loopInfraFileOpTimeline) string {
	if spec.Filename == "" {
		return ""
	}
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("File: %s\n", spec.Filename))
	if spec.Deferred {
		buf.WriteString("Note: disk write deferred for frontend review\n")
	}

	switch spec.Op {
	case "write":
		buf.WriteString(fmt.Sprintf("Operation: write (%d bytes)\n\n", len(spec.NewSegment)))
		buf.WriteString("Written content:\n")
		buf.WriteString(utils.PrefixLines(utils.ShrinkTextBlock(spec.NewSegment, singleFileTimelinePreviewBytes), "  "))
	case "modify":
		buf.WriteString(fmt.Sprintf("Operation: modify lines %d-%d\n\n", spec.StartLine, spec.EndLine))
		buf.WriteString(loopInfraFormatSegmentDiff(spec.OldSegment, spec.NewSegment))
	case "insert":
		buf.WriteString(fmt.Sprintf("Operation: insert at line %d\n\n", spec.InsertLine))
		buf.WriteString(loopInfraFormatSegmentDiff("", spec.NewSegment))
	case "delete":
		if spec.EndLine > 0 && spec.EndLine != spec.StartLine {
			buf.WriteString(fmt.Sprintf("Operation: delete lines %d-%d\n\n", spec.StartLine, spec.EndLine))
		} else {
			buf.WriteString(fmt.Sprintf("Operation: delete line %d\n\n", spec.StartLine))
		}
		buf.WriteString(loopInfraFormatSegmentDiff(spec.OldSegment, ""))
	default:
		return ""
	}
	return buf.String()
}

func loopInfraAddFileOpSuccessTimeline(loop *reactloops.ReActLoop, spec loopInfraFileOpTimeline) {
	if loop == nil || spec.Op == "" {
		return
	}
	body := loopInfraFormatFileOpTimeline(spec)
	if body == "" {
		return
	}
	spilled, _ := reactloops.SpillLongContent(loop, "single_file_"+spec.Op, body)
	loop.GetInvoker().AddToTimeline("file_"+spec.Op, spilled)
}
