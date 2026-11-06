package pcaputil

import (
	"errors"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func (e *ProtocolEvent) normalizeKerberosContext() {
	if e.kerberosClassified || e.Protocol != "kerberos" || e.Rule != "application-layer.kerberos_fields" || e.nativeDiagnosticError != "" {
		return
	}
	e.kerberosClassified = true
	if metadata, ok := e.Metadata.(map[string]any); ok && (e.Status == "decoded" || e.Status == "deferred") {
		if facts, ok := metadata["Semantic Summary"].(map[string]any); ok {
			e.kerberosSummaryFacts = cloneSession(facts)
			return
		}
	}
	facts, err := stream_parser.DecodeKerberosSummaryFacts(e.Raw, e.Entry == "KerberosTCPFields")
	if err == nil && (e.Status == "decoded" || e.Status == "deferred") {
		e.kerberosSummaryFacts = facts
	}
	var context *stream_parser.KerberosContextRequired
	if errors.As(err, &context) {
		e.Status, e.sessionError = classifySessionError(err)
		e.Error = ""
		e.Summary = context.Error()
	} else if err != nil && e.Status == "deferred" {
		e.Status, e.sessionError = classifySessionError(err)
		e.Error = err.Error()
	}
}
