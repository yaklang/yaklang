package yakgrpc

import (
	"archive/zip"
	"context"
	"encoding/json"
	"os"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) ExportAILogs(ctx context.Context, req *ypb.ExportAILogsRequest) (*ypb.ExportAILogsResponse, error) {
	db := s.GetProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("database not initialized")
	}

	sessionID := req.GetSessionID()
	coordinatorIDs := req.GetCoordinatorIDs()
	exportTypes := req.GetExportDataTypes()
	memoryID := req.GetMemoryID()
	outputPath := req.GetOutputPath()

	if memoryID == "" {
		memoryID = "default"
	}
	// If no output path provided, create a temp file
	if outputPath == "" {
		tmpFile, err := os.CreateTemp(consts.GetDefaultYakitBaseTempDir(), "ai-logs-*.zip")
		if err != nil {
			return nil, utils.Errorf("failed to create temp file: %v", err)
		}
		tmpFile.Close()
		outputPath = tmpFile.Name()
	}

	// Prepare data to export
	dataToExport := make(map[string]interface{})

	for _, dataType := range exportTypes {
		switch dataType {
		case "checkpoints":
			var checkpoints []*schema.AiCheckpoint
			if len(coordinatorIDs) > 0 {
				if err := db.Where("coordinator_uuid IN (?)", coordinatorIDs).Find(&checkpoints).Error; err != nil {
					return nil, utils.Errorf("failed to query checkpoints: %v", err)
				}
				dataToExport["AICheckpoints"] = checkpoints
			}

		case "output_event":
			if len(coordinatorIDs) > 0 {
				queryEventRsp, err := s.QueryAIEvent(ctx, &ypb.AIEventQueryRequest{
					Filter: &ypb.AIEventFilter{
						CoordinatorId: coordinatorIDs,
					},
				})
				if err != nil {
					log.Errorf("failed to query events: %v", err)
					continue
				}
				events := lo.Map(queryEventRsp.Events, func(item *ypb.AIOutputEvent, _ int) *schema.AiOutputEvent {
					return &schema.AiOutputEvent{
						CoordinatorId:   item.CoordinatorId,
						Type:            schema.EventType(item.Type),
						NodeId:          item.NodeId,
						IsSystem:        item.IsSystem,
						IsStream:        item.IsStream,
						IsReason:        item.IsReason,
						IsSync:          item.IsSync,
						StreamDelta:     item.StreamDelta,
						IsJson:          item.IsJson,
						Content:         item.Content,
						Timestamp:       item.Timestamp,
						TaskIndex:       item.TaskIndex,
						DisableMarkdown: item.DisableMarkdown,
						SyncID:          item.SyncID,
						EventUUID:       item.EventUUID,
						CallToolID:      item.CallToolID,
						ContentType:     item.ContentType,
						AIService:       item.AIService,
						TaskUUID:        item.TaskUUID,
						AIModelName:     item.AIModelName,
					}
				})
				dataToExport["AIOutputEvent"] = events
			}

		case "memory":
			// Memory uses memoryID
			var memories []*schema.AIMemoryEntity
			if memoryID != "" {
				if err := db.Where("session_id = ?", memoryID).Find(&memories).Error; err != nil {
					return nil, utils.Errorf("failed to query memories: %v", err)
				}
				dataToExport["memory"] = memories
			}

		case "timeline":
			var timelines []*schema.AIAgentRuntime
			if sessionID != "" {
				if err := db.Where("persistent_session = ?", sessionID).Find(&timelines).Error; err != nil {
					return nil, utils.Errorf("failed to query timelines: %v", err)
				}
				dataToExport["timeline"] = timelines
			}
		}
	}

	// Create Zip file
	outFile, err := os.Create(outputPath)
	if err != nil {
		return nil, utils.Errorf("failed to create zip file: %v", err)
	}
	defer outFile.Close()

	zipWriter := zip.NewWriter(outFile)
	defer zipWriter.Close()

	for name, data := range dataToExport {
		w, err := zipWriter.Create(name + ".json")
		if err != nil {
			return nil, utils.Errorf("failed to create zip entry %s: %v", name, err)
		}

		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(data); err != nil {
			return nil, utils.Errorf("failed to encode %s: %v", name, err)
		}
	}

	return &ypb.ExportAILogsResponse{
		FilePath: outputPath,
	}, nil
}
