package scheduletools

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aischedule"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	ListSchedulesToolName      = "list_ai_react_schedules"
	UpdateScheduleToolName     = "update_ai_react_schedule"
	SetScheduleEnabledToolName = "set_ai_react_schedule_enabled"
	DeleteScheduleToolName     = "delete_ai_react_schedule"
)

func requireScheduleRuntime(runtimeConfig *aitool.ToolRuntimeConfig) error {
	if runtimeConfig == nil || runtimeConfig.ProjectDatabase == nil {
		return utils.Error("the current ReAct runtime has no project database")
	}
	return nil
}

func scheduleResult(record *schema.AIReActSchedule) map[string]any {
	result := map[string]any{
		"schedule_uuid":           record.UUID,
		"name":                    record.Name,
		"status":                  record.Status,
		"target_mode":             record.TargetMode,
		"target_session_id":       record.TargetSessionID,
		"created_from_session_id": record.CreatedFromSessionID,
		"original_request":        record.OriginalRequest,
		"task_prompt":             record.Prompt,
		"rrule":                   record.RRule,
		"timezone":                record.Timezone,
		"last_outcome":            record.LastOutcome,
		"last_skip_reason":        record.LastSkipReason,
		"last_error":              record.LastError,
	}
	if record.NextRunAt != nil {
		result["next_run_at"] = record.NextRunAt.Format(time.RFC3339)
	}
	if record.LastFinishedAt != nil {
		result["last_finished_at"] = record.LastFinishedAt.Format(time.RFC3339)
	}
	return result
}

func listSchedulesTool() (*aitool.Tool, error) {
	return aitool.New(
		ListSchedulesToolName,
		aitool.WithDescription("List scheduled AI tasks related to the current chat, including their durable prompts, next run and latest outcome."),
		aitool.WithVerboseName("List Scheduled AI Tasks"),
		aitool.WithVerboseNameZh("查看 AI 定时任务"),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithKeywords([]string{"scheduled tasks", "list schedules", "定时任务", "查看计划任务"}),
		aitool.WithBoolParam("include_all", aitool.WithParam_Default(false), aitool.WithParam_Description("List every project schedule instead of only schedules related to this chat.")),
		aitool.WithCallback(func(ctx context.Context, params aitool.InvokeParams, runtimeConfig *aitool.ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error) {
			if err := requireScheduleRuntime(runtimeConfig); err != nil {
				return nil, err
			}
			query := runtimeConfig.ProjectDatabase.Model(&schema.AIReActSchedule{})
			if !params.GetBool("include_all") {
				sessionID := strings.TrimSpace(runtimeConfig.PersistentSessionID)
				if sessionID == "" {
					return nil, utils.Error("the current chat has no persistent session id")
				}
				query = query.Where("target_session_id = ? OR created_from_session_id = ?", sessionID, sessionID)
			}
			var records []*schema.AIReActSchedule
			if err := query.Order("created_at DESC").Limit(100).Find(&records).Error; err != nil {
				return nil, err
			}
			items := make([]map[string]any, 0, len(records))
			for _, record := range records {
				items = append(items, scheduleResult(record))
			}
			return map[string]any{"count": len(items), "schedules": items}, nil
		}),
	)
}

