package loop_ssa_api_discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// ValidationVerdict is the outcome of endpoint validation.
type ValidationVerdict string

const (
	VerdictAlive       ValidationVerdict = "alive"
	VerdictRejected    ValidationVerdict = "rejected"
	VerdictAuthFailed  ValidationVerdict = "auth_failed"
	VerdictUnreachable ValidationVerdict = "unreachable"
	VerdictNeedsAI     ValidationVerdict = "needs_ai"
)

// ValidationEvidence captures the proof behind a verdict.
type ValidationEvidence struct {
	Verdict     ValidationVerdict `json:"verdict"`
	StatusCode  int               `json:"status_code"`
	Reason      string            `json:"reason"`
	AIAnalysis  string            `json:"ai_analysis,omitempty"`
	Score       int               `json:"score"`
}

// framework 404 fingerprints (lowercased for matching)
var framework404Fingerprints = []string{
	"whitelabel error page",
	"werkzeug debugger",
	"cannot get /",
	"cannot get",
	"cannot post",
	"<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<title>error</title>",
	"404 not found",
	"the requested url was not found",
	"<center>nginx",
	"<title>404 not found</title>",
	"page not found",
	"not found. the requested url",
	"django.http.response.http404",
}

// framework error fingerprints that indicate the endpoint exists but errored
var frameworkErrorFingerprints = []string{
	"traceback (most recent call last)",
	"at java.", "at sun.", "at org.springframework",
	"stack trace:", "exception in thread",
	"<b>fatal error</b>",
	"goroutine ",
	"panic:",
	"unhandled exception",
	"internal server error",
}

// ValidateEndpointAliveAndFunctional probes an endpoint and returns a verdict.
// It uses rule-based checks first, and falls back to AI for ambiguous cases.
func ValidateEndpointAliveAndFunctional(
	ctx context.Context,
	rt *Runtime,
	ep *store.HttpEndpoint,
	cred *store.AuthCredential,
	baseURL string,
	aiEnabled bool,
) (*ValidationEvidence, error) {
	if rt == nil || ep == nil {
		return nil, utils.Error("nil runtime or endpoint")
	}

	samplePath := samplePathLiteralGo(ep.PathPattern)
	fullURL := joinURLGo(baseURL, samplePath)

	headers := make(map[string]string)
	if cred != nil && cred.HeadersJSON != "" {
		_ = json.Unmarshal([]byte(cred.HeadersJSON), &headers)
	}
	if len(headers) == 0 && cred != nil && cred.HeaderName != "" && cred.HeaderValue != "" {
		headers[cred.HeaderName] = cred.HeaderValue
	}

	statusCode, body, err := probeEndpoint(ctx, ep.Method, fullURL, headers)
	if err != nil {
		return recordAttempt(rt, ep, 0, fullURL, VerdictUnreachable, "network error: "+err.Error(), "", 0), nil
	}

	bodyLower := strings.ToLower(body)
	bodyLen := len(body)

	switch {
	case statusCode == 401 || statusCode == 403:
		return recordAttempt(rt, ep, statusCode, fullURL, VerdictAuthFailed,
			fmt.Sprintf("HTTP %d", statusCode), "", 0), nil

	case statusCode == 404:
		if matchesAnyFingerprint(bodyLower, framework404Fingerprints) {
			return recordAttempt(rt, ep, statusCode, fullURL, VerdictRejected,
				"404 with framework default page", "", 0), nil
		}
		return recordAttempt(rt, ep, statusCode, fullURL, VerdictRejected,
			"HTTP 404", "", 0), nil

	case statusCode == 405:
		return recordAttempt(rt, ep, statusCode, fullURL, VerdictRejected,
			"HTTP 405 Method Not Allowed", "", 0), nil

	case statusCode >= 500:
		if matchesAnyFingerprint(bodyLower, frameworkErrorFingerprints) {
			return recordAttempt(rt, ep, statusCode, fullURL, VerdictAlive,
				"5xx with stack trace / framework error (endpoint exists)", "", 70), nil
		}
		if bodyLen < 32 {
			return recordAttempt(rt, ep, statusCode, fullURL, VerdictUnreachable,
				"5xx with empty/short body (gateway error)", "", 0), nil
		}
		return recordAttempt(rt, ep, statusCode, fullURL, VerdictAlive,
			fmt.Sprintf("5xx with substantial body (%d bytes)", bodyLen), "", 50), nil

	case statusCode >= 200 && statusCode < 400:
		if bodyLen < 32 && !looksLikeJSON(body) {
			if aiEnabled {
				return aiValidationFallback(ctx, rt, ep, statusCode, fullURL, body)
			}
			return recordAttempt(rt, ep, statusCode, fullURL, VerdictAlive,
				"short response, AI unavailable", "", 40), nil
		}
		if isAPIPathButHTMLResponse(ep.PathPattern, body) {
			if aiEnabled {
				return aiValidationFallback(ctx, rt, ep, statusCode, fullURL, body)
			}
			return recordAttempt(rt, ep, statusCode, fullURL, VerdictRejected,
				"/api/* path returns HTML welcome page", "", 0), nil
		}
		return recordAttempt(rt, ep, statusCode, fullURL, VerdictAlive,
			fmt.Sprintf("HTTP %d OK (%d bytes)", statusCode, bodyLen), "", 80), nil
	}

	return recordAttempt(rt, ep, statusCode, fullURL, VerdictRejected,
		fmt.Sprintf("unhandled status code %d", statusCode), "", 0), nil
}

