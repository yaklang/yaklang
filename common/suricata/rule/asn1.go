package rule

import (
	"fmt"
	"strconv"
	"strings"
)

// parseASN1Rule parses the Suricata asn1 keyword options. Options are
// separated by commas or whitespace and may appear in any order.
func parseASN1Rule(raw string) (*ASN1Rule, error) {
	tokens := strings.FieldsFunc(raw, func(value rune) bool {
		switch value {
		case ' ', '\t', '\r', '\n', ',':
			return true
		default:
			return false
		}
	})
	if len(tokens) == 0 {
		return nil, fmt.Errorf("asn1 requires at least one option")
	}
	result := &ASN1Rule{Raw: raw}
	for index := 0; index < len(tokens); index++ {
		token := strings.ToLower(tokens[index])
		switch token {
		case "double_overflow":
			result.DoubleOverflow = true
		case "bitstring_overflow":
			result.BitstringOverflow = true
		case "oversize_length":
			value, next, err := parseASN1Uint(tokens, index, 32)
			if err != nil {
				return nil, fmt.Errorf("asn1 oversize_length: %w", err)
			}
			parsed := uint32(value)
			result.OversizeLength = &parsed
			index = next
		case "absolute_offset":
			value, next, err := parseASN1Uint(tokens, index, 16)
			if err != nil {
				return nil, fmt.Errorf("asn1 absolute_offset: %w", err)
			}
			parsed := uint16(value)
			result.AbsoluteOffset = &parsed
			index = next
		case "relative_offset":
			if index+1 >= len(tokens) {
				return nil, fmt.Errorf("asn1 relative_offset requires a value")
			}
			value, err := strconv.ParseInt(tokens[index+1], 10, 32)
			if err != nil || value < -65535 || value > 65535 {
				return nil, fmt.Errorf("asn1 relative_offset %q must be between -65535 and 65535", tokens[index+1])
			}
			parsed := int(value)
			result.RelativeOffset = &parsed
			index++
		default:
			return nil, fmt.Errorf("asn1 option %q is not supported", tokens[index])
		}
	}
	if result.OversizeLength == nil && result.AbsoluteOffset == nil && result.RelativeOffset == nil &&
		!result.BitstringOverflow && !result.DoubleOverflow {
		return nil, fmt.Errorf("asn1 requires a check or offset option")
	}
	return result, nil
}

func parseASN1Uint(tokens []string, index, bits int) (uint64, int, error) {
	if index+1 >= len(tokens) {
		return 0, index, fmt.Errorf("%s requires a value", tokens[index])
	}
	value, err := strconv.ParseUint(tokens[index+1], 10, bits)
	if err != nil {
		return 0, index, fmt.Errorf("%s must be an unsigned %d-bit integer", tokens[index+1], bits)
	}
	return value, index + 1, nil
}
