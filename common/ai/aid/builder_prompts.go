package aid

import (
	_ "embed"
)

//go:embed prompts/plan-to-task-list.txt
var __prompt_GenerateTaskListPrompt string

//go:embed prompts/task-execute.txt
var __prompt_ExecuteTaskPromptTemplate string

//go:embed prompts/tool-param-schema.txt
var __prompt_ToolParamSchemaPromptTemplate string

//go:embed prompts/tool-result-to-decision.txt
var __prompt_ToolResultToDecisionPromptTemplate string

//go:embed prompts/tool-result-history.txt
var __prompt_ToolResultHistoryPromptTemplate string

//go:embed prompts/task-summary.txt
var __prompt_TaskSummary string

//go:embed prompts/report-finished.txt
var __prompt_ReportFinished string

//go:embed prompts/dynamic-plan.txt
var __prompt_DynamicPlan string

//go:embed prompts/current_task_info.txt
var __prompt_currentTaskInfo string

//go:embed prompts/tools-list.txt
var __prompt_ToolsList string

//go:embed prompts/plan-review/plan-incomplete.txt
var planReviewPrompts string

//go:embed prompts/plan-review/plan-create-subtask.txt
var planReviewCreateSubtaskPrompts string
