package loop_ssa_api_discovery

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func pinHTTPParamsToProbeTarget(params aitool.InvokeParams, target *HttpProbeTarget) (aitool.InvokeParams, []string) {
	if target == nil || params == nil {
		return params, nil
	}
	canonicalURL := strings.TrimSpace(target.FullSampleURL)
	canonicalMethod := strings.ToUpper(strings.TrimSpace(target.Method))
	if canonicalURL == "" && canonicalMethod == "" {
		return params, nil
	}
	var notes []string
	if canonicalMethod != "" {
		method := strings.ToUpper(strings.TrimSpace(pickStringParam(params, "method")))
		if method == "" {
			method = "GET"
		}
		if method != canonicalMethod {
			params["method"] = canonicalMethod
			notes = append(notes, fmt.Sprintf("corrected method %s -> %s for pinned endpoint id=%d", method, canonicalMethod, target.VerifiedHttpApiID))
		}
	}
	if canonicalURL == "" {
		return params, notes
	}
	reqURL := strings.TrimSpace(pickStringParam(params, "url"))
	if reqURL == "" {
		params["url"] = canonicalURL
		notes = append(notes, fmt.Sprintf("filled url from pinned endpoint id=%d", target.VerifiedHttpApiID))
		return params, notes
	}
	if urlsEquivalentForProbe(reqURL, canonicalURL) {
		return params, notes
	}
	params["url"] = canonicalURL
	notes = append(notes, fmt.Sprintf("corrected url %q -> %q (use verified_http_apis.full_sample_url for id=%d; do not guess .html suffix or controller paths)", reqURL, canonicalURL, target.VerifiedHttpApiID))
	return params, notes
}

func urlsEquivalentForProbe(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == b {
		return true
	}
	pa, errA := url.Parse(a)
	pb, errB := url.Parse(b)
	if errA != nil || errB != nil {
		return false
	}
	if !strings.EqualFold(pa.Scheme, pb.Scheme) || !strings.EqualFold(pa.Host, pb.Host) {
		return false
	}
	return normURLPath(pa.Path) == normURLPath(pb.Path) && pa.RawQuery == pb.RawQuery
}
