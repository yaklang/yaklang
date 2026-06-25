package loop_ssa_api_discovery

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var validHTTPMethods = map[string]struct{}{
	"GET": {}, "POST": {}, "PUT": {}, "DELETE": {},
	"PATCH": {}, "HEAD": {}, "OPTIONS": {},
}

const maxPathPatternLength = 256

// GatewayResult is the outcome of EndpointInsertionGateway.
type GatewayResult struct {
	EndpointID uint
	Status     string
	Reason     string
	Merged     bool
}

// EndpointInsertionGateway is the single entry point for all endpoint candidates.
// It normalizes, validates, deduplicates, and inserts into http_endpoints with
// status=pending_validation. All callers (AI upsert, static harvest, Yak harvest,
// OpenAPI import) must go through this gateway.
func EndpointInsertionGateway(rt *Runtime, candidate *store.HttpEndpoint) (*GatewayResult, error) {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return nil, utils.Error("nil runtime")
	}
	if candidate == nil {
		return nil, utils.Error("nil candidate")
	}

	reason := NormalizeAndValidateEndpoint(candidate)
	if reason != "" {
		return &GatewayResult{Status: "rejected_at_gate", Reason: reason}, nil
	}

	candidate.SessionID = rt.Session.ID
	if candidate.Status == "" {
		candidate.Status = store.EndpointStatusPendingValidation
	}

	existing, err := rt.Repo.ListHttpEndpoints(rt.Session.ID)
	if err != nil {
		return nil, err
	}
	rk := routeKey(candidate.Method, candidate.PathPattern)
	for i := range existing {
		e := &existing[i]
		if routeKey(e.Method, e.PathPattern) == rk {
			need := false
			if e.HandlerClass == "" && candidate.HandlerClass != "" {
				e.HandlerClass = candidate.HandlerClass
				need = true
			}
			if e.HandlerMethod == "" && candidate.HandlerMethod != "" {
				e.HandlerMethod = candidate.HandlerMethod
				need = true
			}
			if need {
				if err := rt.Repo.UpdateHttpEndpoint(e); err != nil {
					log.Warnf("endpoint_gateway: merge update: %v", err)
				}
			}
			return &GatewayResult{
				EndpointID: e.ID,
				Status:     e.Status,
				Reason:     "merged with existing",
				Merged:     true,
			}, nil
		}
	}

	if candidate.PathPattern == "" {
		candidate.PathPattern = "/"
	}
	if err := rt.Repo.CreateHttpEndpoint(candidate); err != nil {
		return nil, fmt.Errorf("endpoint_gateway: create: %w", err)
	}
	return &GatewayResult{
		EndpointID: candidate.ID,
		Status:     candidate.Status,
		Reason:     "created",
	}, nil
}

// EndpointInsertionGatewayBatch processes a batch of candidates through the gateway.
func EndpointInsertionGatewayBatch(rt *Runtime, candidates []store.HttpEndpoint) (inserted, merged, rejected int) {
	for i := range candidates {
		res, err := EndpointInsertionGateway(rt, &candidates[i])
		if err != nil {
			log.Warnf("endpoint_gateway: batch item: %v", err)
			rejected++
			continue
		}
		switch {
		case res.Status == "rejected_at_gate":
			rejected++
		case res.Merged:
			merged++
		default:
			inserted++
		}
	}
	return
}

// NormalizeAndValidateEndpoint normalizes method/path and returns a non-empty
// rejection reason if the candidate is invalid.
func NormalizeAndValidateEndpoint(ep *store.HttpEndpoint) string {
	ep.Method = strings.ToUpper(strings.TrimSpace(ep.Method))
	if ep.Method == "" {
		return "method is empty"
	}
	if _, ok := validHTTPMethods[ep.Method]; !ok {
		return fmt.Sprintf("invalid HTTP method: %s", ep.Method)
	}

	ep.PathPattern = strings.TrimSpace(ep.PathPattern)
	if ep.PathPattern == "" {
		return "path_pattern is empty"
	}
	if !strings.HasPrefix(ep.PathPattern, "/") {
		ep.PathPattern = "/" + ep.PathPattern
	}
	for strings.Contains(ep.PathPattern, "//") {
		ep.PathPattern = strings.ReplaceAll(ep.PathPattern, "//", "/")
	}
	if len(ep.PathPattern) > maxPathPatternLength {
		return fmt.Sprintf("path_pattern too long (%d > %d)", len(ep.PathPattern), maxPathPatternLength)
	}
	if containsIllegalPathChars(ep.PathPattern) {
		return fmt.Sprintf("path_pattern contains illegal characters: %s", ep.PathPattern)
	}
	if isUnsampleablePath(ep.PathPattern) {
		return fmt.Sprintf("path_pattern is unsampleable (pure wildcard/regex): %s", ep.PathPattern)
	}
	return ""
}

func containsIllegalPathChars(p string) bool {
	for _, r := range p {
		if r == ' ' || r == '\\' || r == '<' || r == '>' || r == '"' {
			if r == '<' && strings.Contains(p, ">") {
				continue
			}
			return true
		}
		if !unicode.IsPrint(r) {
			return true
		}
	}
	return false
}

func isUnsampleablePath(p string) bool {
	clean := strings.TrimSpace(p)
	if !strings.HasPrefix(clean, "/") {
		clean = "/" + clean
	}
	if clean == "/**" || clean == "/*" || clean == "/.*" {
		return true
	}
	if strings.HasPrefix(strings.TrimPrefix(clean, "/"), "regex:") {
		return true
	}
	segments := strings.Split(clean, "/")
	allWild := true
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" || seg == "*" || seg == "**" {
			continue
		}
		allWild = false
		break
	}
	if allWild && len(segments) > 1 {
		return true
	}
	return false
}
