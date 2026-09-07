package scheduletools

import (
	"context"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aischedule"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const CreateScheduleToolName = "create_ai_react_schedule"

func localTimezoneName() string {
	if value := strings.TrimSpace(os.Getenv("TZ")); value != "" {
		if _, err := time.LoadLocation(value); err == nil {
			return value
		}
	}
	if name := strings.TrimSpace(time.Local.String()); name != "" && name != "Local" {
		return name
	}
	if target, err := os.Readlink("/etc/localtime"); err == nil {
		if index := strings.LastIndex(target, "zoneinfo/"); index >= 0 {
			name := strings.TrimPrefix(target[index:], "zoneinfo/")
			if _, err := time.LoadLocation(name); err == nil {
				return name
			}
		}
	}
	return "Local"
}

func recurrenceParts(rruleText string) map[string]string {
	result := make(map[string]string)
	raw := strings.TrimPrefix(strings.ToUpper(aischedule.NormalizeRRULE(rruleText)), "RRULE:")
	for _, item := range strings.Split(raw, ";") {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			result[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return result
}

func defaultFirstRun(rruleText string, now time.Time) time.Time {
	parts := recurrenceParts(rruleText)
	interval, _ := strconv.Atoi(parts["INTERVAL"])
	if interval <= 0 {
		interval = 1
	}
	switch parts["FREQ"] {
	case "MINUTELY":
		return now.Add(time.Duration(interval) * time.Minute).Truncate(time.Second)
	case "HOURLY":
		return now.Add(time.Duration(interval) * time.Hour).Truncate(time.Second)
	case "DAILY":
		return now.AddDate(0, 0, interval).Truncate(time.Second)
	case "WEEKLY":
		return now.AddDate(0, 0, 7*interval).Truncate(time.Second)
	case "MONTHLY":
		return now.AddDate(0, interval, 0).Truncate(time.Second)
	case "YEARLY":
		return now.AddDate(interval, 0, 0).Truncate(time.Second)
	default:
		return now.Add(time.Minute).Truncate(time.Second)
	}
}

func parseFirstRun(value, timezone, rruleText string, now time.Time) (time.Time, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, utils.Errorf("invalid timezone %q: %v", timezone, err)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultFirstRun(rruleText, now.In(location)), nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04"} {
		if parsed, err := time.ParseInLocation(layout, value, location); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, utils.Error("start_at must be RFC3339 or YYYY-MM-DD HH:mm in the selected timezone")
}

func createScheduleTool(now func() time.Time) (*aitool.Tool, error) {
	return aitool.New(
		CreateScheduleToolName,
		aitool.WithDescription("Create a durable future or recurring AI ReAct task. Invoke this only when the user explicitly asks to schedule, repeat, periodically run, remind, monitor, or execute something at a future time."),
		aitool.WithVerboseName("Create Scheduled AI Task"),
		aitool.WithVerboseNameZh("创建 AI 定时任务"),
		aitool.WithKeywords([]string{"schedule", "scheduled task", "recurring", "periodic", "定时任务", "周期任务", "每隔", "每天", "提醒"}),
		aitool.WithUsage(`Use this tool only for an explicit scheduling request. If either the work or timing is materially ambiguous, ask the user first.

Translate recurrence to RFC 5545 RRULE. Examples:
- every 5 minutes: RRULE:FREQ=MINUTELY;INTERVAL=5
- every hour: RRULE:FREQ=HOURLY;INTERVAL=1
- daily: RRULE:FREQ=DAILY;INTERVAL=1
- weekdays: RRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR
- once: RRULE:FREQ=DAILY;COUNT=1

task_prompt must contain only the work to perform at each run, without the scheduling phrase. For "每5分钟查询成都天气", use task_prompt "查询成都的最新天气情况". Omit start_at for interval requests to run first after one complete interval. Use an explicit start_at for wall-clock requests such as "每天9点".

Use continue_current_session when the task should reuse this chat's context and place results here. If that session is processing another turn when the occurrence fires, the occurrence is silently skipped. Use new_session_per_run when occurrences should be independent.`),
		aitool.WithStringParam("name",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("Short user-facing schedule name."),
		),
		aitool.WithStringParam("task_prompt",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("The exact work the AI should perform on each run. Exclude recurrence or timing instructions."),
		),
		aitool.WithStringParam("rrule",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("RFC 5545 recurrence rule, including the RRULE: prefix."),
		),
		aitool.WithStringParam("timezone",
			aitool.WithParam_Required(false),
			aitool.WithParam_Description("IANA timezone such as Asia/Shanghai. Defaults to the backend's local timezone."),
		),
		aitool.WithStringParam("start_at",
			aitool.WithParam_Required(false),
			aitool.WithParam_Description("First eligible run time in RFC3339, or YYYY-MM-DD HH:mm in timezone. For interval schedules, omit it to run first after one full interval."),
		),
		aitool.WithStringParam("target_mode",
			aitool.WithParam_Required(false),
			aitool.WithParam_Default("continue_current_session"),
			aitool.WithParam_Enum("continue_current_session", "new_session_per_run"),
			aitool.WithParam_Description("Continue this chat, or create an isolated conversation for every occurrence."),
		),
		aitool.WithCallback(func(ctx context.Context, params aitool.InvokeParams, runtimeConfig *aitool.ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error) {
			if runtimeConfig == nil || runtimeConfig.ProjectDatabase == nil {
				return nil, utils.Error("the current ReAct runtime has no project database")
			}
			name := strings.TrimSpace(params.GetString("name"))
			taskPrompt := strings.TrimSpace(params.GetString("task_prompt"))
			rruleText := strings.TrimSpace(params.GetString("rrule"))
			if name == "" || taskPrompt == "" || rruleText == "" {
				return nil, utils.Error("name, task_prompt and rrule are required")
			}

			timezone := strings.TrimSpace(params.GetString("timezone"))
			if timezone == "" {
				timezone = localTimezoneName()
			}
			firstRun, err := parseFirstRun(params.GetString("start_at"), timezone, rruleText, now())
			if err != nil {
				return nil, err
			}

			targetMode := schema.AIReActScheduleTargetContinueSession
			targetSessionID := strings.TrimSpace(runtimeConfig.PersistentSessionID)
			if params.GetString("target_mode") == "new_session_per_run" {
				targetMode = schema.AIReActScheduleTargetNewSession
				targetSessionID = ""
			} else if targetSessionID == "" {
				return nil, utils.Error("this chat has no persistent session id; use new_session_per_run or start the task from a saved chat")
			}

			startParams := &ypb.AIStartParams{UseDefaultAIConfig: true, ReviewPolicy: "yolo"}
			if runtimeConfig.PersistentSessionID != "" {
				if cached, err := yakit.GetAISessionMetaStartParamsBySessionID(runtimeConfig.ProjectDatabase, runtimeConfig.PersistentSessionID); err == nil && cached != nil {
					startParams = cached
				}
			}
			// Scheduled runs are unattended. The shared normalizer enforces this as
			// well, but setting it here keeps the stored intent explicit.
			startParams.ReviewPolicy = "yolo"
			originalRequest := strings.TrimSpace(runtimeConfig.CurrentTaskUserInput)
			if originalRequest == "" {
				originalRequest = taskPrompt
			}

			record, err := aischedule.CreateRecord(runtimeConfig.ProjectDatabase, &ypb.AIReActSchedule{
				Name:                 name,
				Status:               schema.AIReActScheduleStatusActive,
				TargetMode:           targetMode,
				TargetSessionID:      targetSessionID,
				CreatedFromSessionID: strings.TrimSpace(runtimeConfig.PersistentSessionID),
				OriginalRequest:      originalRequest,
				Payload: &ypb.AIReActSchedulePayload{
					Prompt:      taskPrompt,
					StartParams: startParams,
				},
				Schedule: &ypb.AIReActScheduleSpec{
					RRule:    rruleText,
					Timezone: timezone,
					StartAt:  firstRun.Unix(),
				},
			})
			if err != nil {
				return nil, err
			}
			result := map[string]any{
				"created":          true,
				"schedule_uuid":    record.UUID,
				"name":             record.Name,
				"original_request": record.OriginalRequest,
				"task_prompt":      record.Prompt,
				"rrule":            record.RRule,
				"timezone":         record.Timezone,
				"target_mode":      record.TargetMode,
			}
			if record.NextRunAt != nil {
				result["next_run_at"] = record.NextRunAt.In(firstRun.Location()).Format(time.RFC3339)
			}
			return result, nil
		}),
	)
}

func CreateScheduleTools() []*aitool.Tool {
	builders := []func() (*aitool.Tool, error){
		func() (*aitool.Tool, error) { return createScheduleTool(time.Now) },
		listSchedulesTool,
		updateSchedulesTool,
		setScheduleEnabledTool,
		deleteScheduleTool,
	}
	tools := make([]*aitool.Tool, 0, len(builders))
	for _, build := range builders {
		tool, err := build()
		if err != nil {
			log.Errorf("create AI schedule tool: %v", err)
			continue
		}
		tools = append(tools, tool)
	}
	return tools
}
