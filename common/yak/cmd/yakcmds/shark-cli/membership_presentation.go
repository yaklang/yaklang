package sharkcli

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func membershipEventProtocol(e *pcaputil.ProtocolEvent) string {
	if e.Protocol == "icmpv6" && e.Profile == "icmpv6-mld" {
		return "MLD"
	}
	if e.Protocol == "igmp" && e.Profile == "igmp-membership-wire" {
		return "IGMP"
	}
	return ""
}

func membershipEventSummary(e *pcaputil.ProtocolEvent) string {
	kind := membershipEventProtocol(e)
	m, _ := eventValues(e)[kind].(map[string]any)
	parts := []string{safeText(e.Summary)}
	recordKey, countKey := "Group Records", "Group Record Count"
	if kind == "MLD" {
		recordKey, countKey = "Address Records", "Address Record Count"
	}
	if count, present := m[countKey]; present {
		parts = append(parts, fmt.Sprintf("records=%d", number(count)))
		if records := mapList(m[recordKey]); len(records) > 0 {
			if address := textValue(records[0]["Multicast Address"]); address != "" {
				parts = append(parts, "first group="+address)
			}
		}
	}
	for _, key := range []string{"Group Address", "Multicast Address"} {
		if address := textValue(m[key]); address != "" {
			parts = append(parts, "group="+address)
		}
	}
	if count, present := m["Source Count"]; present {
		parts = append(parts, fmt.Sprintf("sources=%d", number(count)))
	}
	return strings.Join(parts, " · ")
}
