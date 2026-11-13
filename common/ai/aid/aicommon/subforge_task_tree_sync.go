package aicommon

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/schema"
	"sort"
)

// Keep only the newest projection per child invocation. Stream the rows so a
// long session does not retain every intermediate tree in memory.
func (c *Config) subForgeTaskTreesForSession(sessionID string) ([]json.RawMessage, error) {
	rows, err := c.GetDB().Model(&schema.AiOutputEvent{}).Select("content").Where("session_id = ? AND type = ?", sessionID, schema.EVENT_TYPE_SUB_FORGE_TASK_TREE).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type projection struct {
		Child    string `json:"child_coordinator_id"`
		Revision uint64 `json:"revision"`
	}
	revisions := map[string]uint64{}
	latest := map[string]json.RawMessage{}
	for rows.Next() {
		var content []byte
		if err := rows.Scan(&content); err != nil {
			return nil, err
		}
		var snapshot projection
		if json.Unmarshal(content, &snapshot) != nil || snapshot.Child == "" || snapshot.Revision <= revisions[snapshot.Child] {
			continue
		}
		revisions[snapshot.Child] = snapshot.Revision
		latest[snapshot.Child] = json.RawMessage(content)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(latest))
	for id := range latest {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]json.RawMessage, 0, len(ids))
	for _, id := range ids {
		result = append(result, latest[id])
	}
	return result, nil
}
