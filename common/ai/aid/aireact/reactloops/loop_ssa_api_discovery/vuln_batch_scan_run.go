package loop_ssa_api_discovery

import (
	"context"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// VulnBatchScanParams configures the embedded vuln_batch_scan Yak tool.
type VulnBatchScanParams struct {
	ResourceConcurrent int
	HTTPConcurrent     int
	Concurrent         int // legacy alias for HTTPConcurrent
	Timeout            int
	HostThrottleMS     int
	AIConcurrent       int
	AuthCredentialID   uint
	EndpointIDs        string
	APIDesc            string
	SkipAIReview       bool
}

// DefaultVulnBatchScanParams returns sensible defaults for orchestrator-initiated scans.
func DefaultVulnBatchScanParams() VulnBatchScanParams {
	return VulnBatchScanParams{
		ResourceConcurrent: 8,
		HTTPConcurrent:     16,
		Timeout:            12,
		HostThrottleMS:     0,
		AIConcurrent:       6,
	}
}

func resolveVulnScanCredential(ctx context.Context, rt *Runtime, credID uint) (uint, bool) {
	if credID == 0 {
		if defaultCred := GetDefaultCredentialForSession(rt); defaultCred != nil {
			credID = defaultCred.ID
		}
	}
	if credID == 0 || rt == nil || rt.Repo == nil || rt.Session == nil {
		return 0, false
	}
	cred, err := rt.Repo.GetAuthCredential(rt.Session.ID, credID)
	if err != nil || cred == nil || !cred.Verified {
		log.Warnf("ssa_api_discovery: skip auth for vuln scan: credential %d unavailable", credID)
		return 0, false
	}
	SyncCredentialHeaderFields(cred)
	if strings.TrimSpace(cred.HeadersJSON) == "" && strings.TrimSpace(cred.HeaderValue) == "" {
		log.Warnf("ssa_api_discovery: skip auth for vuln scan: credential %d has empty headers", credID)
		return 0, false
	}
	if rerr := EnsureFreshCredential(ctx, rt, credID); rerr != nil {
		log.Warnf("ssa_api_discovery: auth refresh failed, fallback to unauthenticated vuln scan: %v", rerr)
		return 0, false
	}
	return credID, true
}

func appendVulnScanAuthExtras(extra map[string]any, rt *Runtime, sessID uint, credID uint) {
	if credID == 0 || rt == nil || rt.Repo == nil {
		return
	}
	cred, gerr := rt.Repo.GetAuthCredential(sessID, credID)
	if gerr != nil || cred == nil {
		return
	}
	SyncCredentialHeaderFields(cred)
	if authArg := BuildAuthHeaderCLIArg(cred); authArg != "" {
		extra["auth-header"] = authArg
	}
	if paths, setupErr := SetupAuthRefreshPaths(rt, credID); setupErr == nil {
		_ = WriteAuthHeadersFile(cred, paths.HeadersFile)
		extra["auth-headers-file"] = paths.HeadersFile
		extra["auth-refresh-trigger-file"] = paths.TriggerFile
	}
}

func invokeVulnBatchScan(ctx context.Context, invoker aicommon.AIInvokeRuntime, rt *Runtime, params VulnBatchScanParams, credID uint) (string, error) {
	sess := rt.Session
	base := EffectiveTargetBaseURL(sess)
	httpConc := params.HTTPConcurrent
	if httpConc <= 0 {
		httpConc = params.Concurrent
	}
	if httpConc <= 0 {
		httpConc = 16
	}
	resConc := params.ResourceConcurrent
	if resConc <= 0 {
		resConc = 8
	}

	extra := map[string]any{
		"base-url":            base,
		"session-uuid":        sess.UUID,
		"resource-concurrent": resConc,
		"http-concurrent":     httpConc,
		"concurrent":          httpConc,
		"timeout":             params.Timeout,
		"host-throttle-ms":    params.HostThrottleMS,
		"ai-concurrent":       params.AIConcurrent,
	}
	if dsn := strings.TrimSpace(rt.SessionDBDSN); dsn != "" {
		extra["db-dsn"] = dsn
	} else if rt.SQLitePath != "" {
		extra["sqlite-path"] = rt.SQLitePath
	}
	if params.EndpointIDs != "" {
		extra["endpoint-ids"] = params.EndpointIDs
	} else if targets, terr := ListProbeTargets(rt); terr == nil && len(targets) > 0 {
		if ids := HttpEndpointIDsFromProbeTargets(rt, targets); len(ids) > 0 {
			extra["endpoint-ids"] = JoinUintCSV(ids)
		}
	}
	if params.APIDesc != "" {
		extra["api-desc"] = params.APIDesc
	}
	if params.SkipAIReview {
		extra["skip-ai-review"] = true
	}
	appendVulnScanAuthExtras(extra, rt, sess.ID, credID)

	result, _, err := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, ToolVulnBatchScan, extra)
	content := toolResultTextContent(result)
	return content, err
}

// RunVulnBatchScan executes the embedded vuln_batch_scan tool for the session.
func RunVulnBatchScan(ctx context.Context, invoker aicommon.AIInvokeRuntime, rt *Runtime, params VulnBatchScanParams) (summary string, err error) {
	if rt == nil || rt.Session == nil {
		return "", utils.Error("nil runtime")
	}
	sess := rt.Session
	if EffectiveTargetBaseURL(sess) == "" {
		return "", utils.Error("target base URL not set")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	credID, authUsed := resolveVulnScanCredential(ctx, rt, params.AuthCredentialID)
	content, err := invokeVulnBatchScan(ctx, invoker, rt, params, credID)
	if err != nil {
		return "", err
	}

	dynFindings, _ := rt.Repo.ListDynamicVulnFindings(sess.ID)
	if authUsed && len(dynFindings) == 0 {
		log.Infof("ssa_api_discovery: authenticated greybox scan produced 0 findings, retry unauthenticated")
		content2, err2 := invokeVulnBatchScan(ctx, invoker, rt, params, 0)
		if err2 != nil {
			log.Warnf("ssa_api_discovery: unauthenticated vuln scan retry failed: %v", err2)
		} else {
			content = content2
			dynFindings, _ = rt.Repo.ListDynamicVulnFindings(sess.ID)
		}
	}

	summary = fmt.Sprintf("vuln_batch_scan complete. dynamic_findings=%d auth_mode=%s\n%s",
		len(dynFindings), vulnScanAuthMode(authUsed, credID), utils.ShrinkString(content, 4000))
	if bridged, berr := BridgeAllConfirmedDynamicFindings(rt); berr != nil {
		log.Warnf("ssa_api_discovery: bridge dynamic findings: %v", berr)
	} else if bridged > 0 {
		summary += fmt.Sprintf("\nbridged_to_vuln_verifications=%d", bridged)
	}
	return summary, nil
}

func vulnScanAuthMode(authUsed bool, credID uint) string {
	if !authUsed || credID == 0 {
		return "none"
	}
	return fmt.Sprintf("credential_%d", credID)
}