func matchesAnyFingerprint(bodyLower string, fingerprints []string) bool {
	for _, fp := range fingerprints {
		if strings.Contains(bodyLower, fp) {
			return true
		}
	}
	return false
}

func looksLikeJSON(body string) bool {
	trimmed := strings.TrimSpace(body)
	return (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]"))
}

func isAPIPathButHTMLResponse(pathPattern, body string) bool {
	pp := strings.ToLower(pathPattern)
	if !strings.Contains(pp, "/api") && !strings.Contains(pp, "/v1") &&
		!strings.Contains(pp, "/v2") && !strings.Contains(pp, "/rest") {
		return false
	}
	bl := strings.ToLower(strings.TrimSpace(body))
	return (strings.HasPrefix(bl, "<!doctype") || strings.HasPrefix(bl, "<html")) &&
		len(body) > 500
}

func aiValidationFallback(
	ctx context.Context,
	rt *Runtime,
	ep *store.HttpEndpoint,
	statusCode int,
	fullURL, body string,
) (*ValidationEvidence, error) {
	snippet := body
	if len(snippet) > 1500 {
		snippet = snippet[:1500]
	}
	// AI prompt is prepared for future integration with ai.Chat in Yak context
	_ = fmt.Sprintf("endpoint=%s %s handler=%s url=%s code=%d body_len=%d",
		ep.Method, ep.PathPattern, ep.HandlerClass, fullURL, statusCode, len(snippet))
	_ = ctx
	log.Infof("endpoint_validation: AI fallback for %s %s", ep.Method, ep.PathPattern)

	verdict := VerdictAlive
	score := 50
	aiAnalysis := "AI fallback unavailable in current context"

	return recordAttempt(rt, ep, statusCode, fullURL, verdict, "AI fallback", aiAnalysis, score), nil
}

func recordAttempt(
	rt *Runtime,
	ep *store.HttpEndpoint,
	statusCode int,
	url string,
	verdict ValidationVerdict,
	reason, aiAnalysis string,
	score int,
) *ValidationEvidence {
	now := time.Now()
	ep.Status = string(verdict)
	ep.LastProbedAt = &now
	ep.ProbeStatusCode = statusCode
	ep.FunctionScore = score
	if verdict == VerdictRejected || verdict == VerdictAuthFailed || verdict == VerdictUnreachable {
		ep.RejectReason = reason
	}

	evidence := &ValidationEvidence{
		Verdict:    verdict,
		StatusCode: statusCode,
		Reason:     reason,
		AIAnalysis: aiAnalysis,
		Score:      score,
	}
	evJSON, _ := json.Marshal(evidence)
	ep.ProbeEvidence = string(evJSON)

	if rt != nil && rt.Repo != nil {
		if err := rt.Repo.UpdateHttpEndpointStatus(ep); err != nil {
			log.Warnf("endpoint_validation: update status: %v", err)
		}

		attemptNo := 1
		if n, nerr := rt.Repo.CountEndpointValidationAttempts(rt.Session.ID, ep.ID); nerr == nil {
			attemptNo = n + 1
		}
		attempt := &store.EndpointValidationAttempt{
			SessionID:       rt.Session.ID,
			HttpEndpointID:  ep.ID,
			AttemptNo:       attemptNo,
			URL:             url,
			Method:          ep.Method,
			StatusCode:      statusCode,
			ResponseSnippet: truncateString(string(evJSON), 2000),
			Verdict:         string(verdict),
			Reason:          reason,
			AIAnalysis:      aiAnalysis,
		}
		if err := rt.Repo.CreateEndpointValidationAttempt(attempt); err != nil {
			log.Warnf("endpoint_validation: create attempt: %v", err)
		}
	}

	return evidence
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

// probeEndpoint sends a single HTTP request and returns status code, body, error.
// This is a Go-side probe (not Yak poc.*) for use in the insertion gateway.
func probeEndpoint(ctx context.Context, method, url string, headers map[string]string) (int, string, error) {
	_ = ctx
	log.Infof("endpoint_validation: probing %s %s", method, url)
	return 0, "", fmt.Errorf("probe not yet wired to HTTP client")
}

// samplePathLiteralGo is the Go equivalent of samplePathLiteral in Yak.
func samplePathLiteralGo(pathPattern string) string {
	p := strings.TrimSpace(pathPattern)
	if p == "" {
		return ""
	}
	lp := strings.ToLower(p)
	if strings.HasPrefix(lp, "http://") || strings.HasPrefix(lp, "https://") {
		return p
	}
	p = strings.ReplaceAll(p, "**", "*")
	parts := strings.Split(p, "/")
	var out []string
	for _, seg := range parts {
		if seg == "" {
			out = append(out, seg)
			continue
		}
		if seg == "*" || strings.HasPrefix(seg, ":") {
			seg = "1"
		} else if strings.Contains(seg, "<") && strings.Contains(seg, ">") {
			seg = "1"
		} else if strings.Contains(seg, "{") {
			seg = "1"
		}
		out = append(out, seg)
	}
	return strings.Join(out, "/")
}

// joinURLGo is the Go equivalent of joinURL in Yak.
func joinURLGo(base, pathPattern string) string {
	p := strings.TrimSpace(pathPattern)
	if p == "" {
		return strings.TrimSuffix(strings.TrimSpace(base), "/")
	}
	lp := strings.ToLower(p)
	if strings.HasPrefix(lp, "http://") || strings.HasPrefix(lp, "https://") {
		return p
	}
	b := strings.TrimSuffix(strings.TrimSpace(base), "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if b == "" {
		return p
	}
	return b + p
}
