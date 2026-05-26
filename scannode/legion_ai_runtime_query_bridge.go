package scannode

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
	"google.golang.org/protobuf/proto"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

const (
	defaultAIRuntimeQueryLimit = 50
	maxAIRuntimeQueryLimit     = 200
)

func (b *legionJobBridge) handleAIHTTPFlowsQuery(ctx context.Context, raw []byte) error {
	var command aiv1.QueryAIHTTPFlowsCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai http flows query command: %w", err)
	}

	ref := aiRuntimeQueryRefFromHTTPFlowsCommand(&command)
	if err := validateAIHTTPFlowsQueryCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIHTTPFlowsQueryFailed(
			ctx,
			ref,
			"invalid_ai_http_flows_query_command",
			err.Error(),
		)
	}

	items, pagination, total, err := queryAIHTTPFlows(&command)
	if err != nil {
		return b.ensureAIPublisher().PublishAIHTTPFlowsQueryFailed(
			ctx,
			ref,
			"ai_http_flows_query_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIHTTPFlowsQueried(ctx, ref, items, pagination, total)
}

func (b *legionJobBridge) handleAIRisksQuery(ctx context.Context, raw []byte) error {
	var command aiv1.QueryAIRisksCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai risks query command: %w", err)
	}

	ref := aiRuntimeQueryRefFromRisksCommand(&command)
	if err := validateAIRisksQueryCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIRisksQueryFailed(
			ctx,
			ref,
			"invalid_ai_risks_query_command",
			err.Error(),
		)
	}

	items, pagination, total, err := queryAIRisks(&command)
	if err != nil {
		return b.ensureAIPublisher().PublishAIRisksQueryFailed(
			ctx,
			ref,
			"ai_risks_query_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIRisksQueried(ctx, ref, items, pagination, total)
}

func validateAIHTTPFlowsQueryCommand(nodeID string, command *aiv1.QueryAIHTTPFlowsCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai http flows query metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai http flows query command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai http flows query target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai http flows query target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai http flows query owner_user_id is required")
	case strings.TrimSpace(command.GetRuntimeId()) == "" && strings.TrimSpace(command.GetHiddenIndex()) == "":
		return fmt.Errorf("ai http flows query runtime_id or hidden_index is required")
	default:
		return nil
	}
}

func validateAIRisksQueryCommand(nodeID string, command *aiv1.QueryAIRisksCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai risks query metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai risks query command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai risks query target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai risks query target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai risks query owner_user_id is required")
	case strings.TrimSpace(command.GetRuntimeId()) == "" && command.GetRiskId() == 0:
		return fmt.Errorf("ai risks query runtime_id or risk_id is required")
	default:
		return nil
	}
}

func queryAIHTTPFlows(command *aiv1.QueryAIHTTPFlowsCommand) ([]*aiv1.AIHTTPFlowRecord, *aiv1.AIRuntimePagination, int64, error) {
	page, limit, offset := normalizeAIRuntimeQueryPagination(command.GetPagination())
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, nil, 0, fmt.Errorf("project database is not ready")
	}

	query := db.Model(&schema.HTTPFlow{})
	query = buildAIHTTPFlowQuery(query, command)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, nil, 0, err
	}

	var flows []*schema.HTTPFlow
	if err := query.Order("id desc").Offset(offset).Limit(limit).Find(&flows).Error; err != nil && !gorm.IsRecordNotFoundError(err) {
		return nil, nil, 0, err
	}

	items := make([]*aiv1.AIHTTPFlowRecord, 0, len(flows))
	for _, flow := range flows {
		if flow == nil {
			continue
		}
		raw, err := json.Marshal(flow)
		if err != nil {
			return nil, nil, 0, err
		}
		items = append(items, &aiv1.AIHTTPFlowRecord{
			Id:          int64(flow.ID),
			RuntimeId:   flow.RuntimeId,
			HiddenIndex: flow.HiddenIndex,
			RawJson:     raw,
		})
	}
	return items, &aiv1.AIRuntimePagination{Page: int64(page), Limit: int64(limit)}, total, nil
}

func queryAIRisks(command *aiv1.QueryAIRisksCommand) ([]*aiv1.AIRiskRecord, *aiv1.AIRuntimePagination, int64, error) {
	page, limit, offset := normalizeAIRuntimeQueryPagination(command.GetPagination())
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, nil, 0, fmt.Errorf("project database is not ready")
	}

	query := db.Model(&schema.Risk{})
	query = buildAIRiskQuery(query, command)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, nil, 0, err
	}

	var risks []*schema.Risk
	if err := query.Order("id desc").Offset(offset).Limit(limit).Find(&risks).Error; err != nil && !gorm.IsRecordNotFoundError(err) {
		return nil, nil, 0, err
	}

	items := make([]*aiv1.AIRiskRecord, 0, len(risks))
	for _, risk := range risks {
		if risk == nil {
			continue
		}
		raw, err := json.Marshal(risk)
		if err != nil {
			return nil, nil, 0, err
		}
		items = append(items, &aiv1.AIRiskRecord{
			Id:        uint64(risk.ID),
			RuntimeId: risk.RuntimeId,
			Title:     risk.Title,
			RawJson:   raw,
		})
	}
	return items, &aiv1.AIRuntimePagination{Page: int64(page), Limit: int64(limit)}, total, nil
}

