package auditlog

import "encoding/json"

const LogAgentAPI_QueryAuditLog = "query-auditlog"

type QueryAuditLogRequest struct {
	Start      int64  `json:"start"`
	End        int64  `json:"end"`
	Types      string `json:"types"`
	Page       int    `json:"page"`
	Limit      int    `json:"limit"`
	ServerAddr string `json:"server_addr"`
}

type QueryAuditLogResponse struct {
	Total     int         `json:"total"`
	Page      int         `json:"page"`
	Limit     int         `json:"size"`
	TotalPage int         `json:"total_pages"`
	Data      []*AuditLog `json:"data"`
}

func (q *QueryAuditLogResponse) Load(raw []byte) error {
	return json.Unmarshal(raw, q)
}
