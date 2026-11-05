package pcaputil

import (
	"errors"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func (e *ProtocolEvent) normalizeKerberosContext() {
	if e.Protocol != "kerberos" || e.Rule != "application-layer.kerberos_fields" || e.nativeDiagnosticError != "" {
		return
	}
	_, err := stream_parser.DecodeKerberosSummaryFacts(e.Raw, e.Entry == "KerberosTCPFields")
	var context *stream_parser.KerberosContextRequired
	if errors.As(err, &context) {
		e.Status, e.sessionError = classifySessionError(err)
		e.Error = ""
		e.Summary = context.Error()
	}
}
