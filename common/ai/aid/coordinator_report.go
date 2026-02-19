package aid

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func (c *Coordinator) generateReportViaFocusMode() error {
	reportPrompt := c.buildReportGenerationPrompt()
	reportTask := c.generateAITaskWithName(
		"report-generation",
		reportPrompt,
	)

	artifactFiles := c.collectTaskArtifactFiles()
	if len(artifactFiles) > 0 {
		var attachedResources []*aicommon.AttachedResource
		for _, f := range artifactFiles {
			attachedResources = append(attachedResources,
				aicommon.NewAttachedResource(aicommon.CONTEXT_PROVIDER_TYPE_FILE, f, f))
		}
		reportTask.SetAttachedDatas(attachedResources)
		log.Infof("report generation: attached %d artifact files as references", len(artifactFiles))
	}

	log.Infof("report generation: entering report_generating focus mode loop")
	err := c.ExecuteLoopTask(
		schema.AI_REACT_LOOP_NAME_REPORT_GENERATING,
		reportTask,
	)
	if err != nil {
		return utils.Errorf("report generation focus mode failed: %v", err)
	}
	return nil
}

func (c *Coordinator) buildReportGenerationPrompt() string {
	var builder strings.Builder

	nonce := strings.ToLower(utils.RandStringBytes(6))

	builder.WriteString(fmt.Sprintf("<|REPORT_CONTEXT_%s|>\n", nonce))

	builder.WriteString("## User Original Requirement\n\n")
	builder.WriteString(c.userInput)
	builder.WriteString("\n\n")

	if c.rootTask != nil {
		builder.WriteString("## Task Execution Progress\n\n")
		builder.WriteString(c.rootTask.ProgressWithDetail())
		builder.WriteString("\n\n")

		builder.WriteString("## Task Summaries\n\n")
		c.collectTaskSummaries(&builder, c.rootTask, 0)
		builder.WriteString("\n")
	}

	if c.ContextProvider != nil {
		timeline := c.ContextProvider.Timeline()
		if timeline != "" {
			builder.WriteString("## Execution Timeline\n\n")
			if len(timeline) > 50000 {
				builder.WriteString(timeline[:50000])
				builder.WriteString(fmt.Sprintf("\n... (truncated, total %d chars)\n", len(timeline)))
			} else {
				builder.WriteString(timeline)
			}
			builder.WriteString("\n\n")
		}
	}

	builder.WriteString(fmt.Sprintf("<|REPORT_CONTEXT_END_%s|>\n\n", nonce))

	builder.WriteString("## Report Generation Requirements\n\n")
	builder.WriteString("Based on the above task execution context, generate a comprehensive execution report in Markdown format.\n\n")
	builder.WriteString("The report MUST:\n")
	builder.WriteString("1. Summarize the original user requirement and the overall execution plan\n")
	builder.WriteString("2. Describe the execution process and results for each subtask\n")
	builder.WriteString("3. Include key findings, tool call results, and data obtained during execution\n")
	builder.WriteString("4. Provide a conclusion summarizing the overall execution outcome\n")
	builder.WriteString("5. Use a serious, objective, and neutral tone throughout the report\n")
	builder.WriteString("6. Do NOT use any emoji characters in the report content\n")
	builder.WriteString("7. Use only ASCII characters, Chinese characters, and necessary punctuation\n")
	builder.WriteString("8. Reference the attached artifact files for detailed execution data when available\n")

	return builder.String()
}

func (c *Coordinator) collectTaskSummaries(builder *strings.Builder, task *AiTask, depth int) {
	if task == nil {
		return
	}
	indent := strings.Repeat("  ", depth)

	if task.Index != "" {
		builder.WriteString(fmt.Sprintf("%s### Task %s: %s\n", indent, task.Index, task.Name))
	} else {
		builder.WriteString(fmt.Sprintf("%s### %s\n", indent, task.Name))
	}

	builder.WriteString(fmt.Sprintf("%s- Goal: %s\n", indent, task.Goal))
	builder.WriteString(fmt.Sprintf("%s- Status: %s\n", indent, task.GetStatus()))

	if summary := task.GetSummary(); summary != "" {
		builder.WriteString(fmt.Sprintf("%s- Summary: %s\n", indent, summary))
	}

	if task.LongSummary != "" && task.LongSummary != task.GetSummary() {
		builder.WriteString(fmt.Sprintf("%s- Detail: %s\n", indent, task.LongSummary))
	}

	toolResults := task.GetAllToolCallResults()
	if len(toolResults) > 0 {
		builder.WriteString(fmt.Sprintf("%s- Tool Calls: %d (success: %d, failed: %d)\n",
			indent, len(toolResults), task.GetSuccessCallCount(), task.GetFailCallCount()))
	}

	builder.WriteString("\n")

	for _, subtask := range task.Subtasks {
		c.collectTaskSummaries(builder, subtask, depth+1)
	}
}

func (c *Coordinator) collectTaskArtifactFiles() []string {
	workdir := ""
	if c.Workdir != "" {
		workdir = c.Workdir
	}
	if workdir == "" {
		workdir = c.GetOrCreateWorkDir()
	}
	if workdir == "" {
		workdir = consts.GetDefaultBaseHomeDir()
	}

	var files []string
	if c.rootTask == nil {
		return files
	}

	var collect func(task *AiTask)
	collect = func(task *AiTask) {
		if task == nil {
			return
		}

		taskIndex := task.Index
		if taskIndex == "" {
			taskIndex = "0"
		}
		taskDir := filepath.Join(workdir, aicommon.BuildTaskDirName(taskIndex, task.GetSemanticIdentifier()))
		safeTaskIndex := strings.ReplaceAll(taskIndex, "-", "_")

		summaryPath := filepath.Join(taskDir, fmt.Sprintf("task_%s_result_summary.txt", safeTaskIndex))
		if _, err := os.Stat(summaryPath); err == nil {
			files = append(files, summaryPath)
		}

		timelinePath := filepath.Join(taskDir, fmt.Sprintf("task_%s_timeline_diff.txt", safeTaskIndex))
		if _, err := os.Stat(timelinePath); err == nil {
			files = append(files, timelinePath)
		}

		for _, subtask := range task.Subtasks {
			collect(subtask)
		}
	}

	collect(c.rootTask)
	return files
}
