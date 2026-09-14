package sessionruntime

import (
	"context"
	"encoding/json"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const attachedRecoveryHistoryBlockLimit = 20

func sendAttachedRecoveryHistory(
	ctx context.Context,
	db *gorm.DB,
	send func(*schema.AiOutputEvent) error,
	persistentSession string,
	event *ypb.AIInputEvent,
) error {
	if db == nil {
		return sendAttachedSyncError(send, "recovery_history", utils.Errorf("db is nil"), event.GetSyncID())
	}

	sessionID := persistentSession
	startID := int64(0)
	limit := attachedRecoveryHistoryBlockLimit

	if raw := event.GetSyncJsonInput(); raw != "" {
		var params map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &params); err != nil {
			return sendAttachedSyncError(send, "recovery_history", utils.Errorf("failed to parse recovery history params: %v", err), event.GetSyncID())
		}
		if sid := utils.InterfaceToString(params["session_id"]); sid != "" {
			sessionID = sid
		}
		if rawStartID, ok := params["start_id"]; ok {
			startID = int64(utils.InterfaceToInt(rawStartID))
		}
		if rawLimit, ok := params["limit"]; ok {
			if parsedLimit := utils.InterfaceToInt(rawLimit); parsedLimit > 0 {
				limit = parsedLimit
			}
		}
	}

	if sessionID == "" {
		return sendAttachedSyncError(send, "recovery_history", utils.Errorf("session_id is empty"), event.GetSyncID())
	}

	eventCh, result, err := yakit.YieldAIEventRecoveryHistory(ctx, db, sessionID, startID, limit)
	if err != nil {
		return sendAttachedSyncError(send, "recovery_history", err, event.GetSyncID())
	}
	for recoveredEvent := range eventCh {
		if recoveredEvent == nil {
			continue
		}
		recoveredEvent.IsSync = true
		if err := send(recoveredEvent); err != nil {
			return err
		}
	}

	return sendAttachedSyncJSON(send, schema.EVENT_TYPE_STRUCTURED, "recovery_history", map[string]interface{}{
		"session_id":         sessionID,
		"requested_start_id": startID,
		"block_count":        result.BlockCount,
		"event_count":        result.EventCount,
		"next_start_id":      result.NextStartID,
		"has_more":           result.HasMore,
	}, event.GetSyncID())
}

func sendAttachedSyncError(send func(*schema.AiOutputEvent) error, nodeID string, err error, syncID string) error {
	if err == nil {
		return nil
	}
	return sendAttachedSyncJSON(send, schema.EVENT_TYPE_STRUCTURED, nodeID, map[string]any{
		"error": err.Error(),
	}, syncID)
}

func sendAttachedSyncJSON(send func(*schema.AiOutputEvent) error, eventType schema.EventType, nodeID string, payload any, syncID string) error {
	return send(&schema.AiOutputEvent{
		Type:      eventType,
		NodeId:    nodeID,
		IsJson:    true,
		IsSync:    true,
		Content:   utils.Jsonify(payload),
		Timestamp: time.Now().Unix(),
		SyncID:    syncID,
	})
}
