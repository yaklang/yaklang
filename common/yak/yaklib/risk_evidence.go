package yaklib

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	RiskEvidenceMaxRisks       = 5
	RiskEvidenceMaxPacketPairs = 20
)

type RiskEvidencePacketPair struct {
	HTTPFlowID   int64
	URL          string
	RequestFile  string
	ResponseFile string
}

type RiskEvidenceItem struct {
	ID                uint
	Hash              string
	Title             string
	RiskType          string
	Severity          string
	URL               string
	Host              string
	IP                string
	Parameter         string
	Payload           string
	Description       string
	Details           string
	Solution          string
	Disposition       string
	DuplicateOfRiskID uint
	TagReason         string
	Method            string
	Replayable        bool
	StateChanging     bool
	HasAuthMaterial   bool
	RequestPreview    string
	ResponsePreview   string
	RequestFile       string
	ResponseFile      string
	PacketPairs       []*RiskEvidencePacketPair
}

type RiskEvidenceResult struct {
	DatabaseID  string
	Items       []*RiskEvidenceItem
	MissingIDs  []int64
	PacketLimit int
}

func (r *RiskEvidenceResult) Dump() string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "database_id=%s hits=%d missing=%d packet_limit=%d\n",
		r.DatabaseID, len(r.Items), len(r.MissingIDs), r.PacketLimit)
	if len(r.MissingIDs) > 0 {
		fmt.Fprintf(&b, "missing_ids=%s\n", formatInt64List(r.MissingIDs))
	}
	for _, item := range r.Items {
		if item == nil {
			continue
		}
		fmt.Fprintf(&b, `
--- risk_evidence risk_id=%d hash=%s ---
title=%s
type=%s severity=%s
url=%s host=%s ip=%s
parameter=%s
payload=%s
disposition=%s duplicate_of_risk_id=%d tag_reason=%s
method=%s replayable=%t state_changing=%t has_auth_material=%t
request_file=%s
response_file=%s
description:
%s
details:
%s
solution:
%s
`, item.ID, item.Hash, item.Title, item.RiskType, item.Severity,
			item.URL, item.Host, item.IP, item.Parameter, item.Payload,
			schema.NormalizeRiskDisposition(item.Disposition), item.DuplicateOfRiskID, item.TagReason,
			item.Method, item.Replayable, item.StateChanging, item.HasAuthMaterial,
			emptyAsDash(item.RequestFile), emptyAsDash(item.ResponseFile),
			item.Description, item.Details, item.Solution)
		if len(item.PacketPairs) > 0 {
			b.WriteString("packet_pairs:\n")
			for _, pair := range item.PacketPairs {
				if pair == nil {
					continue
				}
				fmt.Fprintf(&b, "- httpflow_id=%d url=%s request_file=%s response_file=%s\n",
					pair.HTTPFlowID, pair.URL,
					emptyAsDash(pair.RequestFile), emptyAsDash(pair.ResponseFile))
			}
		}
		if item.RequestPreview != "" {
			b.WriteString("saved_request_preview:\n--- REQUEST PREVIEW ---\n")
			b.WriteString(item.RequestPreview)
			if !strings.HasSuffix(item.RequestPreview, "\n") {
				b.WriteByte('\n')
			}
			b.WriteString("--- END REQUEST PREVIEW ---\n")
		} else {
			b.WriteString("saved_request_preview: (none)\n")
		}
		if item.ResponsePreview != "" {
			b.WriteString("saved_response_preview:\n--- RESPONSE PREVIEW ---\n")
			b.WriteString(item.ResponsePreview)
			if !strings.HasSuffix(item.ResponsePreview, "\n") {
				b.WriteByte('\n')
			}
			b.WriteString("--- END RESPONSE PREVIEW ---\n")
		} else {
			b.WriteString("saved_response_preview: (none)\n")
		}
	}
	return strings.TrimSpace(b.String())
}

// GetRiskEvidenceInDatabase loads bounded Risk evidence for review workflows.
// Raw packets are written to files so callers can replay exact bytes without
// relying on potentially truncated prompt content.
func GetRiskEvidenceInDatabase(
	ids []int64,
	packetLimit int,
	opts ...DBHistoryOption,
) (*RiskEvidenceResult, error) {
	ids, err := normalizeRiskEvidenceIDs(ids)
	if err != nil {
		return nil, err
	}
	if packetLimit < 256 || packetLimit > 32768 {
		return nil, fmt.Errorf("packet_limit must be 256..32768")
	}

	c, err := historyConfig(0, opts)
	if err != nil {
		return nil, err
	}
	if c.methods != "" || c.status != "" || c.source != "" || c.packetLimit != 0 {
		return nil, fmt.Errorf("HTTP-specific options are not supported by risk evidence queries")
	}
	if err := validateRiskSelectionScope(c, ids); err != nil {
		return nil, err
	}
	db, closeDB, err := historyProjectDatabase(c)
	if err != nil {
		return nil, err
	}
	defer closeDB()

	var risks []*schema.Risk
	if err := db.Model(&schema.Risk{}).Where("id IN (?)", ids).Find(&risks).Error; err != nil {
		return nil, utils.Wrap(err, "load risk evidence")
	}
	byID := make(map[uint]*schema.Risk, len(risks))
	for _, risk := range risks {
		byID[risk.ID] = risk
	}

	result := &RiskEvidenceResult{
		DatabaseID:  "current",
		PacketLimit: packetLimit,
	}
	for _, id := range ids {
		risk := byID[uint(id)]
		if risk == nil {
			result.MissingIDs = append(result.MissingIDs, id)
			continue
		}
		result.Items = append(result.Items, buildRiskEvidenceItem(risk, packetLimit))
	}
	return result, c.ctx.Err()
}

