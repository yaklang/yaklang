package scannode

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	ssav1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ssa/v1"
)

// Legacy subjects select JSON explicitly. Never try this codec after a v2
// decode error: the same command must not execute twice during an upgrade.
const (
	legionCommandLegacySSADebugQuery      = "ssa.debug.query"
	legionCommandLegacySSALogTail         = "ssa.log.tail"
	legionCommandLegacySSAIRProgramDelete = "ssa.ir_program.delete"
	legacySSADebugResultPrefix            = "legion.realtime.ssa.debug.result"
	legacySSALogTailResultPrefix          = "legion.realtime.ssa.log.tail"
	legacySSAIRProgramDeleteResultPrefix  = "legion.realtime.ssa.ir_program.delete"
)

type legacySSALogTailCommand struct {
	QueryID    string `json:"query_id"`
	JobID      string `json:"job_id"`
	AttemptID  string `json:"attempt_id"`
	Offset     int64  `json:"offset"`
	MaxBytes   int64  `json:"max_bytes"`
	TaskStatus string `json:"task_status,omitempty"`
	LogKind    string `json:"log_kind,omitempty"`
}

// Historical diagnostic replies correlate through the unique reply subject;
// unlike v2, their JSON bodies do not contain query_id.
type legacySSALogTailResponse struct {
	Found      bool   `json:"found"`
	Reason     string `json:"reason,omitempty"`
	TotalBytes int64  `json:"total_bytes"`
	Offset     int64  `json:"offset"`
	HasMore    bool   `json:"has_more"`
	Content    string `json:"content,omitempty"`
}

type legacySSAIRProgramDeleteCommand struct {
	RequestID     string `json:"request_id"`
	ProgramName   string `json:"program_name"`
	StoreIdentity string `json:"store_identity"`
}

type legacySSAIRProgramDeleteResponse struct {
	RequestID     string `json:"request_id"`
	ProgramName   string `json:"program_name"`
	StoreIdentity string `json:"store_identity"`
	Success       bool   `json:"success"`
	Deleted       bool   `json:"deleted"`
	AlreadyAbsent bool   `json:"already_absent,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

func (b *legionJobBridge) handleLegacySSADebugQuery(ctx context.Context, raw []byte) error {
	var request ssaDebugQueryPayload
	if err := json.Unmarshal(raw, &request); err != nil {
		return fmt.Errorf("unmarshal legacy ssa debug query: %w", err)
	}
	return b.handleSSADebugQueryPayload(ctx, request, func(ctx context.Context, id string, response ssaDebugQueryResponse) error {
		return b.publishLegacySSAResponse(ctx, legacySSADebugResultPrefix+"."+id, response)
	})
}

func (b *legionJobBridge) handleLegacySSALogTail(ctx context.Context, raw []byte) error {
	var request legacySSALogTailCommand
	if err := json.Unmarshal(raw, &request); err != nil {
		return fmt.Errorf("unmarshal legacy ssa log tail: %w", err)
	}
	// JSON escaping can expand a byte to six bytes. Keep this response below
	// NATS' 1 MiB limit even for control characters in a task log.
	const legacyMaxBytes = 128 * 1024
	if request.MaxBytes > legacyMaxBytes {
		request.MaxBytes = legacyMaxBytes
	}
	return b.handleSSALogTailPayload(ctx, &ssav1.LogTailCommand{
		QueryId: request.QueryID, JobId: request.JobID, AttemptId: request.AttemptID,
		Offset: request.Offset, MaxBytes: request.MaxBytes, TaskStatus: request.TaskStatus, LogKind: request.LogKind,
	}, func(ctx context.Context, id string, response *ssav1.LogTailResult) error {
		return b.publishLegacySSAResponse(ctx, legacySSALogTailResultPrefix+"."+id, legacySSALogTailResponse{
			Found: response.Found, Reason: response.Reason, TotalBytes: response.TotalBytes,
			Offset: response.Offset, HasMore: response.HasMore, Content: response.Content,
		})
	})
}

func (b *legionJobBridge) handleLegacySSAIRProgramDelete(ctx context.Context, raw []byte) error {
	var request legacySSAIRProgramDeleteCommand
	if err := json.Unmarshal(raw, &request); err != nil {
		return fmt.Errorf("unmarshal legacy ssa IR program delete: %w", err)
	}
	return handleSSAIRProgramDeleteRequest(ctx, &ssav1.DeleteIRProgramCommand{
		RequestId: request.RequestID, ProgramName: request.ProgramName, StoreIdentity: request.StoreIdentity,
	}, b.ssaIRProgramDeleter(), func(ctx context.Context, id string, response *ssav1.DeleteIRProgramResult) error {
		return b.publishLegacySSAResponse(ctx, legacySSAIRProgramDeleteResultPrefix+"."+id, legacySSAIRProgramDeleteResponse{
			RequestID: response.RequestId, ProgramName: response.ProgramName, StoreIdentity: response.StoreIdentity,
			Success: response.Success, Deleted: response.Deleted, AlreadyAbsent: response.AlreadyAbsent, Reason: response.Reason,
		})
	})
}

func (b *legionJobBridge) publishLegacySSAResponse(ctx context.Context, subject string, response any) error {
	raw, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("marshal legacy SSA response: %w", err)
	}
	if b == nil {
		return fmt.Errorf("capability event publisher is not ready")
	}
	publisher, ok := b.capabilityPublisher.(*capabilityEventPublisher)
	if !ok || publisher == nil {
		return fmt.Errorf("capability event publisher is not ready")
	}
	if b.agent != nil && b.agent.node != nil {
		ctx = b.agent.node.GetRootContext()
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return publisher.PublishRaw(ctx, subject, raw)
}
