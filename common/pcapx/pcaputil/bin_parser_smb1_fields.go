package pcaputil

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

type smb1NegotiateBoundedReader struct {
	*bytes.Reader
	bits uint64
}

func (r *smb1NegotiateBoundedReader) InputBitLength() uint64 { return r.bits }

// decodeSMB1NegotiateFields runs the existing canonical CIFS request parser
// once. Its successful Node is projected by the same helper as ParseStructured;
// framing, OEM dialect grammar and reply refusal stay in the canonical rule.
func decodeSMB1NegotiateFields(raw []byte, budget ParserBudget) (map[string]any, error) {
	return decodeSMB1CIFSFields(raw, budget, false)
}

func decodeSMB1NegotiateResponseFields(raw []byte, budget ParserBudget) (map[string]any, error) {
	return decodeSMB1CIFSFields(raw, budget, true)
}

func decodeSMB1CIFSFields(raw []byte, budget ParserBudget, response bool) (map[string]any, error) {
	defaults := DefaultParserBudget()
	for _, pair := range [][2]*int{
		{&budget.MaxFrameBytes, &defaults.MaxFrameBytes}, {&budget.MaxMessageBytes, &defaults.MaxMessageBytes},
		{&budget.MaxBufferedBytes, &defaults.MaxBufferedBytes}, {&budget.MaxRecursionDepth, &defaults.MaxRecursionDepth},
		{&budget.MaxCollectionElements, &defaults.MaxCollectionElements}, {&budget.ProbeBytes, &defaults.ProbeBytes},
	} {
		if *pair[0] < 0 {
			return nil, protocolError(ErrResourceExceeded, "CIFS: negative parser budget")
		}
		if *pair[0] == 0 {
			*pair[0] = *pair[1]
		}
	}
	maxSize, depth, entry := 65574, 4, "CIFSDirectTCP"
	if response {
		maxSize, depth, entry = 65608, 3, "CIFSDirectTCPNegotiateExtendedResponse"
	}
	if len(raw) > maxSize || len(raw) > budget.MaxFrameBytes || len(raw) > budget.MaxMessageBytes || len(raw) > budget.MaxBufferedBytes {
		return nil, protocolError(ErrResourceExceeded, "CIFS: bounded profile exceeds byte budget")
	}
	// DirectTCP -> Request -> Dialects -> Dialect -> terminal is the complete
	// canonical profile. No caller budget can turn it into an unbounded tree.
	if budget.MaxRecursionDepth < depth {
		return nil, protocolError(ErrResourceExceeded, "CIFS: selected profile requires structural depth %d", depth)
	}
	r := &smb1NegotiateBoundedReader{Reader: bytes.NewReader(raw), bits: uint64(len(raw)) * 8}
	node, err := parser.ParseBinary(r, "application-layer.cifs", entry)
	if err != nil {
		return nil, err
	}
	if r.Len() != 0 {
		return nil, fmt.Errorf("CIFS: %d unconsumed bounded request bytes", r.Len())
	}
	var fields map[string]any
	if response {
		fields, err = parser.ProjectCIFSNegotiateResponseFields(node, raw)
	} else {
		fields, err = parser.ProjectCIFSNegotiateFields(node, raw, budget.MaxCollectionElements)
	}
	if errors.Is(err, parser.ErrCIFSProjectionBudget) {
		return nil, protocolError(ErrResourceExceeded, "CIFS: dialect collection budget exceeded")
	}
	if err != nil {
		return nil, err
	}
	if sessionSnapshotSize(fields, false) > budget.MaxBufferedBytes {
		return nil, protocolError(ErrResourceExceeded, "CIFS: owned field projection exceeds buffer budget")
	}
	return fields, nil
}
