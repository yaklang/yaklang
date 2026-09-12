package parser

import (
	"fmt"
	"path"
	"strings"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// StructuredPlan is an immutable exact-message execution plan. It can be shared
// by workers and held by a flow's routing state. It never retains input, results,
// a VM, or a negotiated session. A protocol family alone is insufficient: entry
// includes the explicit phase, direction, version and capability assumptions.
type StructuredPlan struct {
	rule, entry string
	decode      stream_parser.StructuredDecoder
}

// PrepareStructured resolves one audited embedded entry once, for use AFTER
// detection and framing. Unsupported entries return an error; ParseStructured
// remains available for all rules. Preparing a plan does not validate a flow's
// protocol or negotiate a session, and does not parse/cache a sample result.
func PrepareStructured(rule, entry string) (*StructuredPlan, error) {
	parts := strings.Split(rule, ".")
	parts[len(parts)-1] += ".yaml"
	decode := structuredDecoder(path.Join(parts...), entry)
	if decode == nil {
		return nil, fmt.Errorf("structured plan: unsupported exact entry %s/%s", rule, entry)
	}
	return &StructuredPlan{rule: rule, entry: entry, decode: decode}, nil
}

// Parse validates and decodes ONE complete message into independently owned
// fields and metadata. Reassembled TCP chunks must first be framed into the
// boundary required by this entry (some entries describe a message block).
//
// Unlike ParseStructured, invalid wire returns the native validation error
// directly, without rerunning a failed parse through the VM for Yak source
// diagnostics. No partial result is published. The caller decides whether to
// retain, switch or invalidate affinity; a failure never silently passes as raw
// data. Custom parser registrations still execute through the ordinary path.
func (p *StructuredPlan) Parse(data []byte) (map[string]any, error) {
	if p == nil || p.decode == nil {
		return nil, fmt.Errorf("structured plan: uninitialized plan")
	}
	if base.ParserRegistration("default") != defaultParserRegistration {
		return parseStructuredFallback(data, p.rule, p.entry)
	}
	fields, metadata, err := p.decode(data)
	if err != nil {
		return nil, err
	}
	return map[string]any{"fields": fields, "metadata": metadata}, nil
}
