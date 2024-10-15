package sfweb

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/yaklang/yaklang/common/utils"
)

type SyntaxFlowScanRequest struct {
	Content string `json:"content"`
	Lang    string `json:"lang"`
}

type SyntaxFlowScanResponse struct {
	Risk []SyntaxFlowScanRisk `json:"risk"`
}

type SyntaxFlowScanRisk struct {
	RuleName  string   `json:"rule_name"`
	Severity  Severity `json:"severity"`
	Title     string   `json:"title"`
	Type      string   `json:"type"`
	VarName   string   `json:"var_name"`
	ResultID  int64    `json:"result_id"`
	Timestamp int64    `json:"timestamp"`
}

type Severity string

const (
	Fatal  Severity = "fatal"
	High   Severity = "high"
	Info   Severity = "info"
	Low    Severity = "low"
	Middle Severity = "middle"
)

func (s *SyntaxFlowWebServer) registerScanRoute() {
	s.router.HandleFunc("/scan", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeErrorJson(w, utils.Wrap(err, "read body error"))
			return
		}
		var req SyntaxFlowScanRequest
		if err = json.Unmarshal(body, &req); err != nil {
			writeErrorJson(w, utils.Wrap(err, "unmarshal request error"))
			return
		}
	}).Name("syntaxflow scan").Methods(http.MethodPost)
}
