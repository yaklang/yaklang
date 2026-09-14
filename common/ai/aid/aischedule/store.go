package aischedule

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	DefaultGraceSeconds = int64(300)
	DefaultMaxRuntime   = int64(7200)
)

// NormalizeStartParams turns an interactive chat configuration into a safe
// unattended-run configuration. Scheduled runs cannot stop and wait for an
// approval, and request-scoped identifiers must never leak into future runs.
func NormalizeStartParams(params *ypb.AIStartParams) (*ypb.AIStartParams, error) {
	if params == nil {
		params = &ypb.AIStartParams{UseDefaultAIConfig: true}
	} else {
		params = proto.Clone(params).(*ypb.AIStartParams)
	}
	params.CoordinatorId = ""
	params.Sequence = 0
	params.UserQuery = ""
	params.TimelineSessionID = ""
	params.Attach = false
	params.PreferSessionCachedConfig = false
	params.Source = "ai"
	params.DisallowRequireForUserPrompt = true
	params.AllowPlanUserInteract = false
	// A schedule is an unattended execution boundary. Keeping the review
	// policy inherited from the chat can either block forever or, when a run is
	// restored, unexpectedly ask the user to approve a background tool call.
	// Permission/sandbox boundaries still apply; only interactive review is
	// disabled here.
	params.ReviewPolicy = "yolo"
	return params, nil
}

func MarshalPayload(payload *ypb.AIReActSchedulePayload) (string, string, error) {
	if payload == nil {
		return "", "", utils.Error("schedule payload is required")
	}
	params, err := NormalizeStartParams(payload.GetStartParams())
	if err != nil {
		return "", "", err
	}
	paramsRaw, err := protojson.Marshal(params)
	if err != nil {
		return "", "", utils.Errorf("marshal schedule start params failed: %v", err)
	}
	resourcesRaw, err := json.Marshal(payload.GetAttachedResourceInfos())
	if err != nil {
		return "", "", utils.Errorf("marshal schedule resources failed: %v", err)
	}
	return string(paramsRaw), string(resourcesRaw), nil
}

func UnmarshalPayload(record *schema.AIReActSchedule) (*ypb.AIReActSchedulePayload, error) {
	if record == nil {
		return nil, utils.Error("schedule is nil")
	}
	params := &ypb.AIStartParams{}
	if raw := strings.TrimSpace(record.StartParams); raw != "" {
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal([]byte(raw), params); err != nil {
			return nil, utils.Errorf("unmarshal schedule start params failed: %v", err)
		}
	}
	params, err := NormalizeStartParams(params)
	if err != nil {
		return nil, err
	}
	resources := make([]*ypb.AttachedResourceInfo, 0)
	if raw := strings.TrimSpace(record.AttachedResourceInfos); raw != "" {
		if err := json.Unmarshal([]byte(raw), &resources); err != nil {
			return nil, utils.Errorf("unmarshal schedule resources failed: %v", err)
		}
	}
	return &ypb.AIReActSchedulePayload{
		Prompt:                record.Prompt,
		StartParams:           params,
		AttachedResourceInfos: resources,
		FocusModeLoop:         record.FocusModeLoop,
	}, nil
}

