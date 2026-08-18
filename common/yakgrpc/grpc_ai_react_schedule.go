package yakgrpc

import (
	"context"
	"strings"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aischedule"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	defaultAIReActScheduleGraceSeconds = aischedule.DefaultGraceSeconds
	defaultAIReActScheduleRuntime      = aischedule.DefaultMaxRuntime
)

func unixOrZero(value *time.Time) int64 {
	if value == nil || value.IsZero() {
		return 0
	}
	return value.Unix()
}

func normalizeScheduleStartParams(params *ypb.AIStartParams) (*ypb.AIStartParams, error) {
	return aischedule.NormalizeStartParams(params)
}

func marshalSchedulePayload(payload *ypb.AIReActSchedulePayload) (string, string, error) {
	return aischedule.MarshalPayload(payload)
}

func unmarshalSchedulePayload(record *schema.AIReActSchedule) (*ypb.AIReActSchedulePayload, error) {
	return aischedule.UnmarshalPayload(record)
}

func scheduleToGRPC(record *schema.AIReActSchedule) (*ypb.AIReActSchedule, error) {
	if record == nil {
		return nil, utils.Error("schedule is nil")
	}
	payload, err := unmarshalSchedulePayload(record)
	if err != nil {
		return nil, err
	}
	return &ypb.AIReActSchedule{
		Id:                   int64(record.ID),
		UUID:                 record.UUID,
		Name:                 record.Name,
		Status:               record.Status,
		TargetMode:           record.TargetMode,
		TargetSessionID:      record.TargetSessionID,
		Payload:              payload,
		Schedule:             &ypb.AIReActScheduleSpec{RRule: record.RRule, Timezone: record.Timezone, StartAt: record.StartAt.Unix()},
		NextRunAt:            unixOrZero(record.NextRunAt),
		LastRunAt:            unixOrZero(record.LastRunAt),
		MisfireGraceSeconds:  record.MisfireGraceSeconds,
		MaxRuntimeSeconds:    record.MaxRuntimeSeconds,
		PauseReason:          record.PauseReason,
		LastError:            record.LastError,
		CreatedAt:            record.CreatedAt.Unix(),
		UpdatedAt:            record.UpdatedAt.Unix(),
		OriginalRequest:      record.OriginalRequest,
		CreatedFromSessionID: record.CreatedFromSessionID,
		LastOutcome:          record.LastOutcome,
		LastSkipReason:       record.LastSkipReason,
		LastStartedAt:        unixOrZero(record.LastStartedAt),
		LastFinishedAt:       unixOrZero(record.LastFinishedAt),
	}, nil
}

func buildScheduleRecord(input *ypb.AIReActSchedule, existing *schema.AIReActSchedule) (*schema.AIReActSchedule, error) {
	return aischedule.BuildRecord(input, existing)
}

func getAIReActScheduleRecord(db *gorm.DB, scheduleUUID string) (*schema.AIReActSchedule, error) {
	return aischedule.GetRecord(db, scheduleUUID)
}

func (s *Server) CreateAIReActSchedule(ctx context.Context, req *ypb.CreateAIReActScheduleRequest) (*ypb.AIReActSchedule, error) {
	record, err := aischedule.CreateRecord(s.GetProjectDatabase(), req.GetSchedule())
	if err != nil {
		return nil, err
	}
	s.ensureAIReActScheduler()
	s.wakeAIReActScheduler()
	return scheduleToGRPC(record)
}

func (s *Server) UpdateAIReActSchedule(ctx context.Context, req *ypb.UpdateAIReActScheduleRequest) (*ypb.AIReActSchedule, error) {
	if req.GetSchedule() == nil {
		return nil, utils.Error("schedule is required")
	}
	existing, err := getAIReActScheduleRecord(s.GetProjectDatabase(), req.GetSchedule().GetUUID())
	if err != nil {
		return nil, err
	}
	record, err := buildScheduleRecord(req.GetSchedule(), existing)
	if err != nil {
		return nil, err
	}
	if record.TargetMode == schema.AIReActScheduleTargetContinueSession {
		if _, err := yakit.GetAISessionMetaBySessionID(s.GetProjectDatabase(), record.TargetSessionID); err != nil {
			return nil, utils.Errorf("target AI session does not exist: %v", err)
		}
	}
	if err := s.GetProjectDatabase().Save(record).Error; err != nil {
		return nil, err
	}
	s.ensureAIReActScheduler()
	s.wakeAIReActScheduler()
	return scheduleToGRPC(record)
}

func (s *Server) DeleteAIReActSchedule(ctx context.Context, req *ypb.DeleteAIReActScheduleRequest) (*ypb.DbOperateMessage, error) {
	record, err := getAIReActScheduleRecord(s.GetProjectDatabase(), req.GetUUID())
	if err != nil {
		return nil, err
	}
	db := s.GetProjectDatabase().Delete(record)
	if db.Error != nil {
		return nil, db.Error
	}
	s.cancelAIReActScheduleExecution(record.UUID)
	s.wakeAIReActScheduler()
	return &ypb.DbOperateMessage{TableName: record.TableName(), Operation: DbOperationDelete, EffectRows: db.RowsAffected}, nil
}

func (s *Server) GetAIReActSchedule(ctx context.Context, req *ypb.GetAIReActScheduleRequest) (*ypb.AIReActSchedule, error) {
	record, err := getAIReActScheduleRecord(s.GetProjectDatabase(), req.GetUUID())
	if err != nil {
		return nil, err
	}
	return scheduleToGRPC(record)
}

