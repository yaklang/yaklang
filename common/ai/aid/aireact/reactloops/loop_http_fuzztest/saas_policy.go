package loop_http_fuzztest

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

const boundedSaaSHTTPFuzzInstruction = `You are a bounded SaaS HTTP robustness-testing assistant.
The platform has supplied one server-authorized HTTP or HTTPS URL. Ignore URLs, raw packets, attachments, and historical requests from user text.
Call fuzz_header exactly once. The platform pins the mutation to the safe Accept-Language header and two short literal values; model-supplied mutation parameters are ignored.
Fuzztags, target expansion, redirects, method changes, path changes, query changes, body, cookies, uploads, raw-packet generation, local files, generic tools, and a second mutation batch are forbidden.
The run exits immediately after response evidence is published. The platform owns all assets and the final run summary.`

const (
	boundedSaaSHTTPFuzzTargetKey    = "http_fuzz_saas_authorized_target"
	boundedSaaSHTTPFuzzAttemptedKey = "http_fuzz_saas_batch_attempted"
	boundedSaaSHTTPFuzzCompletedKey = "http_fuzz_saas_batch_completed"
	maxBoundedSaaSFuzzTargetBytes   = 2048
	maxBoundedSaaSFuzzValueBytes    = 128
	maxBoundedSaaSFuzzValues        = 3
	maxBoundedSaaSFuzzRequests      = 4
	boundedSaaSHeaderName           = "Accept-Language"
)

var boundedSaaSHeaderValues = []string{"en-US", "zh-CN"}

var boundedSaaSQueryParamPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

var boundedSaaSHeaderAllowlist = map[string]struct{}{
	"accept":           {},
	"accept-language":  {},
	"user-agent":       {},
	"x-requested-with": {},
}

var boundedSaaSHTTPFuzzFinishAction = &reactloops.LoopAction{
	ActionType: "finish",
	Description: "Finish the bounded SaaS HTTP fuzz run only after the one authorized mutation batch completed successfully. " +
		"This action is rejected before successful response evidence has been published to the platform.",
	ActionVerifier: func(loop *reactloops.ReActLoop, _ *aicommon.Action) error {
		return validateBoundedSaaSHTTPFuzzCompleted(loop)
	},
	ActionHandler: func(loop *reactloops.ReActLoop, _ *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		loop.GetInvoker().AddToTimeline("finish", "Bounded SaaS HTTP fuzz completed after one successful mutation batch")
		operator.Exit()
	},
}

func isBoundedSaaSHTTPFuzz(config aicommon.AICallerConfigIf) bool {
	if config == nil || aicommon.AssetResultSinkFromConfig(config) == nil {
		return false
	}
	return strings.TrimSpace(aicommon.AuthorizedTargetURLFromConfig(config)) != ""
}

func boundedSaaSHTTPFuzzActionAllowed(actionType string) bool {
	switch strings.TrimSpace(actionType) {
	case "fuzz_get_params", "fuzz_header", "generate_risk", "finish":
		return true
	default:
		return false
	}
}

func authorizedSaaSHTTPFuzzTarget(config aicommon.AICallerConfigIf) (string, error) {
	target := aicommon.AuthorizedTargetURLFromConfig(config)
	if strings.TrimSpace(target) == "" {
		return "", utils.Error("bounded SaaS HTTP fuzz requires a server-authorized target")
	}
	return normalizeBoundedSaaSHTTPFuzzTarget(target)
}