func updateSchedulesTool() (*aitool.Tool, error) {
	return aitool.New(
		UpdateScheduleToolName,
		aitool.WithDescription("Update the durable work, name, recurrence, timezone, first run or destination of an existing scheduled AI task."),
		aitool.WithVerboseName("Update Scheduled AI Task"),
		aitool.WithVerboseNameZh("修改 AI 定时任务"),
		aitool.WithKeywords([]string{"update schedule", "edit scheduled task", "修改定时任务"}),
		aitool.WithStringParam("schedule_uuid", aitool.WithParam_Required(true), aitool.WithParam_Description("Schedule UUID.")),
		aitool.WithStringParam("name", aitool.WithParam_Required(false)),
		aitool.WithStringParam("task_prompt", aitool.WithParam_Required(false), aitool.WithParam_Description("Replacement durable work instruction without timing words.")),
		aitool.WithStringParam("rrule", aitool.WithParam_Required(false), aitool.WithParam_Description("Replacement RFC 5545 RRULE.")),
		aitool.WithStringParam("timezone", aitool.WithParam_Required(false)),
		aitool.WithStringParam("start_at", aitool.WithParam_Required(false), aitool.WithParam_Description("RFC3339 or YYYY-MM-DD HH:mm in timezone.")),
		aitool.WithStringParam("target_mode", aitool.WithParam_Required(false), aitool.WithParam_Enum("continue_current_session", "new_session_per_run")),
		aitool.WithCallback(func(ctx context.Context, params aitool.InvokeParams, runtimeConfig *aitool.ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error) {
			if err := requireScheduleRuntime(runtimeConfig); err != nil {
				return nil, err
			}
			record, err := aischedule.GetRecord(runtimeConfig.ProjectDatabase, params.GetString("schedule_uuid"))
			if err != nil {
				return nil, err
			}
			payload, err := aischedule.UnmarshalPayload(record)
			if err != nil {
				return nil, err
			}
			input := &ypb.AIReActSchedule{
				UUID: record.UUID, Name: record.Name, Status: record.Status,
				TargetMode: record.TargetMode, TargetSessionID: record.TargetSessionID,
				CreatedFromSessionID: record.CreatedFromSessionID, OriginalRequest: record.OriginalRequest,
				Payload:             payload,
				Schedule:            &ypb.AIReActScheduleSpec{RRule: record.RRule, Timezone: record.Timezone, StartAt: record.StartAt.Unix()},
				MisfireGraceSeconds: record.MisfireGraceSeconds, MaxRuntimeSeconds: record.MaxRuntimeSeconds,
			}
			if params.Has("name") {
				input.Name = strings.TrimSpace(params.GetString("name"))
			}
			if params.Has("task_prompt") {
				input.Payload.Prompt = strings.TrimSpace(params.GetString("task_prompt"))
			}
			if params.Has("rrule") {
				input.Schedule.RRule = strings.TrimSpace(params.GetString("rrule"))
			}
			if params.Has("timezone") {
				input.Schedule.Timezone = strings.TrimSpace(params.GetString("timezone"))
			}
			if params.Has("start_at") {
				startAt, err := parseFirstRun(params.GetString("start_at"), input.Schedule.Timezone, input.Schedule.RRule, time.Now())
				if err != nil {
					return nil, err
				}
				input.Schedule.StartAt = startAt.Unix()
			}
			if params.Has("target_mode") {
				switch params.GetString("target_mode") {
				case "new_session_per_run":
					input.TargetMode = schema.AIReActScheduleTargetNewSession
					input.TargetSessionID = ""
				case "continue_current_session":
					input.TargetMode = schema.AIReActScheduleTargetContinueSession
					input.TargetSessionID = strings.TrimSpace(runtimeConfig.PersistentSessionID)
					if input.TargetSessionID == "" {
						return nil, utils.Error("the current chat has no persistent session id")
					}
				default:
					return nil, utils.Error("invalid target_mode")
				}
			}
			updated, err := aischedule.BuildRecord(input, record)
			if err != nil {
				return nil, err
			}
			if updated.TargetMode == schema.AIReActScheduleTargetContinueSession {
				if _, err := yakit.GetAISessionMetaBySessionID(runtimeConfig.ProjectDatabase, updated.TargetSessionID); err != nil {
					return nil, utils.Errorf("target AI session does not exist: %v", err)
				}
			}
			if err := runtimeConfig.ProjectDatabase.Save(updated).Error; err != nil {
				return nil, err
			}
			return map[string]any{"updated": true, "schedule": scheduleResult(updated)}, nil
		}),
	)
}

func setScheduleEnabledTool() (*aitool.Tool, error) {
	return aitool.New(
		SetScheduleEnabledToolName,
		aitool.WithDescription("Pause future triggers or resume an existing scheduled AI task. Pausing does not cancel an execution that has already started."),
		aitool.WithVerboseName("Pause or Resume Scheduled AI Task"),
		aitool.WithVerboseNameZh("暂停或恢复 AI 定时任务"),
		aitool.WithKeywords([]string{"pause schedule", "resume schedule", "暂停定时任务", "恢复定时任务"}),
		aitool.WithStringParam("schedule_uuid", aitool.WithParam_Required(true)),
		aitool.WithBoolParam("enabled", aitool.WithParam_Required(true)),
		aitool.WithCallback(func(ctx context.Context, params aitool.InvokeParams, runtimeConfig *aitool.ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error) {
			if err := requireScheduleRuntime(runtimeConfig); err != nil {
				return nil, err
			}
			record, err := aischedule.GetRecord(runtimeConfig.ProjectDatabase, params.GetString("schedule_uuid"))
			if err != nil {
				return nil, err
			}
			if params.GetBool("enabled") {
				rule, err := aischedule.Parse(record.RRule, record.Timezone, record.StartAt)
				if err != nil {
					return nil, err
				}
				next, ok := rule.Next(time.Now().Add(-time.Second))
				if ok {
					record.Status, record.NextRunAt = schema.AIReActScheduleStatusActive, &next
				} else {
					record.Status, record.NextRunAt = schema.AIReActScheduleStatusCompleted, nil
				}
				record.PauseReason = ""
			} else {
				record.Status = schema.AIReActScheduleStatusPaused
				record.PauseReason = "paused by user"
			}
			if err := runtimeConfig.ProjectDatabase.Save(record).Error; err != nil {
				return nil, err
			}
			return map[string]any{"updated": true, "schedule": scheduleResult(record)}, nil
		}),
	)
}

func deleteScheduleTool() (*aitool.Tool, error) {
	return aitool.New(
		DeleteScheduleToolName,
		aitool.WithDescription("Permanently delete a scheduled AI task so it cannot trigger again. An execution that has already started continues."),
		aitool.WithVerboseName("Delete Scheduled AI Task"),
		aitool.WithVerboseNameZh("删除 AI 定时任务"),
		aitool.WithKeywords([]string{"delete schedule", "remove scheduled task", "删除定时任务"}),
		aitool.WithStringParam("schedule_uuid", aitool.WithParam_Required(true)),
		aitool.WithCallback(func(ctx context.Context, params aitool.InvokeParams, runtimeConfig *aitool.ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error) {
			if err := requireScheduleRuntime(runtimeConfig); err != nil {
				return nil, err
			}
			record, err := aischedule.GetRecord(runtimeConfig.ProjectDatabase, params.GetString("schedule_uuid"))
			if err != nil {
				return nil, err
			}
			result := runtimeConfig.ProjectDatabase.Delete(record)
			if result.Error != nil {
				return nil, result.Error
			}
			return map[string]any{"deleted": result.RowsAffected > 0, "schedule_uuid": record.UUID, "name": record.Name}, nil
		}),
	)
}
