package sharkcli

import (
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"strings"
)

// A retained PDU may arrive after its byte-contributing packet was classified.
// The caption must describe the same snapshot as the field tree, even in the
// quick view before the worker finishes. Preserve the frozen carrier caption
// when no qualified application observation remains (including history loss).
func inspectionProtocol(raw *capturedPacket, events []*pcaputil.ProtocolEvent, fallback string) string {
	if raw.history != nil {
		if current := presentEvents(events); current.protocol != "" {
			// A short CCS or otherwise unconfirmed record does not erase the
			// captured connection evidence already used by the packet row.
			// Preserve its qualified carrier/version without upgrading the
			// record's fields, authentication, or a port-only association.
			if (current.protocol == "TLS?" || current.status == "record-only") &&
				(fallback == "TLS" || strings.HasPrefix(fallback, "TLSv")) && hasObservedTLS(raw) {
				return fallback
			}
			return current.protocol
		}
	}
	return fallback
}
