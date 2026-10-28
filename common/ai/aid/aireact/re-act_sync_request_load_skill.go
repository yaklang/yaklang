package aireact

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// re-act_sync_request_load_skill.go 实现「用户强制加载 SKILL」sync 事件处理.
//
// 客户端发起:
//
//	AIInputEvent{
//	  IsSyncMessage: true,
//	  SyncType:      "load_skill_sync",
//	  SyncJsonInput: `{"skill_names": ["x","y"], "task_id": "<可选>"}`,
//	  SyncID:        "<回调用>",
//	}
//
// 服务端处理:
//  1. 解析 skill_names.
//  2. 逐个走 SkillsContextManager.LoadForcedSkill (来源标记 user-forced, 正文进 Timeline).
//  3. 每个成功加载的 skill 在 timeline 写 "user_loaded_skill" 条目 (用户要求明确记录).
//  4. 成功后 SubmitSkillHit(name, "user_force") 累计命中反馈.
//  5. EmitSyncJSON 同步返回 loaded/failed/already_loaded.
//  6. EmitSessionSnapshot 刷新前端能力清单.
//
// 关键词: load_skill sync, 用户强制加载, timeline user_loaded_skill, frozen_block

// loadSkillRequest 是 load_skill sync 事件的请求体.
type loadSkillRequest struct {
	SkillNames []string `json:"skill_names"`
	TaskID     string   `json:"task_id,omitempty"`
}

func parseLoadSkillRequest(event *ypb.AIInputEvent) loadSkillRequest {
	var req loadSkillRequest
	if event == nil || strings.TrimSpace(event.SyncJsonInput) == "" {
		return req
	}
	if err := json.Unmarshal([]byte(event.SyncJsonInput), &req); err != nil {
		return req
	}
	return req
}

// skillManagerForSessionInput 保留活跃 loop 的专用 loader；空闲会话使用 config
// 的 manager，手动加载和热加载因此不再依赖 loop 的生命周期。
func (r *ReAct) skillManagerForSessionInput() *aiskillloader.SkillsContextManager {
	if loop := r.GetCurrentLoop(); loop != nil {
		if mgr := loop.GetSkillsContextManager(); mgr != nil {
			return mgr
		}
	}
	return r.config.GetSkillsContextManager()
}

// HandleSyncTypeLoadSkillEvent 处理用户强制加载 SKILL 的 sync 事件.
func (r *ReAct) HandleSyncTypeLoadSkillEvent(event *ypb.AIInputEvent) error {
	req := parseLoadSkillRequest(event)

	loaded := make([]string, 0)
	failed := make(map[string]string, 0)
	alreadyForced := make([]string, 0)

	loop := r.GetCurrentLoop()
	mgr := r.skillManagerForSessionInput()
	if mgr == nil {
		_, _ = r.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "load_skill", map[string]interface{}{
			"loaded":         loaded,
			"failed":         failed,
			"already_loaded": alreadyForced,
			"error":          "skills context manager not configured",
		}, event.SyncID)
		return nil
	}

	for _, name := range req.SkillNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		added, err := mgr.LoadForcedSkill(name)
		if err != nil {
			failed[name] = err.Error()
			// 失败也写 timeline, 复用既有 entryType.
			r.AddToTimeline("skill_load_failed", fmt.Sprintf("User forced load FAILED: %s (%v)", name, err))
			continue
		}
		if !added {
			// 已是 forced: 幂等, 计入 already_loaded.
			alreadyForced = append(alreadyForced, name)
			continue
		}
		loaded = append(loaded, name)
		// 用户要求: timeline 明确写出用户加载了什么 SKILL.
		r.AddToTimeline("user_loaded_skill", fmt.Sprintf("User forced load: %s", name))
		// 命中反馈: 用户强制加载是高价值反馈点.
		aicommon.SubmitSkillHit(r.config, name, aicommon.StatsSourceSkillUserForce)
	}

	response := map[string]interface{}{
		"loaded":         loaded,
		"failed":         failed,
		"already_loaded": alreadyForced,
	}
	_, _ = r.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "load_skill", response, event.SyncID)

	// 刷新前端能力清单 (forced skill 正文已进 Timeline).
	reactloops.EmitSessionSnapshot(r.config, loop, r.GetCurrentTask())
	return nil
}
