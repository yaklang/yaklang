package airaghttp

import (
	"encoding/json"
	"net/http"

	"github.com/yaklang/yaklang/common/log"
)

// writeJSON 写出 JSON 响应
func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Errorf("write json response failed: %v", err)
	}
}

// writeJSONError 写出统一的 JSON 错误响应
func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]interface{}{
		"ok":    false,
		"error": message,
	})
}
