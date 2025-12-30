package yakgrpc

import (
	"archive/zip"
	"context"
	"encoding/json"
	"os"

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
	outputPath := req.GetOutputPath()

	// If no output path provided, create a temp file
	if outputPath == "" {
		tmpFile, err := os.CreateTemp("", "ai-logs-*.zip")
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
			var events []*schema.AiOutputEvent
			if len(coordinatorIDs) > 0 {
				if err := db.Where("coordinator_id IN (?)", coordinatorIDs).Find(&events).Error; err != nil {
					return nil, utils.Errorf("failed to query events: %v", err)
				}
				dataToExport["AIOutputEvent"] = events
			}

		case "memory":
			// Memory uses SessionID
			var memories []*schema.AIMemoryEntity
			if sessionID != "" {
				if err := db.Where("session_id = ?", sessionID).Find(&memories).Error; err != nil {
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
