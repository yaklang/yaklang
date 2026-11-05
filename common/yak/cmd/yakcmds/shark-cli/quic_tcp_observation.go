package sharkcli

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func unsupportedQUICTCPCarrier(e *pcaputil.ProtocolEvent) bool {
	return e != nil && strings.EqualFold(e.Transport, "tcp") &&
		(e.Protocol == "quic" || e.Protocol == "http3" || e.Protocol == "doq")
}

// Old snapshots or caller-supplied events may contain the former unsupported
// TCP candidate. Preserve its owned bytes and diagnostic without projecting a
// QUIC grammar, TLS tree, authentication claim, or decoded-PDU completion.
func appendUnsupportedQUICTCPCarrier(out *[]field, sources *[]detailSource, e *pcaputil.ProtocolEvent) {
	root := field{text: fmt.Sprintf("TCP payload · protocol unconfirmed · observation #%d", e.ID), branch: true}
	if len(e.Raw) > 0 {
		*sources = append(*sources, detailSource{Name: "Unidentified TCP payload · observed stream bytes", Data: bytes.Clone(e.Raw)})
		root.source, root.hasRange, root.end = len(*sources), true, len(e.Raw)
	}
	*out = append(*out, root,
		field{depth: 1, text: "Rejected candidate: " + eventProtocolName(e) + " · UDP carrier required"},
		field{depth: 1, text: fmt.Sprintf("Observed TCP bytes: %d; application identity unconfirmed", len(e.Raw))},
	)
	if e.Status != "" {
		*out = append(*out, field{depth: 1, text: "Candidate status: " + clipText(e.Status, 256)})
	}
	if e.Summary != "" {
		*out = append(*out, field{depth: 1, text: "Candidate diagnostic: " + clipText(e.Summary, 2048)})
	}
	if e.Error != "" && e.Error != e.Summary {
		*out = append(*out, field{depth: 1, text: "Candidate error: " + clipText(e.Error, 2048)})
	}
}