// BuildRecord is the single validation and normalization path shared by the
// gRPC CRUD API and conversational ReAct tools.
func BuildRecord(input *ypb.AIReActSchedule, existing *schema.AIReActSchedule) (*schema.AIReActSchedule, error) {
	if input == nil || input.GetPayload() == nil || input.GetSchedule() == nil {
		return nil, utils.Error("schedule, payload and recurrence are required")
	}
	if input.GetSchedule().GetStartAt() <= 0 {
		return nil, utils.Error("schedule start time is required")
	}
	name := strings.TrimSpace(input.GetName())
	prompt := strings.TrimSpace(input.GetPayload().GetPrompt())
	if name == "" || prompt == "" {
		return nil, utils.Error("schedule name and prompt are required")
	}
	targetMode := strings.TrimSpace(input.GetTargetMode())
	if targetMode == "" {
		if strings.TrimSpace(input.GetTargetSessionID()) == "" {
			targetMode = schema.AIReActScheduleTargetNewSession
		} else {
			targetMode = schema.AIReActScheduleTargetContinueSession
		}
	}
	if targetMode != schema.AIReActScheduleTargetContinueSession && targetMode != schema.AIReActScheduleTargetNewSession {
		return nil, utils.Error("invalid schedule target mode")
	}
	targetSessionID := strings.TrimSpace(input.GetTargetSessionID())
	if targetMode == schema.AIReActScheduleTargetContinueSession && targetSessionID == "" {
		return nil, utils.Error("target session id is required when continuing a session")
	}
	if targetMode == schema.AIReActScheduleTargetNewSession {
		targetSessionID = ""
	}
	startAt := time.Unix(input.GetSchedule().GetStartAt(), 0).UTC()
	rule, err := Parse(input.GetSchedule().GetRRule(), input.GetSchedule().GetTimezone(), startAt)
	if err != nil {
		return nil, err
	}
	nextRunAt, hasNext := rule.Next(time.Now().Add(-time.Second))
	paramsRaw, resourcesRaw, err := MarshalPayload(input.GetPayload())
	if err != nil {
		return nil, err
	}
	status := strings.TrimSpace(input.GetStatus())
	if status == "" {
		status = schema.AIReActScheduleStatusActive
	}
	if status != schema.AIReActScheduleStatusActive && status != schema.AIReActScheduleStatusPaused {
		return nil, utils.Error("invalid schedule status")
	}
	misfire := input.GetMisfireGraceSeconds()
	if misfire <= 0 {
		misfire = DefaultGraceSeconds
	}
	maxRuntime := input.GetMaxRuntimeSeconds()
	if maxRuntime <= 0 {
		maxRuntime = DefaultMaxRuntime
	}
	if maxRuntime > 24*60*60 {
		return nil, utils.Error("max runtime cannot exceed 24 hours")
	}
	if existing == nil {
		existing = &schema.AIReActSchedule{UUID: uuid.NewString()}
	}
	originalRequest := strings.TrimSpace(input.GetOriginalRequest())
	if originalRequest == "" {
		originalRequest = strings.TrimSpace(existing.OriginalRequest)
	}
	if originalRequest == "" {
		// Manual/API-created schedules may not have a separate conversational
		// utterance. Falling back keeps provenance useful without inventing text.
		originalRequest = prompt
	}
	createdFromSessionID := strings.TrimSpace(input.GetCreatedFromSessionID())
	if createdFromSessionID == "" {
		createdFromSessionID = strings.TrimSpace(existing.CreatedFromSessionID)
	}
	existing.Name = name
	existing.Status = status
	existing.TargetMode = targetMode
	existing.TargetSessionID = targetSessionID
	existing.CreatedFromSessionID = createdFromSessionID
	existing.OriginalRequest = originalRequest
	existing.Prompt = prompt
	existing.StartParams = paramsRaw
	existing.AttachedResourceInfos = resourcesRaw
	existing.FocusModeLoop = strings.TrimSpace(input.GetPayload().GetFocusModeLoop())
	existing.RRule = NormalizeRRULE(input.GetSchedule().GetRRule())
	existing.Timezone = strings.TrimSpace(input.GetSchedule().GetTimezone())
	if existing.Timezone == "" {
		existing.Timezone = DefaultTimezone
	}
	existing.StartAt = startAt
	existing.NextRunAt = nil
	if hasNext {
		existing.NextRunAt = &nextRunAt
	} else {
		existing.Status = schema.AIReActScheduleStatusCompleted
	}
	existing.MisfireGraceSeconds = misfire
	existing.MaxRuntimeSeconds = maxRuntime
	existing.PauseReason = ""
	existing.LastError = ""
	return existing, nil
}

func GetRecord(db *gorm.DB, scheduleUUID string) (*schema.AIReActSchedule, error) {
	if db == nil {
		return nil, utils.Error("project database is unavailable")
	}
	scheduleUUID = strings.TrimSpace(scheduleUUID)
	if scheduleUUID == "" {
		return nil, utils.Error("schedule uuid is required")
	}
	record := &schema.AIReActSchedule{}
	if err := db.Where("uuid = ?", scheduleUUID).First(record).Error; err != nil {
		return nil, err
	}
	return record, nil
}

// CreateRecord persists a project-scoped schedule. The scheduler's regular
// one-second poll observes the new row, so a caller outside yakgrpc does not
// need access to the server wake channel.
func CreateRecord(db *gorm.DB, input *ypb.AIReActSchedule) (*schema.AIReActSchedule, error) {
	if db == nil {
		return nil, utils.Error("project database is unavailable")
	}
	record, err := BuildRecord(input, nil)
	if err != nil {
		return nil, err
	}
	if record.TargetMode == schema.AIReActScheduleTargetContinueSession {
		if _, err := yakit.GetAISessionMetaBySessionID(db, record.TargetSessionID); err != nil {
			return nil, utils.Errorf("target AI session does not exist: %v", err)
		}
	}
	if err := db.Create(record).Error; err != nil {
		return nil, err
	}
	return record, nil
}