func normalizeRiskEvidenceIDs(ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("at least one risk ID is required")
	}
	if len(ids) > RiskEvidenceMaxRisks {
		return nil, fmt.Errorf("risk evidence supports at most %d risks per call", RiskEvidenceMaxRisks)
	}
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, fmt.Errorf("risk ID must be positive: %d", id)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one risk ID is required")
	}
	return out, nil
}

func buildRiskEvidenceItem(risk *schema.Risk, packetLimit int) *RiskEvidenceItem {
	if risk == nil {
		return nil
	}
	request := unquoteRiskEvidenceField(risk.QuotedRequest)
	response := unquoteRiskEvidenceField(risk.QuotedResponse)
	if request == "" || response == "" {
		for _, pair := range risk.PacketPairs {
			if pair == nil {
				continue
			}
			if request == "" {
				request = pair.Request
			}
			if response == "" {
				response = pair.Response
			}
			if request != "" && response != "" {
				break
			}
		}
	}

	requestPreview, requestFile := saveRiskEvidencePacket("request-"+fmt.Sprint(risk.ID), request, packetLimit)
	responsePreview, responseFile := saveRiskEvidencePacket("response-"+fmt.Sprint(risk.ID), response, packetLimit)
	method := riskEvidenceRequestMethod(request)

	item := &RiskEvidenceItem{
		ID:                risk.ID,
		Hash:              risk.Hash,
		Title:             risk.Title,
		RiskType:          risk.RiskType,
		Severity:          risk.Severity,
		URL:               risk.Url,
		Host:              risk.Host,
		IP:                risk.IP,
		Parameter:         risk.Parameter,
		Payload:           risk.Payload,
		Description:       shrinkRiskEvidenceText(risk.Description, packetLimit),
		Details:           shrinkRiskEvidenceText(unquoteRiskEvidenceField(risk.Details), packetLimit),
		Solution:          shrinkRiskEvidenceText(risk.Solution, packetLimit),
		Disposition:       schema.NormalizeRiskDisposition(risk.Disposition),
		DuplicateOfRiskID: risk.DuplicateOfRiskID,
		TagReason:         risk.TagReason,
		Method:            method,
		Replayable:        strings.TrimSpace(request) != "",
		StateChanging:     method != "" && method != "GET" && method != "HEAD" && method != "OPTIONS",
		HasAuthMaterial:   riskRequestHasAuthMaterial(request),
		RequestPreview:    requestPreview,
		ResponsePreview:   responsePreview,
		RequestFile:       requestFile,
		ResponseFile:      responseFile,
	}
	for index, pair := range risk.PacketPairs {
		if pair == nil || index >= RiskEvidenceMaxPacketPairs {
			continue
		}
		_, pairRequestFile := saveRiskEvidencePacket(fmt.Sprintf("pair-%d-request-%d", risk.ID, index+1), pair.Request, packetLimit)
		_, pairResponseFile := saveRiskEvidencePacket(fmt.Sprintf("pair-%d-response-%d", risk.ID, index+1), pair.Response, packetLimit)
		item.PacketPairs = append(item.PacketPairs, &RiskEvidencePacketPair{
			HTTPFlowID:   pair.HTTPFlowId,
			URL:          pair.Url,
			RequestFile:  pairRequestFile,
			ResponseFile: pairResponseFile,
		})
	}
	return item
}

func unquoteRiskEvidenceField(value string) string {
	if value == "" {
		return ""
	}
	if unquoted, err := strconv.Unquote(value); err == nil {
		return unquoted
	}
	return value
}

func saveRiskEvidencePacket(label, content string, limit int) (preview string, filePath string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", ""
	}
	filePath = consts.TempAIFileFast("attached-risk-"+label+"-*.http", content)
	return shrinkRiskEvidenceText(content, limit), filePath
}

func shrinkRiskEvidenceText(content string, limit int) string {
	content = strings.TrimSpace(content)
	if len(content) <= limit {
		return content
	}
	end := limit
	for end > 0 && !utf8.RuneStart(content[end]) {
		end--
	}
	return content[:end] + "\n... (truncated; full packet is available in the referenced file)"
}

func riskEvidenceRequestMethod(request string) string {
	firstLine := request
	if index := strings.IndexAny(firstLine, "\r\n"); index >= 0 {
		firstLine = firstLine[:index]
	}
	fields := strings.Fields(firstLine)
	if len(fields) == 0 {
		return ""
	}
	return strings.ToUpper(fields[0])
}

func riskRequestHasAuthMaterial(request string) bool {
	lower := strings.ToLower(request)
	return strings.Contains(lower, "\nauthorization:") ||
		strings.HasPrefix(lower, "authorization:") ||
		strings.Contains(lower, "\ncookie:") ||
		strings.HasPrefix(lower, "cookie:")
}

func emptyAsDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func init() {
	RiskExports["GetRiskEvidenceInDatabase"] = GetRiskEvidenceInDatabase
}
