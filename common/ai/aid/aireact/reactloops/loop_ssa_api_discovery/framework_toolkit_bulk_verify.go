package loop_ssa_api_discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var httpStatusLineRe = regexp.MustCompile(`(?i)HTTP/1\.[01]\s+(\d{3})`)

func parseHTTPStatusFromToolOutput(content string) int {
	m := httpStatusLineRe.FindStringSubmatch(content)
	if len(m) < 2 {
		return 0
	}
	code, _ := strconv.Atoi(m[1])
	return code
}

func executeProgrammaticHTTPRequest(ctx context.Context, invoker aicommon.AIInvokeRuntime, rt *Runtime, params aitool.InvokeParams) (string, int, error) {
	if invoker == nil {
		return "", 0, utils.Error("nil invoker")
	}
	result, _, err := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, "do_http_request", params)
	if err != nil {
		return "", 0, err
	}
	content := toolResultTextContent(result)
	return content, parseHTTPStatusFromToolOutput(content), nil
}

func buildBulkVerifyURL(rt *Runtime, path string) string {
	if rt == nil || rt.Session == nil {
		return path
	}
	base := EffectiveTargetBaseURL(rt.Session)
	path = normURLPath(path)
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	return strings.TrimRight(base, "/") + path
}

func bulkVerifyQueryFromPath(path string) string {
	path = strings.TrimSpace(path)
	if i := strings.Index(path, "?"); i >= 0 && i+1 < len(path) {
		return path[i+1:]
	}
	return ""
}

func buildMinimalPostParams(params []CombinedAPIParam) string {
	vals := url.Values{}
	for _, p := range params {
		if p.Location != "post" && p.Location != "query" {
			continue
		}
		if p.Name == "" || p.Name == "_csrf" {
			continue
		}
		vals.Set(p.Name, "1")
	}
	return vals.Encode()
}

func buildBulkVerifyParams(rt *Runtime, rec CombinedAPIRecord, credID uint) aitool.InvokeParams {
	params := aitool.InvokeParams{
		"url":    buildBulkVerifyURL(rt, rec.Path),
		"method": rec.Method,
	}
	if q := bulkVerifyQueryFromPath(rec.Path); q != "" {
		params["query-params"] = q
	}
	if credID > 0 {
		params["auth_credential_id"] = credID
	}
	if rec.RequiresCsrf() {
		method := normalizeHTTPMethod(rec.Method)
		if method == "POST" || method == "PUT" || method == "PATCH" || method == "DELETE" {
			params["content-type"] = "application/x-www-form-urlencoded"
			if post := buildMinimalPostParams(rec.Params); post != "" {
				params["post-params"] = post
			}
		}
	}
	return params
}

func hasParamNamed(params []CombinedAPIParam, name string) bool {
	for _, p := range params {
		if p.Name == name {
			return true
		}
	}
	return false
}

func buildMinimalFormBody(params []CombinedAPIParam) string {
	return buildMinimalPostParams(params)
}

func credentialIDForCombinedRecord(rt *Runtime, rec CombinedAPIRecord) uint {
	realm := strings.TrimSpace(rec.Auth.Realm)
	if realm == "" && strings.HasPrefix(rec.Path, "/admin") {
		realm = AuthRealmAdmin
	}
	id, _ := ResolveCredentialIDForProbe(rt, rec.HandlerClass, "", "", realm)
	return id
}

