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

type skillInventoryRequest struct {
	Query    string `json:"query"`
	Keyword  string `json:"keyword"`
	Q        string `json:"q"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Limit    int    `json:"limit"`
}

func parseSkillInventoryRequest(event *ypb.AIInputEvent) (query string, page, pageSize int) {
	if event == nil || strings.TrimSpace(event.SyncJsonInput) == "" {
		return "", 0, 0
	}
	var req skillInventoryRequest
	if err := json.Unmarshal([]byte(event.SyncJsonInput), &req); err != nil {
		return "", 0, 0
	}
	query = strings.TrimSpace(req.Query)
	if query == "" {
		query = strings.TrimSpace(req.Keyword)
	}
	if query == "" {
		query = strings.TrimSpace(req.Q)
	}
	pageSize = req.PageSize
	if pageSize <= 0 {
		pageSize = req.Limit
	}
	return query, req.Page, pageSize
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

// HandleSyncTypeSkillLoadedEvent 返回全部已加载 SKILL，不占用可加载清单分页。
func (r *ReAct) HandleSyncTypeSkillLoadedEvent(event *ypb.AIInputEvent) error {
	inventory := aicommon.BuildSessionLoadedSkillInventory(r.skillManagerForSessionInput())
	_, _ = r.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "skill_loaded", inventory, event.SyncID)
	return nil
}

// HandleSyncTypeSkillAvailableEvent 分页返回可加载且未加载的 SKILL。
// query、keyword、q 任一非空时，只返回名称或描述匹配的未加载技能。
func (r *ReAct) HandleSyncTypeSkillAvailableEvent(event *ypb.AIInputEvent) error {
	query, page, pageSize := parseSkillInventoryRequest(event)
	inventory, err := aicommon.BuildSessionAvailableSkillInventory(r.skillManagerForSessionInput(), query, page, pageSize)
	if err != nil {
		_, _ = r.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "skill_available", map[string]any{
			"skills": []any{},
			"query":  query,
			"error":  err.Error(),
		}, event.SyncID)
		return nil
	}
	_, _ = r.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "skill_available", inventory, event.SyncID)
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
	detail.URL = aicommon.BuildSessionSkillResourceURL(r.config, name)
	_, _ = r.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "skill_detail", detail, event.SyncID)
	return nil
}
