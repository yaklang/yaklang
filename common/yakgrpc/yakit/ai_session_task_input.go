package yakit

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	defaultAISessionTaskInputPageSize = 30
	maxAISessionTaskInputPageSize     = 200
)

// AISessionTaskResource is the transport-friendly representation persisted for
// resources attached by one input.
type AISessionTaskResource struct {
	Type  string `json:"type"`
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
}

type SaveAISessionTaskInputParams struct {
	SessionID       string
	TaskID          string
	TaskUUID        string
	UserInputUUID   string
	UserInput       string
	InputSource     string
	FocusModeLoop   string
	ScheduleUUID    string
	ScheduleName    string
	ScheduledAt     string
	ScheduleTrigger string
	Resources       []AISessionTaskResource
}

type AISessionTaskInputFilter struct {
	SessionID     string
	TaskID        string
	TaskUUID      string
	UserInputUUID string
	InputSource   string
}

type AISessionCurrentResources struct {
	SessionID        string                  `json:"session_id"`
	TaskID           string                  `json:"task_id"`
	UserInputUUID    string                  `json:"user_input_uuid,omitempty"`
	TaskResources    []AISessionTaskResource `json:"task_resources"`
	SessionResources []AISessionTaskResource `json:"session_resources"`
}

func marshalAISessionTaskJSON(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func unmarshalAISessionTaskResources(raw string) ([]AISessionTaskResource, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []AISessionTaskResource{}, nil
	}
	var resources []AISessionTaskResource
	if err := json.Unmarshal([]byte(raw), &resources); err != nil {
		return nil, err
	}
	return resources, nil
}

// SaveAISessionTaskInput persists one task input snapshot. task_uuid is a
// single-column idempotency key; no composite uniqueness or foreign key is used.
func SaveAISessionTaskInput(db *gorm.DB, params *SaveAISessionTaskInputParams) error {
	if db == nil {
		return utils.Error("database is nil")
	}
	if !db.HasTable(&schema.AISessionTaskInput{}) {
		return nil
	}
	if params == nil {
		return utils.Error("AI session task input is nil")
	}
	sessionID := strings.TrimSpace(params.SessionID)
	taskID := strings.TrimSpace(params.TaskID)
	taskUUID := strings.TrimSpace(params.TaskUUID)
	if sessionID == "" {
		return utils.Error("session_id is empty")
	}
	if taskID == "" {
		return utils.Error("task_id is empty")
	}
	if taskUUID == "" {
		return utils.Error("task_uuid is empty")
	}

	resources, err := marshalAISessionTaskJSON(params.Resources)
	if err != nil {
		return utils.Errorf("marshal AI session task resources failed: %v", err)
	}

	record := &schema.AISessionTaskInput{
		SessionID:       sessionID,
		TaskID:          taskID,
		TaskUUID:        taskUUID,
		UserInputUUID:   strings.TrimSpace(params.UserInputUUID),
		UserInput:       params.UserInput,
		InputSource:     strings.TrimSpace(params.InputSource),
		FocusModeLoop:   strings.TrimSpace(params.FocusModeLoop),
		ScheduleUUID:    strings.TrimSpace(params.ScheduleUUID),
		ScheduleName:    strings.TrimSpace(params.ScheduleName),
		ScheduledAt:     strings.TrimSpace(params.ScheduledAt),
		ScheduleTrigger: strings.TrimSpace(params.ScheduleTrigger),
		Resources:       resources,
	}
	updates := map[string]any{
		"session_id":       record.SessionID,
		"task_id":          record.TaskID,
		"user_input_uuid":  record.UserInputUUID,
		"user_input":       record.UserInput,
		"input_source":     record.InputSource,
		"focus_mode_loop":  record.FocusModeLoop,
		"schedule_uuid":    record.ScheduleUUID,
		"schedule_name":    record.ScheduleName,
		"scheduled_at":     record.ScheduledAt,
		"schedule_trigger": record.ScheduleTrigger,
		"resources":        record.Resources,
	}
	query := bizhelper.ExactQueryString(db.Model(&schema.AISessionTaskInput{}), "task_uuid", taskUUID)
	if result := query.Assign(updates).FirstOrCreate(record); result.Error != nil {
		return utils.Errorf("save AI session task input failed: %v", result.Error)
	}
	return nil
}