func probeResultFromBulkVerify(rt *Runtime, rec CombinedAPIRecord, content string, status int, credID uint, source string) *ProbeResult {
	path := normURLPath(rec.Path)
	fullURL := buildBulkVerifyURL(rt, path)
	verified := status > 0 && status < 400 && status != 401
	rejectReason := ""
	verdictReason := fmt.Sprintf("framework_toolkit bulk verify status=%d", status)
	if status == 401 || status == 403 {
		verified = false
		if status == 403 && strings.Contains(strings.ToLower(content), "csrf") {
			rejectReason = "csrf_required"
		} else {
			rejectReason = fmt.Sprintf("http_%d", status)
		}
	}
	if status == 404 {
		verified = false
		if rec.RequiresCsrf() {
			rejectReason = "csrf_required"
			if strings.TrimSpace(content) == "" {
				verdictReason = "framework_toolkit bulk verify: @Csrf endpoint returned 404 (route exists; check _csrf)"
			}
		} else {
			rejectReason = "not_found"
		}
	}
	attempt, _ := json.Marshal(map[string]any{"status": status, "source": source})
	pr := &ProbeResult{
		Verified:        verified,
		Method:          rec.Method,
		PathPattern:     path,
		FullSampleURL:   fullURL,
		EffectiveBase:   EffectiveTargetBaseURL(rt.Session),
		ProbeStatusCode: status,
		ResponseExcerpt: utils.ShrinkString(content, 4000),
		VerdictReason:   verdictReason,
		ProbeAttempts:   []json.RawMessage{attempt},
		HandlerFile:     rec.BackendFile,
		HandlerSymbol:   rec.HandlerMethod,
		RejectReason:    rejectReason,
		Source:          source,
		URLSpace:        rec.Auth.Realm,
	}
	if credID > 0 {
		pr.VerdictReason += fmt.Sprintf(" cred_id=%d", credID)
	}
	return pr
}

func runFrameworkToolkitBulkVerify(ctx context.Context, invoker aicommon.AIInvokeRuntime, rt *Runtime, catalog *CombinedAPICatalog, source string) (*ToolkitVerifyReport, error) {
	if rt == nil || catalog == nil {
		return nil, utils.Error("nil runtime or catalog")
	}
	if source == "" {
		source = "framework_toolkit"
	}
	report := &ToolkitVerifyReport{TotalRecords: len(catalog.Records)}
	if !rt.Session.TargetReachable {
		log.Infof("ssa_api_discovery: bulk verify skipped (target unreachable)")
		report.Skipped = len(catalog.Records)
		return report, nil
	}
	n, csrfWarns := PrefetchCsrfTokensForSession(ctx, invoker, rt)
	if n > 0 {
		log.Infof("ssa_api_discovery: bulk verify csrf prefetch cached=%d", n)
	}
	for _, w := range csrfWarns {
		log.Warnf("ssa_api_discovery: bulk verify csrf prefetch: %s", w)
	}
	conc := controllerVerifyConcurrent()
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, rec := range catalog.Records {
		rec := rec
		if destructive, _ := isProbeDestructivePath(rec.Path); destructive {
			report.DestructiveSkip++
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			credID := credentialIDForCombinedRecord(rt, rec)
			params := buildBulkVerifyParams(rt, rec, credID)
			var cred *store.AuthCredential
			if credID > 0 && rt.Repo != nil {
				var err error
				cred, err = rt.Repo.GetAuthCredential(rt.Session.ID, credID)
				if err == nil && cred != nil {
					syncCsrfFromCredentialCookie(rt, cred)
					stripManualCsrfFromParams(params, defaultCsrfParamName)
					applyAuthCredentialToHTTPParams(params, cred)
					applyCachedCsrfForCredentialIfRequired(rt, credID, params, rec.RequiresCsrf())
				}
			}
			params, _ = augmentDoHTTPParams(params)
			content, status, err := executeProgrammaticHTTPRequest(ctx, invoker, rt, params)
			if cred != nil && status > 0 {
				if _, _ = captureCsrfFromHTTPResponse(rt, cred, buildBulkVerifyURL(rt, rec.Path), content); true {
					// refresh csrf cache from probe pages when available
				}
			}
			mu.Lock()
			defer mu.Unlock()
			report.Probed++
			if err != nil {
				report.Errors++
				return
			}
			pr := probeResultFromBulkVerify(rt, rec, content, status, credID, source)
			if _, uerr := UpsertVerifiedHttpApiFromProbeResult(rt, pr); uerr != nil {
				report.Errors++
				return
			}
			if pr.Verified {
				report.Verified++
			} else {
				report.Rejected++
			}
		}()
	}
	wg.Wait()
	log.Infof("ssa_api_discovery: bulk verify probed=%d verified=%d rejected=%d destructive_skip=%d errors=%d",
		report.Probed, report.Verified, report.Rejected, report.DestructiveSkip, report.Errors)
	return report, nil
}
