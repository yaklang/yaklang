package bin_parser

import "github.com/yaklang/yaklang/common/bin-parser/parser"

// StructuredPlan is a reusable exact-message plan; see parser.StructuredPlan.
type StructuredPlan = parser.StructuredPlan

// PrepareStructured resolves an explicit entry after detection and framing.
func PrepareStructured(rule, entry string) (*StructuredPlan, error) {
	return parser.PrepareStructured(rule, entry)
}

// ParseStructured parses one complete, explicitly bounded message into owned
// fields and metadata. It does not serialize JSON or infer a protocol/session.
// See parser.ParseStructured for the fast-path and compatibility behavior.
func ParseStructured(data []byte, rule string, keys ...string) (map[string]any, error) {
	return parser.ParseStructured(data, rule, keys...)
}

// ParseStructuredWithConfig preserves explicit per-message framing context.
func ParseStructuredWithConfig(data []byte, rule string, config map[string]any, keys ...string) (map[string]any, error) {
	return parser.ParseStructuredWithConfig(data, rule, config, keys...)
}