func FilterAISessionTaskInputs(db *gorm.DB, filter *AISessionTaskInputFilter) *gorm.DB {
	db = db.Model(&schema.AISessionTaskInput{})
	if filter == nil {
		return db
	}
	if value := strings.TrimSpace(filter.SessionID); value != "" {
		db = bizhelper.ExactQueryString(db, "session_id", value)
	}
	if value := strings.TrimSpace(filter.TaskID); value != "" {
		db = bizhelper.ExactQueryString(db, "task_id", value)
	}
	if value := strings.TrimSpace(filter.TaskUUID); value != "" {
		db = bizhelper.ExactQueryString(db, "task_uuid", value)
	}
	if value := strings.TrimSpace(filter.UserInputUUID); value != "" {
		db = bizhelper.ExactQueryString(db, "user_input_uuid", value)
	}
	if value := strings.TrimSpace(filter.InputSource); value != "" {
		db = bizhelper.ExactQueryString(db, "input_source", value)
	}
	return db
}

func normalizeAISessionTaskInputPaging(paging *ypb.Paging) *ypb.Paging {
	if paging == nil {
		return &ypb.Paging{Page: 1, Limit: defaultAISessionTaskInputPageSize, OrderBy: "id", Order: "desc"}
	}
	cloned := &ypb.Paging{
		Page:     paging.GetPage(),
		Limit:    paging.GetLimit(),
		OrderBy:  paging.GetOrderBy(),
		Order:    paging.GetOrder(),
		RawOrder: paging.GetRawOrder(),
		BeforeId: paging.GetBeforeId(),
		AfterId:  paging.GetAfterId(),
	}
	if cloned.Page <= 0 {
		cloned.Page = 1
	}
	if cloned.Limit <= 0 {
		cloned.Limit = defaultAISessionTaskInputPageSize
	}
	if cloned.Limit > maxAISessionTaskInputPageSize {
		cloned.Limit = maxAISessionTaskInputPageSize
	}
	if strings.TrimSpace(cloned.OrderBy) == "" {
		cloned.OrderBy = "id"
		cloned.Order = "desc"
	}
	return cloned
}

func QueryAISessionTaskInputs(db *gorm.DB, filter *AISessionTaskInputFilter, paging *ypb.Paging) (*bizhelper.Paginator, []*schema.AISessionTaskInput, error) {
	if db == nil {
		return nil, nil, utils.Error("database is nil")
	}
	if !db.HasTable(&schema.AISessionTaskInput{}) {
		return &bizhelper.Paginator{}, []*schema.AISessionTaskInput{}, nil
	}
	paging = normalizeAISessionTaskInputPaging(paging)
	query := FilterAISessionTaskInputs(db, filter)
	query = bizhelper.OrderByPaging(query, paging)
	records := make([]*schema.AISessionTaskInput, 0)
	paginator, result := bizhelper.YakitPagingQuery(query, paging, &records)
	if result.Error != nil {
		return nil, nil, utils.Errorf("query AI session task inputs failed: %v", result.Error)
	}
	return paginator, records, nil
}

