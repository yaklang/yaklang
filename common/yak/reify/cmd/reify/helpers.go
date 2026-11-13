package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func splitPassNames(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if name := strings.TrimSpace(part); name != "" {
			result = append(result, name)
		}
	}
	return result
}

func targetSummary(target core.Target) string {
	summary := fmt.Sprintf("%s/%s, %d-bit, %s-endian", target.OS, target.Architecture, target.Bits, target.ByteOrder)
	if target.ABI != "" {
		summary += ", abi " + target.ABI
	}
	if target.Machine != "" {
		summary += ", machine " + target.Machine
	}
	if target.Mode != "" {
		summary += ", mode " + target.Mode
	}
	return summary
}

func parserSummary(parser *core.ParserInfo) string {
	if parser == nil {
		return "none"
	}
	return fmt.Sprintf("%s (%s)", parser.Name, decoderState(parser.Available))
}

func decoderState(available bool) string {
	if available {
		return "available"
	}
	return "unavailable"
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
