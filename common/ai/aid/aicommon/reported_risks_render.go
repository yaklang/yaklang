package aicommon

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/schema"
)

// ReportedRiskItem is a compact, serializable summary of a single risk that
// has been persisted via cybersecurity-risk or any other risk-emitting tool.
// It carries only the fields needed for AI dedup: target, type, parameter,
// title, severity. The full risk record remains in the risk database.
type ReportedRiskItem struct {
	ID          string `json:"id"`        // stable hash for dedup / eviction
	Severity    string `json:"severity"`  // high / middle / low / info / debug
	RiskType    string `json:"risk_type"` // sqli / xss / rce / ...
	Target      string `json:"target"`    // normalized URL or host:port
	Parameter   string `json:"parameter"` // triggering param, or ""
	Title       string `json:"title"`     // one-line title (titleVerbose preferred)
	CreatedUnix int64  `json:"created_unix"`
}

// ReportedRiskStore is the running session's shared deduplication ledger.
// Prompt content comes from immutable Timeline entries, not this store.
type ReportedRiskStore struct {
	mu    sync.Mutex
	Items []ReportedRiskItem `json:"items"`
}

// NewReportedRiskStore returns an empty store.
func NewReportedRiskStore() *ReportedRiskStore {
	return &ReportedRiskStore{Items: make([]ReportedRiskItem, 0)}
}

// Marshal serializes the store to JSON.
func (s *ReportedRiskStore) Marshal() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, _ := json.Marshal(s)
	return string(raw)
}

// UnmarshalReportedRiskStore deserializes a JSON payload. Returns an empty
// store when the payload is empty or invalid.
func UnmarshalReportedRiskStore(data string) *ReportedRiskStore {
	if strings.TrimSpace(data) == "" {
		return NewReportedRiskStore()
	}
	store := &ReportedRiskStore{}
	if err := json.Unmarshal([]byte(data), store); err != nil {
		return NewReportedRiskStore()
	}
	if store.Items == nil {
		store.Items = make([]ReportedRiskItem, 0)
	}
	return store
}

// computeReportedRiskID returns a deterministic ID for dedup: a compact hash
// over normalized target + risk_type + parameter. Two risks with the same
// target, type, and parameter yield the same ID, so the store can detect
// duplicates and skip re-appending.
func computeReportedRiskID(target, riskType, parameter string) string {
	key := strings.Join([]string{
		strings.ToLower(strings.TrimSpace(target)),
		strings.ToLower(strings.TrimSpace(riskType)),
		strings.ToLower(strings.TrimSpace(parameter)),
	}, "|")
	return stableShortHash(key)
}

// stableShortHash returns a short hex digest for use as a deterministic ID.
func stableShortHash(s string) string {
	return fmt.Sprintf("%x", simpleHash(s))
}

// simpleHash is a non-cryptographic 64-bit hash (FNV-1a). Sufficient for
// dedup IDs — we are not doing security-sensitive keying here.
func simpleHash(s string) uint64 {
	var h uint64 = 14695981039346656037 // FNV offset
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211 // FNV prime
	}
	return h
}

// AppendFromRisk extracts a ReportedRiskItem from a full schema.Risk and
// appends it to the store if it is not a duplicate (same target + type +
// parameter). Returns true if a new item was added, false if it was a dup.
func (s *ReportedRiskStore) AppendFromRisk(risk *schema.Risk) bool {
	if s == nil || risk == nil {
		return false
	}
	item := reportedRiskItemFromRisk(risk)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hasRiskLocked(item.ID) {
		return false
	}
	s.Items = append(s.Items, item)
	return true
}

func reportedRiskItemFromRisk(risk *schema.Risk) ReportedRiskItem {
	target := normalizeRiskTargetForSummary(risk.Url, risk.Host, risk.Port)
	riskType := strings.TrimSpace(risk.RiskType)
	parameter := strings.TrimSpace(risk.Parameter)
	title := pickRiskTitle(risk.Title, risk.RiskTypeVerbose)
	severity := strings.TrimSpace(risk.Severity)

	id := computeReportedRiskID(target, riskType, parameter)

	return ReportedRiskItem{
		ID:          id,
		Severity:    severity,
		RiskType:    riskType,
		Target:      target,
		Parameter:   parameter,
		Title:       title,
		CreatedUnix: time.Now().Unix(),
	}
}

func (s *ReportedRiskStore) hasRiskLocked(id string) bool {
	for _, item := range s.Items {
		if item.ID == id {
			return true
		}
	}
	return false
}

// formatReportedRiskLine renders a single risk as one compact line:
//
//	[HIGH] sqli @ example.com/login (username) — 登录接口 SQL 注入
func formatReportedRiskLine(item *ReportedRiskItem) string {
	severity := strings.ToUpper(strings.TrimSpace(item.Severity))
	if severity == "" {
		severity = "INFO"
	}
	severityTag := fmt.Sprintf("[%-4s]", severity)

	riskType := strings.TrimSpace(item.RiskType)
	if riskType == "" {
		riskType = "info"
	}

	target := strings.TrimSpace(item.Target)
	if target == "" {
		target = "?"
	}

	param := strings.TrimSpace(item.Parameter)
	if param == "" {
		param = "-"
	}

	title := truncateRiskTitle(item.Title)

	return fmt.Sprintf("%s %s @ %s (%s) — %s",
		severityTag, riskType, target, param, title)
}

// pickRiskTitle prefers the verbose (Chinese) title when available, falling
// back to the English title.
func pickRiskTitle(title, titleVerbose string) string {
	t := strings.TrimSpace(titleVerbose)
	if t == "" {
		t = strings.TrimSpace(title)
	}
	return t
}

// truncateRiskTitle caps the title to 40 runes with an ellipsis.
func truncateRiskTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "(untitled)"
	}
	runes := []rune(title)
	if len(runes) > 40 {
		return string(runes[:40]) + "..."
	}
	return title
}

// normalizeRiskTargetForSummary strips scheme, default ports, and trailing
// slash from a URL or falls back to host:port. The result is a compact
// target string suitable for one-line risk summaries.
func normalizeRiskTargetForSummary(url, host string, port int) string {
	s := strings.TrimSpace(url)
	if s != "" {
		// Strip scheme.
		s = strings.TrimPrefix(s, "https://")
		s = strings.TrimPrefix(s, "http://")
		// Strip default ports.
		s = strings.Replace(s, ":443/", "/", 1)
		s = strings.Replace(s, ":80/", "/", 1)
		// Strip trailing slash (but keep root "/").
		if len(s) > 1 {
			s = strings.TrimSuffix(s, "/")
		}
		// Truncate to 60 runes.
		if len([]rune(s)) > 60 {
			s = string([]rune(s)[:60]) + "..."
		}
		return s
	}
	// Fallback to host:port.
	h := strings.TrimSpace(host)
	if h == "" {
		return ""
	}
	if port > 0 && port != 80 && port != 443 {
		return fmt.Sprintf("%s:%d", h, port)
	}
	return h
}