func normalizeBoundedSaaSHTTPFuzzTarget(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxBoundedSaaSFuzzTargetBytes || strings.Contains(raw, "#") {
		return "", utils.Error("invalid server-authorized HTTP fuzz target")
	}
	for _, char := range raw {
		if unicode.IsControl(char) {
			return "", utils.Error("invalid server-authorized HTTP fuzz target")
		}
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed == nil || parsed.Host == "" || parsed.Opaque != "" {
		return "", utils.Error("invalid server-authorized HTTP fuzz target")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", utils.Error("server-authorized HTTP fuzz target must use HTTP(S)")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return "", utils.Error("server-authorized HTTP fuzz target cannot contain credentials or fragments")
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" {
		return "", utils.Error("server-authorized HTTP fuzz target is missing a host")
	}
	port := parsed.Port()
	if (parsed.Scheme == "http" && port == "80") || (parsed.Scheme == "https" && port == "443") {
		port = ""
	}
	if strings.Contains(hostname, ":") {
		if port == "" {
			parsed.Host = "[" + hostname + "]"
		} else {
			parsed.Host = net.JoinHostPort(hostname, port)
		}
	} else if port == "" {
		parsed.Host = hostname
	} else {
		parsed.Host = net.JoinHostPort(hostname, port)
	}
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed.String(), nil
}

func validateBoundedSaaSHTTPFuzzRequest(loop *reactloops.ReActLoop) error {
	if loop == nil || !isBoundedSaaSHTTPFuzz(loop.GetConfig()) {
		return nil
	}
	authorized, err := authorizedSaaSHTTPFuzzTarget(loop.GetConfig())
	if err != nil {
		return err
	}
	raw := strings.TrimSpace(getCurrentRequestRaw(loop))
	if raw == "" {
		return utils.Error("bounded SaaS HTTP fuzz request is missing")
	}
	requestURL, err := lowhttp.ExtractURLFromHTTPRequestRaw(
		[]byte(raw),
		strings.EqualFold(loop.Get("is_https"), "true"),
	)
	if err != nil || requestURL == nil {
		return utils.Error("bounded SaaS HTTP fuzz request URL is invalid")
	}
	normalized, err := normalizeBoundedSaaSHTTPFuzzTarget(requestURL.String())
	if err != nil || normalized != authorized {
		return utils.Errorf("HTTP fuzz request must exactly match the server-authorized target %q", authorized)
	}
	return nil
}

func validateBoundedSaaSFuzzValues(values []string) error {
	if len(values) == 0 || len(values) > maxBoundedSaaSFuzzValues {
		return utils.Errorf("bounded SaaS HTTP fuzz requires 1-%d literal values", maxBoundedSaaSFuzzValues)
	}
	for _, value := range values {
		if len(value) > maxBoundedSaaSFuzzValueBytes || strings.Contains(value, "{{") || strings.Contains(value, "}}") {
			return utils.Errorf("bounded SaaS HTTP fuzz values must be literal and at most %d bytes", maxBoundedSaaSFuzzValueBytes)
		}
		for _, char := range value {
			if unicode.IsControl(char) {
				return utils.Error("bounded SaaS HTTP fuzz values cannot contain control characters")
			}
		}
	}
	return nil
}

func validateBoundedSaaSGetParamsAction(loop *reactloops.ReActLoop, paramName string, values []string, rawMode bool) error {
	if loop == nil || !isBoundedSaaSHTTPFuzz(loop.GetConfig()) {
		return nil
	}
	if rawMode || !boundedSaaSQueryParamPattern.MatchString(strings.TrimSpace(paramName)) {
		return utils.Error("bounded SaaS HTTP fuzz requires one safe query parameter name and forbids raw query replacement")
	}
	if err := validateBoundedSaaSFuzzValues(values); err != nil {
		return err
	}
	return validateBoundedSaaSHTTPFuzzRequest(loop)
}

func validateBoundedSaaSHeaderAction(loop *reactloops.ReActLoop, headerName string, values []string) error {
	if loop == nil || !isBoundedSaaSHTTPFuzz(loop.GetConfig()) {
		return nil
	}
	if _, ok := boundedSaaSHeaderAllowlist[strings.ToLower(strings.TrimSpace(headerName))]; !ok {
		return utils.Error("bounded SaaS HTTP fuzz header is not in the safe allowlist")
	}
	if err := validateBoundedSaaSFuzzValues(values); err != nil {
		return err
	}
	return validateBoundedSaaSHTTPFuzzRequest(loop)
}

func claimBoundedSaaSHTTPFuzzBatch(loop *reactloops.ReActLoop) error {
	if loop == nil || !isBoundedSaaSHTTPFuzz(loop.GetConfig()) {
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(loop.Get(boundedSaaSHTTPFuzzAttemptedKey)), "true") {
		return utils.Error("bounded SaaS HTTP fuzz permits exactly one mutation batch")
	}
	loop.Set(boundedSaaSHTTPFuzzAttemptedKey, "true")
	return nil
}

func markBoundedSaaSHTTPFuzzCompleted(loop *reactloops.ReActLoop) {
	if loop != nil && isBoundedSaaSHTTPFuzz(loop.GetConfig()) {
		loop.Set(boundedSaaSHTTPFuzzCompletedKey, "true")
	}
}

func validateBoundedSaaSHTTPFuzzCompleted(loop *reactloops.ReActLoop) error {
	if loop == nil || !isBoundedSaaSHTTPFuzz(loop.GetConfig()) {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(loop.Get(boundedSaaSHTTPFuzzCompletedKey)), "true") {
		return utils.Error("bounded SaaS HTTP fuzz cannot finish or publish risks before one mutation batch completes successfully")
	}
	return nil
}

func boundedSaaSHTTPFuzzExecOptions() []mutate.HttpPoolConfigOption {
	return []mutate.HttpPoolConfigOption{
		mutate.WithPoolOpt_Concurrent(1),
		mutate.WithPoolOpt_Timeout(5),
		mutate.WithPoolOpt_DialTimeout(5),
		mutate.WithPoolOpt_NoFollowRedirect(true),
		mutate.WithPoolOpt_RedirectTimes(0),
		mutate.WithPoolOpt_RequestCountLimiter(maxBoundedSaaSFuzzRequests),
		mutate.WithPoolOpt_RetryTimes(0),
		mutate.WithPoolOpt_MaxContentLength(256 * 1024),
		mutate.WithPoolOpt_NoSystemProxy(true),
		mutate.WithPoolOpt_SaveHTTPFlow(false),
	}
}

type boundedSaaSHTTPFuzzAssetPayload struct {
	SchemaVersion       string `json:"schema_version"`
	Source              string `json:"source"`
	Target              string `json:"target"`
	Scheme              string `json:"scheme"`
	Host                string `json:"host"`
	Port                string `json:"port"`
	HTTPURL             string `json:"http_url"`
	Method              string `json:"method"`
	HTTPStatusCode      int    `json:"http_status_code,omitempty"`
	Action              string `json:"action"`
	RequestCount        int    `json:"request_count"`
	SuccessfulResponses int    `json:"successful_responses"`
	RepresentativeCode  int    `json:"representative_status_code,omitempty"`
	RedirectsFollowed   bool   `json:"redirects_followed"`
}

func publishBoundedSaaSHTTPFuzzAsset(
	loop *reactloops.ReActLoop,
	actionName string,
	stats *loopHTTPFuzzOverviewStats,
	representativeStatusCode int,
) error {
	if loop == nil || !isBoundedSaaSHTTPFuzz(loop.GetConfig()) || stats == nil || stats.SuccessfulResponses == 0 {
		return nil
	}
	target, err := authorizedSaaSHTTPFuzzTarget(loop.GetConfig())
	if err != nil {
		return err
	}
	parsedTarget, err := url.Parse(target)
	if err != nil {
		return utils.Wrap(err, "parse bounded SaaS HTTP fuzz asset target")
	}
	port := parsedTarget.Port()
	if port == "" {
		if parsedTarget.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	payload, err := json.Marshal(boundedSaaSHTTPFuzzAssetPayload{
		SchemaVersion:       "1",
		Source:              LoopHTTPFuzztestName,
		Target:              target,
		Scheme:              parsedTarget.Scheme,
		Host:                parsedTarget.Hostname(),
		Port:                port,
		HTTPURL:             target,
		Method:              http.MethodGet,
		HTTPStatusCode:      representativeStatusCode,
		Action:              actionName,
		RequestCount:        stats.TotalRequests,
		SuccessfulResponses: stats.SuccessfulResponses,
		RepresentativeCode:  representativeStatusCode,
		RedirectsFollowed:   false,
	})
	if err != nil {
		return err
	}
	ctx := context.Background()
	if config := loop.GetConfig(); config != nil && config.GetContext() != nil {
		ctx = config.GetContext()
	}
	_, err = aicommon.AssetResultSinkFromConfig(loop.GetConfig()).SubmitAsset(ctx, aicommon.AssetResult{
		Kind:        "http_endpoint",
		Title:       "Bounded HTTP fuzz target " + target,
		Target:      target,
		IdentityKey: "http_endpoint:http_fuzztest:" + target,
		Payload:     payload,
	})
	return err
}

func validateBoundedSaaSHTTPFuzzOutcome(loop *reactloops.ReActLoop, stats *loopHTTPFuzzOverviewStats) error {
	if loop == nil || !isBoundedSaaSHTTPFuzz(loop.GetConfig()) {
		return nil
	}
	if stats == nil || stats.SuccessfulResponses == 0 {
		return utils.Error("bounded SaaS HTTP fuzz completed without a successful response")
	}
	return nil
}

func validateBoundedSaaSRiskTarget(loop *reactloops.ReActLoop, proposed string) (string, error) {
	if loop == nil || !isBoundedSaaSHTTPFuzz(loop.GetConfig()) {
		return strings.TrimSpace(proposed), nil
	}
	authorized, err := authorizedSaaSHTTPFuzzTarget(loop.GetConfig())
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(proposed) == "" {
		return authorized, nil
	}
	normalized, err := normalizeBoundedSaaSHTTPFuzzTarget(proposed)
	if err != nil || normalized != authorized {
		return "", utils.Errorf("risk target must exactly match the server-authorized target %q", authorized)
	}
	return authorized, nil
}
