package yakgrpc

import (
	"archive/zip"
	"context"
	"encoding/json"
	"os"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httptrace"
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
			var finalCoordinatorIDs []string

			// If sessionID exists, query coordinatorIDs from AIEvent table
			if sessionID != "" {
				page := 1
				pageSize := 100
				coordinatorIDMap := make(map[string]bool)

				for {
					var events []*schema.AiOutputEvent
					offset := (page - 1) * pageSize
					if err := db.Where("session_id = ?", sessionID).
						Offset(offset).
						Limit(pageSize).
						Find(&events).Error; err != nil {
						return nil, utils.Errorf("failed to query events by sessionID: %v", err)
					}

					if len(events) == 0 {
						break
					}

					for _, event := range events {
						if event.CoordinatorId != "" {
							coordinatorIDMap[event.CoordinatorId] = true
						}
					}

					// If we got fewer results than pageSize, we've reached the end
					if len(events) < pageSize {
						break
					}

					page++
				}

				// Convert map keys to slice
				for id := range coordinatorIDMap {
					finalCoordinatorIDs = append(finalCoordinatorIDs, id)
				}
			} else if len(coordinatorIDs) > 0 {
				// Use provided coordinatorIDs if sessionID is not set
				finalCoordinatorIDs = coordinatorIDs
			}

			// Query checkpoints with collected coordinatorIDs
			if len(finalCoordinatorIDs) > 0 {
				if err := db.Where("coordinator_uuid IN (?)", finalCoordinatorIDs).Find(&checkpoints).Error; err != nil {
					return nil, utils.Errorf("failed to query checkpoints: %v", err)
				}
				dataToExport["AICheckpoints"] = checkpoints
			}

		case "output_event":
			// Export persisted records directly. The protobuf round trip used to
			// discard session/model/recovery metadata and database timestamps.
			var events []*schema.AiOutputEvent
			query := db.Order("id ASC")
			if sessionID != "" {
				query = query.Where("session_id = ?", sessionID)
			} else if len(coordinatorIDs) > 0 {
				query = query.Where("coordinator_id IN (?)", coordinatorIDs)
			} else {
				continue
			}
			if err := query.Find(&events).Error; err != nil {
				return nil, utils.Errorf("failed to export events: %v", err)
			}
			dataToExport["AIOutputEvent"] = events

		case "memory":
			var memories []*schema.AIMemoryEntity
			// If sessionID exists, query by sessionID; otherwise use memoryID
			if sessionID != "" {
				if err := db.Where("session_id = ?", sessionID).Find(&memories).Error; err != nil {
					return nil, utils.Errorf("failed to query memories by sessionID: %v", err)
				}
				dataToExport["memory"] = memories
			} else if memoryID != "" {
				if err := db.Where("memory_id = ?", memoryID).Find(&memories).Error; err != nil {
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
	outFile, err := os.OpenFile(outputPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
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

	for _, kind := range exportTypes {
		if kind == "output_event" || kind == "network" {
			if _, err := httptrace.Export(ctx, zipWriter, "", sessionID, coordinatorIDs); err != nil {
				return nil, utils.Errorf("export network debug: %v", err)
			}
			break
		}
	}
	if err := zipWriter.Close(); err != nil {
		return nil, err
	}
	if err := outFile.Close(); err != nil {
		return nil, err
	}
	return &ypb.ExportAILogsResponse{
		FilePath: outputPath,
	}, nil
}
