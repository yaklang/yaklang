//go:build hids

package rule

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/yaklang/yaklang/common/yak"
)

type temporaryRuleActionResult struct {
	Title            string
	Severity         string
	Tags             []string
	Detail           map[string]any
	EvidenceRequests []map[string]any
}

func validateActionExpression(
	sandbox *yak.Sandbox,
	action string,
	vars map[string]any,
) error {
	action = strings.TrimSpace(action)
	if sandbox == nil || action == "" {
		return nil
	}
	raw, err := sandbox.ExecuteAsExpression(action, vars)
	if err != nil {
		return err
	}
	result, err := parseTemporaryRuleActionResult(raw)
	if err != nil {
		return err
	}
	return validateTemporaryRuleEvidenceRequestExpressions(sandbox, result.EvidenceRequests)
}

func parseTemporaryRuleActionResult(raw any) (temporaryRuleActionResult, error) {
	if raw == nil {
		return temporaryRuleActionResult{}, nil
	}

	values := helperGeneralMap(raw)
	if len(values) == 0 {
		return temporaryRuleActionResult{}, fmt.Errorf("action must return an object")
	}

	severity, err := parseTemporaryRuleActionSeverity(values["severity"])
	if err != nil {
		return temporaryRuleActionResult{}, err
	}
	detail, err := parseTemporaryRuleActionDetail(values["detail"])
	if err != nil {
		return temporaryRuleActionResult{}, err
	}
	evidenceRequests, err := parseTemporaryRuleEvidenceRequests(firstNonNil(
		values["evidence_requests"],
		values["evidence_request"],
		values["evidence"],
	))
	if err != nil {
		return temporaryRuleActionResult{}, err
	}

	return temporaryRuleActionResult{
		Title:            strings.TrimSpace(helperStringField(values, "title")),
		Severity:         severity,
		Tags:             normalizeActionTags(values["tags"]),
		Detail:           detail,
		EvidenceRequests: evidenceRequests,
	}, nil
}

func normalizeActionTags(value any) []string {
	items := flattenHelperStrings(value)
	if len(items) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(items))
	for _, item := range items {
		tag := strings.TrimSpace(item)
		if tag == "" {
			continue
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		normalized = append(normalized, tag)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func parseTemporaryRuleActionSeverity(value any) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(fmt.Sprint(firstNonNil(value, ""))))
	switch normalized {
	case "":
		return "", nil
	case "critical", "high", "medium", "low", "unknown":
		return normalized, nil
	default:
		return "", fmt.Errorf("severity must be one of critical, high, medium, low, unknown")
	}
}

func parseTemporaryRuleActionDetail(value any) (map[string]any, error) {
	if value == nil {
		return nil, nil
	}
	if text, ok := value.(string); ok {
		text = strings.TrimSpace(text)
		if text == "" {
			return nil, nil
		}
		return map[string]any{"summary": text}, nil
	}

	detail := helperGeneralMap(value)
	if len(detail) == 0 {
		return nil, fmt.Errorf("detail must be an object or string")
	}
	return cloneMapStringAny(detail), nil
}

func parseTemporaryRuleEvidenceRequests(value any) ([]map[string]any, error) {
	if value == nil {
		return nil, nil
	}

	items := []any{}
	switch typed := value.(type) {
	case []any:
		items = typed
	default:
		reflected := reflect.ValueOf(value)
		if reflected.IsValid() && reflected.Kind() == reflect.Slice {
			items = make([]any, 0, reflected.Len())
			for index := 0; index < reflected.Len(); index++ {
				items = append(items, reflected.Index(index).Interface())
			}
		} else {
			items = append(items, typed)
		}
	}

	requests := make([]map[string]any, 0, len(items))
	for index, item := range items {
		request, err := parseTemporaryRuleEvidenceRequest(item)
		if err != nil {
			return nil, fmt.Errorf("evidence_requests[%d]: %w", index, err)
		}
		requests = append(requests, request)
	}
	return requests, nil
}

func parseTemporaryRuleEvidenceRequest(value any) (map[string]any, error) {
	request := helperGeneralMap(value)
	if len(request) == 0 {
		return nil, fmt.Errorf("must be an object")
	}

	kind := strings.TrimSpace(helperStringField(request, "kind"))
	if kind == "" {
		return nil, fmt.Errorf("kind is required")
	}

	normalized := map[string]any{
		"kind": kind,
	}

	if target := strings.TrimSpace(helperStringField(request, "target")); target != "" {
		normalized["target"] = target
	}
	if reason := strings.TrimSpace(helperStringField(request, "reason")); reason != "" {
		normalized["reason"] = reason
	}

	metadata := map[string]any{}
	if rawMetadata := helperGeneralMap(request["metadata"]); len(rawMetadata) > 0 {
		for key, value := range rawMetadata {
			metadata[key] = cloneValue(value)
		}
	}
	for key, value := range request {
		switch key {
		case "kind", "target", "reason", "metadata":
			continue
		default:
			metadata[key] = cloneValue(value)
		}
	}
	if len(metadata) > 0 {
		normalized["metadata"] = metadata
	}

	return normalized, nil
}

func temporaryRuleActionResultMap(result temporaryRuleActionResult) map[string]any {
	actionResult := map[string]any{}
	if result.Title != "" {
		actionResult["title"] = result.Title
	}
	if result.Severity != "" {
		actionResult["severity"] = result.Severity
	}
	if len(result.Tags) > 0 {
		actionResult["tags"] = cloneStringSlice(result.Tags)
	}
	if len(result.Detail) > 0 {
		actionResult["detail"] = cloneMapStringAny(result.Detail)
	}
	if len(result.EvidenceRequests) > 0 {
		actionResult["evidence_requests"] = cloneValue(result.EvidenceRequests)
	}
	if len(actionResult) == 0 {
		return nil
	}
	return actionResult
}

func validateTemporaryRuleEvidenceRequestExpressions(
	sandbox *yak.Sandbox,
	requests []map[string]any,
) error {
	if sandbox == nil || len(requests) == 0 {
		return nil
	}

	for index, request := range requests {
		if err := validateTemporaryRuleEvidenceRequestExpression(sandbox, request); err != nil {
			return fmt.Errorf("evidence_requests[%d]: %w", index, err)
		}
	}
	return nil
}

func validateTemporaryRuleEvidenceRequestExpression(
	sandbox *yak.Sandbox,
	request map[string]any,
) error {
	metadata := helperGeneralMap(request["metadata"])
	if len(metadata) == 0 {
		return nil
	}

	validationContext := buildScanValidationContext()
	for _, field := range []string{
		"scan_match",
		"match",
		"filter",
		"entry_match",
		"finding_match",
	} {
		expression := strings.TrimSpace(helperStringField(metadata, field))
		if expression == "" {
			continue
		}
		if err := ValidateBooleanExpression(sandbox, expression, validationContext); err != nil {
			return fmt.Errorf("%s: %w", field, err)
		}
	}
	return nil
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
