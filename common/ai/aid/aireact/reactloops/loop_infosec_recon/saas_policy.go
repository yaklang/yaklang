package loop_infosec_recon

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const boundedSaaSReconInstruction = `You are a bounded SaaS business-service verification assistant.
Operate only on the single http or https URL explicitly supplied by the user.
Call recon_register_seed exactly once with that URL. Do not invent scope_hosts or widen the target.
Then call probe_api_candidates exactly once. The probe is restricted to one HEAD request, does not follow redirects, publishes the verified endpoint to the SaaS asset sink, and ends the run.
No crawling, port discovery, local-file access, arbitrary HTTP requests, or target expansion is permitted.`

type reconProbeSettings struct {
	limit           int
	concurrency     int
	useHead         bool
	timeout         time.Duration
	followRedirects bool
}

func isBoundedSaaSRecon(config aicommon.AICallerConfigIf) bool {
	return config != nil && aicommon.AssetResultSinkFromConfig(config) != nil
}

func infosecReconAllowedActions(saasMode, allowUserInteraction bool) []string {
	if saasMode {
		return []string{
			"recon_register_seed",
			"probe_api_candidates",
		}
	}

	allowed := []string{
		schema.AI_REACT_LOOP_ACTION_DIRECTLY_ANSWER,
		"finish",
		schema.AI_REACT_LOOP_ACTION_KNOWLEDGE_ENHANCE,
		schema.AI_REACT_LOOP_ACTION_SEARCH_CAPABILITIES,
		schema.AI_REACT_LOOP_ACTION_LOAD_CAPABILITY,
		schema.AI_REACT_LOOP_ACTION_LOADING_SKILLS,
		schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES,
		schema.AI_REACT_LOOP_ACTION_CHANGE_SKILL_VIEW_OFFSET,
		"recon_register_seed",
		"api_pool_merge",
		ToolCrawlJsCollector,
		ToolJsStaticExtractAI,
		"probe_api_candidates",
		"web_search",
		"scan_port",
		"simple_crawler",
		"banner_grab",
		"dig",
		"do_http_request",
		"batch_do_http_request",
		"read_file",
		"find_files",
		"grep_text",
		"url_content_summary",
		"subdomain_scan",
		"network_space_search",
		"search_knowledge",
	}
	if allowUserInteraction {
		allowed = append(allowed, schema.AI_REACT_LOOP_ACTION_ASK_FOR_CLARIFICATION)
	}
	return allowed
}

func normalizeSaaSReconScope(seedURL, requestedScope string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(seedURL))
	if err != nil {
		return "", err
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return "", utils.Error("SaaS recon seed URL is missing a host")
	}

	requested := ParseScopeHostSet(requestedScope)
	if len(requested) == 0 {
		return host, nil
	}
	if len(requested) != 1 || !requested[host] {
		return "", utils.Errorf(
			"SaaS recon scope cannot widen beyond seed host %q",
			host,
		)
	}
	return host, nil
}

func boundedSaaSReconProbeSettings(
	_ int,
	_ int,
	_ bool,
	requestedTimeout time.Duration,
) reconProbeSettings {
	if requestedTimeout <= 0 || requestedTimeout > 5*time.Second {
		requestedTimeout = 5 * time.Second
	}
	return reconProbeSettings{
		limit:           1,
		concurrency:     1,
		useHead:         true,
		timeout:         requestedTimeout,
		followRedirects: false,
	}
}

func addSaaSReconSeedCandidate(
	pool *APIPool,
	seedURL string,
	scopeHost string,
) (int, []string) {
	return MergeFindings(pool, seedURL, []struct {
		URL        string
		Method     string
		Source     string
		Evidence   string
		Confidence float64
	}{
		{
			URL:        seedURL,
			Method:     http.MethodGet,
			Source:     "saas_declared_seed",
			Evidence:   "exact user-declared URL",
			Confidence: 1,
		},
	}, scopeHost)
}
