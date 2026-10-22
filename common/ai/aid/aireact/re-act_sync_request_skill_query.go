package aireact

import (
	"encoding/json"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type skillDetailRequest struct {
	SkillName string `json:"skill_name"`
	Name      string `json:"name"`
}

func parseSkillDetailRequest(event *ypb.AIInputEvent) string {
	if event == nil || strings.TrimSpace(event.SyncJsonInput) == "" {
		return ""
	}
	var req skillDetailRequest
	if err := json.Unmarshal([]byte(event.SyncJsonInput), &req); err != nil {
		return ""
	}
	if name := strings.TrimSpace(req.SkillName); name != "" {
		return name
	}
	return strings.TrimSpace(req.Name)
}

// HandleSyncTypeSkillInventoryEvent 返回 session 当前已加载和可加载的 SKILL。
// 查询不要求存在活跃 task/loop。
func (r *ReAct) HandleSyncTypeSkillInventoryEvent(event *ypb.AIInputEvent) error {
	mgr := r.skillManagerForSessionInput()
	items := aicommon.BuildSessionSkillInventory(mgr)
	_, _ = r.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "skill_inventory", items, event.SyncID)
	return nil
}

// HandleSyncTypeSkillDetailEvent 返回指定 SKILL 的详情。已加载内容以 Timeline
// 可见分片为准；未加载内容直接读取 SKILL.md。
func (r *ReAct) HandleSyncTypeSkillDetailEvent(event *ypb.AIInputEvent) error {
	name := parseSkillDetailRequest(event)
	detail, err := aicommon.BuildSessionSkillDetail(r.skillManagerForSessionInput(), name)
	if err != nil {
		_, _ = r.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "skill_detail", map[string]any{
			"name":  name,
			"error": err.Error(),
		}, event.SyncID)
		return nil
	}
	_, _ = r.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "skill_detail", detail, event.SyncID)
	return nil
}
