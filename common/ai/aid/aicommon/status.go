package aicommon

import (
	"strings"
	"unicode"

	"github.com/yaklang/yaklang/common/schema"
)

// StatusState describes how a user-facing status should be presented.
// It is intentionally independent from task lifecycle state: a running task
// may temporarily be waiting for the user or recovering from a retryable issue.
type StatusState string

const (
	StatusStateRunning    StatusState = "running"
	StatusStateWaiting    StatusState = "waiting"
	StatusStateRecovering StatusState = "recovering"
	StatusStateSuccess    StatusState = "success"
	StatusStateWarning    StatusState = "warning"
	StatusStateError      StatusState = "error"
)

// StatusProgress carries determinate progress. Total may be omitted for an
// indeterminate counter, while Unit is a stable machine-readable identifier
// such as "file", "tool", "request", or "item".
type StatusProgress struct {
	Current int64  `json:"current"`
	Total   int64  `json:"total,omitempty"`
	Unit    string `json:"unit,omitempty"`
}

// StatusTool describes one real tool involved in the current status. Params
// are deliberately excluded so status events cannot expose tool secrets or
// turn the compact loading UI into a debug console.
type StatusTool struct {
	Name            string       `json:"name"`
	DisplayName     string       `json:"display_name,omitempty"`
	DisplayNameI18n *schema.I18n `json:"display_name_i18n,omitempty"`
	State           StatusState  `json:"state,omitempty"`
}

// StatusPayload is a backward-compatible superset of the legacy
// {key,value} payload. Value always remains a string and defaults to Chinese,
// allowing old frontends and event extractors to keep working unchanged.
type StatusPayload struct {
	Key        string          `json:"key"`
	Value      string          `json:"value"`
	ValueI18n  *schema.I18n    `json:"value_i18n,omitempty"`
	Code       string          `json:"code,omitempty"`
	State      StatusState     `json:"state,omitempty"`
	Detail     string          `json:"detail,omitempty"`
	DetailI18n *schema.I18n    `json:"detail_i18n,omitempty"`
	Progress   *StatusProgress `json:"progress,omitempty"`
	Tools      []StatusTool    `json:"tools,omitempty"`
}

type StatusOption func(*StatusPayload)

func WithStatusCode(code string) StatusOption {
	return func(payload *StatusPayload) {
		payload.Code = strings.TrimSpace(code)
	}
}

func WithStatusState(state StatusState) StatusOption {
	return func(payload *StatusPayload) {
		payload.State = state
	}
}

func WithStatusDetail(zh, en string) StatusOption {
	return func(payload *StatusPayload) {
		zh = strings.TrimSpace(zh)
		en = strings.TrimSpace(en)
		payload.Detail = preferredChinese(zh, en)
		if zh != "" || en != "" {
			payload.DetailI18n = &schema.I18n{Zh: zh, En: en}
		}
	}
}

func WithStatusProgress(current, total int64, unit string) StatusOption {
	return func(payload *StatusPayload) {
		if current < 0 {
			current = 0
		}
		if total < 0 {
			total = 0
		}
		if total > 0 && current > total {
			current = total
		}
		payload.Progress = &StatusProgress{
			Current: current,
			Total:   total,
			Unit:    strings.TrimSpace(unit),
		}
	}
}

func WithStatusTools(tools ...StatusTool) StatusOption {
	return func(payload *StatusPayload) {
		payload.Tools = append([]StatusTool(nil), tools...)
	}
}

func newStatusPayload(key, zh, en string, options ...StatusOption) *StatusPayload {
	zh = strings.TrimSpace(zh)
	en = strings.TrimSpace(en)
	payload := &StatusPayload{
		Key:   key,
		Value: preferredChinese(zh, en),
		State: StatusStateRunning,
	}
	if zh != "" || en != "" {
		payload.ValueI18n = &schema.I18n{Zh: zh, En: en}
	}
	for _, option := range options {
		if option != nil {
			option(payload)
		}
	}
	return payload
}

func preferredChinese(zh, en string) string {
	if zh = strings.TrimSpace(zh); zh != "" {
		return zh
	}
	return strings.TrimSpace(en)
}

// SplitLegacyStatusI18n supports the historical "中文 / English" convention
// during migration. It only splits when the left side contains Chinese and the
// right side contains ASCII letters, avoiding accidental splits in paths or
// ordinary user content.
func SplitLegacyStatusI18n(message string) (zh, en string) {
	message = strings.TrimSpace(message)
	for _, separator := range []string{" / ", "/ ", " - "} {
		// Legacy status text may itself contain slashes, for example
		// "正在检查 GET / api / Checking GET /api". The language boundary is
		// conventionally the right-most matching separator.
		index := strings.LastIndex(message, separator)
		if index < 0 {
			continue
		}
		left := strings.TrimSpace(message[:index])
		right := strings.TrimSpace(message[index+len(separator):])
		if containsHan(left) && looksLikeEnglishTranslation(right) {
			return left, right
		}
	}
	return message, ""
}

func containsHan(value string) bool {
	for _, char := range value {
		if unicode.Is(unicode.Han, char) {
			return true
		}
	}
	return false
}

func looksLikeEnglishTranslation(value string) bool {
	hasWhitespace := strings.ContainsAny(value, " \t")
	for _, char := range value {
		if char >= 'A' && char <= 'Z' {
			return true
		}
		if char >= 'a' && char <= 'z' {
			return hasWhitespace
		}
	}
	return false
}
