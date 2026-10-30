package yakit

import (
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

// Browser identity hues follow YTray's six-color cycle, expressed using the
// existing HTTPFlow color tags so released Yakit clients can render them.
func browserInstanceHTTPFlowColor(badge string) string {
	badge = strings.TrimSpace(badge)
	if len(badge) == 0 || len(badge) > 2 {
		return ""
	}
	ordinal := 0
	for _, letter := range badge {
		if letter < 'A' || letter > 'Z' {
			return ""
		}
		ordinal = ordinal*26 + int(letter-'A') + 1
	}
	palette := [...]string{
		schema.FLOW_COLOR_ORANGE, schema.FLOW_COLOR_BLUE, schema.FLOW_COLOR_GREEN,
		schema.FLOW_COLOR_PURPLE, schema.FLOW_COLOR_RED, schema.FLOW_COLOR_CYAN,
	}
	return palette[(ordinal-1)%len(palette)]
}

// AppendBrowserInstanceHTTPFlowTags adds a default browser color while keeping
// explicit user/rule colors (including TrafficGuard findings) authoritative.
func AppendBrowserInstanceHTTPFlowTags(tags []string, identity httpctx.BrowserInstance) []string {
	label := identity.HTTPFlowTag()
	if label == "" {
		return tags
	}
	// Parallel requests can share the configured tag slice. Keep each response's
	// browser identity independent even when that slice has spare capacity.
	tags = append([]string(nil), tags...)
	hasLabel, hasColor := false, false
	for _, tag := range tags {
		hasLabel = hasLabel || tag == label
		hasColor = hasColor || strings.HasPrefix(strings.ToUpper(tag), schema.COLORPREFIX)
	}
	if !hasLabel {
		tags = append(tags, label)
	}
	if color := browserInstanceHTTPFlowColor(identity.Badge); !hasColor && color != "" {
		tags = append(tags, color)
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
