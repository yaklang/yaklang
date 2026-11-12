package s390x

import "fmt"

// fmtSscanf parses one two-digit hex byte.
func fmtSscanf(text string, value *byte) (int, error) {
	return fmt.Sscanf(text, "%02x", value)
}
