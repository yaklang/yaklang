package aivizhttp

import (
	"net/http"
	"sort"

	"github.com/gorilla/mux"
	"github.com/yaklang/yaklang/common/schema"
)

// handleSessionTimeline 返回按 task_index 分组的事件时间线
// GET /sessions/{sessionId}/timeline
func (s *VizHTTPServer) handleSessionTimeline(w http.ResponseWriter, r *http.Request) {
	sessionID := mux.Vars(r)["sessionId"]
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}

	// Only select columns the timeline needs — skip stream_delta BLOB to cut
	// memory on large sessions. eventToItem may reference stream_delta for
	// display, but it is rarely populated for persisted events.
	var events []*schema.AiOutputEvent
	s.db.Select("id, created_at, updated_at, deleted_at, coordinator_id, type, node_id, task_id, task_uuid, task_index, call_tool_id, content, is_stream, is_reason, timestamp, session_id, ai_model_name, ai_service, task_semantic_label, is_recovery_block, recovery_index_id").
		Where("session_id = ?", sessionID).Order("id asc").Find(&events)

	// 按 task_index 分组
	taskMap := make(map[string]*TimelineTask)
	taskOrder := make([]string, 0)

	for _, e := range events {
		if e == nil {
			continue
		}
		taskIdx := e.TaskIndex
		if taskIdx == "" {
			taskIdx = "default"
		}

		task, exists := taskMap[taskIdx]
		if !exists {
			task = &TimelineTask{
				TaskIndex: taskIdx,
				TaskId:    e.TaskId,
				Events:    make([]EventItem, 0),
			}
			taskMap[taskIdx] = task
			taskOrder = append(taskOrder, taskIdx)
		}
		task.Events = append(task.Events, eventToItem(e))
		task.EventCount++
	}

	// 输出
	result := make([]TimelineTask, 0, len(taskOrder))
	for _, idx := range taskOrder {
		result = append(result, *taskMap[idx])
	}

	// 按 task_index 升序排列
	sort.Slice(result, func(i, j int) bool {
		return result[i].TaskIndex < result[j].TaskIndex
	})

	writeJSON(w, http.StatusOK, TimelineResponse{
		Tasks: result,
		Total: len(result),
	})
}
