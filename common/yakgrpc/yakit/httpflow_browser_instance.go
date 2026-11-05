package yakit

import (
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

// AppendBrowserInstanceHTTPFlowTags records browser attribution without assigning
// a row color. Existing user/rule colors (including TrafficGuard findings) remain intact.
func AppendBrowserInstanceHTTPFlowTags(tags []string, identity httpctx.BrowserInstance) []string {
	label := identity.HTTPFlowTag()
	if label == "" {
		return tags
	}
	// Parallel requests can share the configured tag slice. Keep each response's
	// browser identity independent even when that slice has spare capacity.
	tags = append([]string(nil), tags...)
	hasLabel := false
	for _, tag := range tags {
		hasLabel = hasLabel || tag == label
	}
	if !hasLabel {
		tags = append(tags, label)
	}
	return tags
}

func ApplyBrowserInstanceToHTTPFlow(flow *schema.HTTPFlow, identity httpctx.BrowserInstance) {
	if flow == nil || identity.HTTPFlowTag() == "" {
		return
	}
	var tags []string
	if flow.Tags != "" {
		tags = strings.Split(flow.Tags, "|")
	}
	flow.Tags = strings.Join(AppendBrowserInstanceHTTPFlowTags(tags, identity), "|")
}