func GetLatestAISessionTaskInput(db *gorm.DB, sessionID string) (*schema.AISessionTaskInput, error) {
	if db == nil {
		return nil, utils.Error("database is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, utils.Error("session_id is empty")
	}
	if !db.HasTable(&schema.AISessionTaskInput{}) {
		return nil, nil
	}
	var record schema.AISessionTaskInput
	query := bizhelper.ExactQueryString(db.Model(&schema.AISessionTaskInput{}), "session_id", sessionID)
	query = bizhelper.QueryOrder(query, "id", "desc")
	if err := query.First(&record).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func GetAISessionTaskInputByTaskUUID(db *gorm.DB, taskUUID string) (*schema.AISessionTaskInput, error) {
	if db == nil {
		return nil, utils.Error("database is nil")
	}
	taskUUID = strings.TrimSpace(taskUUID)
	if taskUUID == "" {
		return nil, utils.Error("task_uuid is empty")
	}
	if !db.HasTable(&schema.AISessionTaskInput{}) {
		return nil, nil
	}
	var record schema.AISessionTaskInput
	query := bizhelper.ExactQueryString(db.Model(&schema.AISessionTaskInput{}), "task_uuid", taskUUID)
	if err := query.First(&record).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func collectAISessionResourceHistory(records []*schema.AISessionTaskInput) ([]AISessionTaskResource, error) {
	resources := make([]AISessionTaskResource, 0)
	seen := make(map[string]struct{})
	// Walk inputs newest to oldest while preserving resource order within each
	// input.
	for i := len(records) - 1; i >= 0; i-- {
		record := records[i]
		if record == nil {
			continue
		}
		recordResources, err := unmarshalAISessionTaskResources(record.Resources)
		if err != nil {
			return nil, utils.Errorf("unmarshal resources for task %s failed: %v", record.TaskUUID, err)
		}
		for _, resource := range recordResources {
			key := resource.Type + "\x00" + resource.Key + "\x00" + resource.Value
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			resources = append(resources, resource)
		}
	}
	return resources, nil
}

// GetAISessionCurrentResources returns the resources attached by the latest
// input and the distinct resources attached across the session's input history.
// Resources from newer inputs come first; order within an input is preserved.
func GetAISessionCurrentResources(db *gorm.DB, sessionID string) (*AISessionCurrentResources, error) {
	if db == nil {
		return nil, utils.Error("database is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, utils.Error("session_id is empty")
	}
	if !db.HasTable(&schema.AISessionTaskInput{}) {
		return nil, nil
	}

	var records []*schema.AISessionTaskInput
	query := db.Model(&schema.AISessionTaskInput{}).
		Select("id, session_id, task_id, task_uuid, user_input_uuid, resources")
	query = bizhelper.ExactQueryString(query, "session_id", sessionID)
	query = bizhelper.QueryOrder(query, "id", "asc")
	if err := query.Find(&records).Error; err != nil {
		return nil, utils.Errorf("query AI session resource history failed: %v", err)
	}
	if len(records) == 0 {
		return nil, nil
	}

	sessionResources, err := collectAISessionResourceHistory(records)
	if err != nil {
		return nil, err
	}
	latest := records[len(records)-1]
	taskResources, err := unmarshalAISessionTaskResources(latest.Resources)
	if err != nil {
		return nil, utils.Errorf("unmarshal resources for task %s failed: %v", latest.TaskUUID, err)
	}
	return &AISessionCurrentResources{
		SessionID:        latest.SessionID,
		TaskID:           latest.TaskID,
		UserInputUUID:    latest.UserInputUUID,
		TaskResources:    taskResources,
		SessionResources: sessionResources,
	}, nil
}

func YieldAISessionTaskInputs(ctx context.Context, db *gorm.DB, filter *AISessionTaskInputFilter, opts ...bizhelper.YieldModelOpts) chan *schema.AISessionTaskInput {
	query := FilterAISessionTaskInputs(db, filter)
	query = bizhelper.QueryOrder(query, "id", "desc")
	return bizhelper.YieldModel[*schema.AISessionTaskInput](ctx, query, opts...)
}

func DeleteAISessionTaskInputsBySessionID(db *gorm.DB, sessionID string) error {
	if db == nil {
		return utils.Error("database is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return utils.Error("session_id is empty")
	}
	if !db.HasTable(&schema.AISessionTaskInput{}) {
		return nil
	}
	query := bizhelper.ExactQueryString(db.Unscoped(), "session_id", sessionID)
	return query.Delete(&schema.AISessionTaskInput{}).Error
}

func DeleteAllAISessionTaskInputs(db *gorm.DB) error {
	if db == nil {
		return utils.Error("database is nil")
	}
	if !db.HasTable(&schema.AISessionTaskInput{}) {
		return nil
	}
	return db.Unscoped().Delete(&schema.AISessionTaskInput{}).Error
}
