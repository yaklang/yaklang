package stream_parser

import "github.com/yaklang/yaklang/common/bin-parser/parser/base"

// These are complete, byte-for-byte source matches for three read-only IEC
// result expressions. This is not an interpreter or a schema shortcut: parsing
// and the unformatted Result have already run normally. A source edit or an
// unexpected Go value type takes the ordinary expression evaluator unchanged.
const (
	outScalarNumberSource = "return data.Value[2].Value\n"
	outScalarBytesSource  = "if len(data.Value) > 2 { return data.Value[2].Value }\nreturn \"\"\n"
	outScalarLengthSource = "result = 0\nfor index, octet = range data.Value {\n  if index == 0 {\n    if octet.Value < 128 { return octet.Value }\n  } else { result = (result << 8) + octet.Value }\n}\nreturn result\n"
)

// evalScalarOut is only called with the fixed bindings of a fresh ExecOut.
// No engine, NodeValue, byte slice or result is retained across invocations.
// The input's field values retain the ordinary evaluator's reference semantics;
// ExecOut still creates the outer NodeValue and restores expression config.
func evalScalarOut(source string, data *base.NodeValue) (any, bool) {
	if data == nil {
		return nil, false
	}
	values, ok := data.Value.([]*base.NodeValue)
	if !ok {
		return nil, false
	}
	switch source {
	case outScalarNumberSource:
		if len(values) < 3 || values[2] == nil {
			return nil, false
		}
		value, ok := values[2].Value.(uint64)
		return value, ok
	case outScalarBytesSource:
		if len(values) <= 2 {
			return "", true
		}
		if values[2] == nil {
			return nil, false
		}
		value, ok := values[2].Value.([]byte)
		return value, ok
	case outScalarLengthSource:
		if len(values) == 0 || values[0] == nil {
			return nil, false
		}
		first, ok := values[0].Value.(uint8)
		if ok && first < 128 {
			// Preserve the uint8 type and the expression's immediate return,
			// including its deliberate non-inspection of any later elements.
			return first, true
		}
		if !ok || first < 0x81 || first > 0x84 || len(values) != int(first&0x7f)+1 {
			return nil, false
		}
		var length uint64
		for _, octet := range values[1:] {
			if octet == nil {
				return nil, false
			}
			value, ok := octet.Value.(uint8)
			if !ok {
				return nil, false
			}
			length = length<<8 | uint64(value)
		}
		// The Yak fold starts at the literal int(0). Its integer operators
		// normalize positive results above the host's MaxInt to int64; they
		// do not return the uint64 type used by IECNumber. At most four octets
		// keeps every intermediate value nonnegative and within int64.
		if length > uint64(^uint(0)>>1) {
			return int64(length), true
		}
		return int(length), true
	}
	return nil, false
}