func buildAIHTTPFlowQuery(query *gorm.DB, command *aiv1.QueryAIHTTPFlowsCommand) *gorm.DB {
	if runtimeID := strings.TrimSpace(command.GetRuntimeId()); runtimeID != "" {
		query = query.Where("runtime_id = ?", runtimeID)
	}
	if hiddenIndex := strings.TrimSpace(command.GetHiddenIndex()); hiddenIndex != "" {
		query = query.Where("hidden_index = ?", hiddenIndex)
	}
	if method := strings.TrimSpace(command.GetMethod()); method != "" {
		query = query.Where("method = ?", method)
	}
	if statusCode := command.GetStatusCode(); statusCode > 0 {
		query = query.Where("status_code = ?", statusCode)
	}
	if contentType := strings.TrimSpace(command.GetContentType()); contentType != "" {
		query = query.Where("content_type LIKE ?", likePattern(contentType))
	}
	if keyword := strings.TrimSpace(command.GetKeyword()); keyword != "" {
		pattern := likePattern(keyword)
		query = query.Where(
			"url LIKE ? OR host LIKE ? OR request LIKE ? OR response LIKE ? OR payload LIKE ?",
			pattern,
			pattern,
			pattern,
			pattern,
			pattern,
		)
	}
	return query
}

func buildAIRiskQuery(query *gorm.DB, command *aiv1.QueryAIRisksCommand) *gorm.DB {
	if runtimeID := strings.TrimSpace(command.GetRuntimeId()); runtimeID != "" {
		query = query.Where("runtime_id = ?", runtimeID)
	}
	if riskID := command.GetRiskId(); riskID > 0 {
		query = query.Where("id = ?", riskID)
	}
	if severity := strings.TrimSpace(command.GetSeverity()); severity != "" {
		query = query.Where("severity = ?", severity)
	}
	if riskType := strings.TrimSpace(command.GetRiskType()); riskType != "" {
		query = query.Where("risk_type = ? OR risk_type_verbose = ?", riskType, riskType)
	}
	if network := strings.TrimSpace(command.GetNetwork()); network != "" {
		pattern := likePattern(network)
		query = query.Where("ip LIKE ? OR host LIKE ? OR url LIKE ?", pattern, pattern, pattern)
	}
	if title := strings.TrimSpace(command.GetTitle()); title != "" {
		pattern := likePattern(title)
		query = query.Where("title LIKE ? OR title_verbose LIKE ?", pattern, pattern)
	}
	if keyword := strings.TrimSpace(command.GetKeyword()); keyword != "" {
		pattern := likePattern(keyword)
		query = query.Where(
			"title LIKE ? OR title_verbose LIKE ? OR url LIKE ? OR ip LIKE ? OR host LIKE ? OR description LIKE ? OR solution LIKE ? OR details LIKE ? OR payload LIKE ? OR quoted_request LIKE ? OR quoted_response LIKE ?",
			pattern,
			pattern,
			pattern,
			pattern,
			pattern,
			pattern,
			pattern,
			pattern,
			pattern,
			pattern,
			pattern,
		)
	}
	return query
}

func likePattern(value string) string {
	return "%" + strings.TrimSpace(value) + "%"
}

func normalizeAIRuntimeQueryPagination(pagination *aiv1.AIRuntimePagination) (int, int, int) {
	limit := normalizeAIRuntimeQueryLimit(pagination)
	page := 1
	if pagination != nil && pagination.GetPage() > 0 {
		page = int(pagination.GetPage())
	}
	return page, limit, (page - 1) * limit
}

func normalizeAIRuntimeQueryLimit(pagination *aiv1.AIRuntimePagination) int {
	if pagination == nil || pagination.GetLimit() <= 0 {
		return defaultAIRuntimeQueryLimit
	}
	if pagination.GetLimit() > maxAIRuntimeQueryLimit {
		return maxAIRuntimeQueryLimit
	}
	return int(pagination.GetLimit())
}

func aiRuntimeQueryRefFromHTTPFlowsCommand(command *aiv1.QueryAIHTTPFlowsCommand) aiRuntimeQueryCommandRef {
	return aiRuntimeQueryCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiRuntimeQueryRefFromRisksCommand(command *aiv1.QueryAIRisksCommand) aiRuntimeQueryCommandRef {
	return aiRuntimeQueryCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}