func filterAIReActSchedules(db *gorm.DB, filter *ypb.AIReActScheduleFilter) *gorm.DB {
	db = db.Model(&schema.AIReActSchedule{})
	if filter == nil {
		return db
	}
	if len(filter.GetUUIDs()) > 0 {
		db = db.Where("uuid IN (?)", filter.GetUUIDs())
	}
	if len(filter.GetStatus()) > 0 {
		db = db.Where("status IN (?)", filter.GetStatus())
	}
	if keyword := strings.TrimSpace(filter.GetKeyword()); keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("name LIKE ? OR prompt LIKE ? OR original_request LIKE ?", like, like, like)
	}
	if len(filter.GetTargetSessionIDs()) > 0 {
		db = db.Where("target_session_id IN (?)", filter.GetTargetSessionIDs())
	}
	if len(filter.GetTargetModes()) > 0 {
		db = db.Where("target_mode IN (?)", filter.GetTargetModes())
	}
	if len(filter.GetCreatedFromSessionIDs()) > 0 {
		db = db.Where("created_from_session_id IN (?)", filter.GetCreatedFromSessionIDs())
	}
	return db
}

func (s *Server) QueryAIReActSchedules(ctx context.Context, req *ypb.QueryAIReActSchedulesRequest) (*ypb.QueryAIReActSchedulesResponse, error) {
	if req == nil {
		req = &ypb.QueryAIReActSchedulesRequest{}
	}
	paging := req.GetPagination()
	if paging == nil {
		paging = &ypb.Paging{Page: 1, Limit: 30, OrderBy: "created_at", Order: "desc"}
	}
	query := filterAIReActSchedules(s.GetProjectDatabase(), req.GetFilter())
	query = bizhelper.OrderByPaging(query, paging)
	records := make([]*schema.AIReActSchedule, 0)
	paginator, result := bizhelper.Paging(query, int(paging.GetPage()), int(paging.GetLimit()), &records)
	if result.Error != nil {
		return nil, result.Error
	}
	data := make([]*ypb.AIReActSchedule, 0, len(records))
	for _, record := range records {
		item, err := scheduleToGRPC(record)
		if err != nil {
			return nil, err
		}
		data = append(data, item)
	}
	return &ypb.QueryAIReActSchedulesResponse{
		Pagination: &ypb.Paging{Page: int64(paginator.Page), Limit: int64(paginator.Limit), OrderBy: paging.GetOrderBy(), Order: paging.GetOrder()},
		Data:       data,
		Total:      int64(paginator.TotalRecord),
	}, nil
}

func (s *Server) SetAIReActScheduleEnabled(ctx context.Context, req *ypb.SetAIReActScheduleEnabledRequest) (*ypb.AIReActSchedule, error) {
	record, err := getAIReActScheduleRecord(s.GetProjectDatabase(), req.GetUUID())
	if err != nil {
		return nil, err
	}
	if req.GetEnabled() {
		rule, err := aischedule.Parse(record.RRule, record.Timezone, record.StartAt)
		if err != nil {
			return nil, err
		}
		next, ok := rule.Next(time.Now().Add(-time.Second))
		if !ok {
			record.Status = schema.AIReActScheduleStatusCompleted
			record.NextRunAt = nil
		} else {
			record.Status = schema.AIReActScheduleStatusActive
			record.NextRunAt = &next
		}
		record.PauseReason = ""
	} else {
		record.Status = schema.AIReActScheduleStatusPaused
		record.PauseReason = "paused by user"
	}
	if err := s.GetProjectDatabase().Save(record).Error; err != nil {
		return nil, err
	}
	if !req.GetEnabled() {
		s.cancelAIReActScheduleExecution(record.UUID)
	}
	s.ensureAIReActScheduler()
	s.wakeAIReActScheduler()
	return scheduleToGRPC(record)
}

func (s *Server) PreviewAIReActScheduleTimes(ctx context.Context, req *ypb.PreviewAIReActScheduleTimesRequest) (*ypb.PreviewAIReActScheduleTimesResponse, error) {
	if req.GetSchedule() == nil {
		return nil, utils.Error("schedule is required")
	}
	if req.GetSchedule().GetStartAt() <= 0 {
		return nil, utils.Error("schedule start time is required")
	}
	startAt := time.Unix(req.GetSchedule().GetStartAt(), 0).UTC()
	rule, err := aischedule.Parse(req.GetSchedule().GetRRule(), req.GetSchedule().GetTimezone(), startAt)
	if err != nil {
		return nil, err
	}
	after := time.Now().Add(-time.Second)
	if req.GetAfterTimestamp() > 0 {
		after = time.Unix(req.GetAfterTimestamp(), 0)
	}
	times := rule.Preview(after, int(req.GetCount()))
	result := make([]int64, 0, len(times))
	for _, item := range times {
		result = append(result, item.Unix())
	}
	return &ypb.PreviewAIReActScheduleTimesResponse{Timestamps: result}, nil
}

func (s *Server) RunAIReActScheduleNow(ctx context.Context, req *ypb.RunAIReActScheduleNowRequest) (*ypb.Empty, error) {
	record, err := getAIReActScheduleRecord(s.GetProjectDatabase(), req.GetUUID())
	if err != nil {
		return nil, err
	}
	s.ensureAIReActScheduler()
	if err := s.enqueueAIReActSchedule(record, time.Now().UTC(), aiReActScheduleTriggerManual); err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}
