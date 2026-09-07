package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeAuditNodeString(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"valid ascii", "hello", "hello"},
		{"valid utf8", "\u4e2d\u6587\u503c", "\u4e2d\u6587\u503c"},
		{"truncated multibyte lead", "abc\xe2", "abc\ufffd"},
		{"truncated multibyte continuation", "abc\x80", "abc\ufffd"},
		{"invalid latin1", "caf\xe9", "caf\ufffd"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, sanitizeAuditNodeString(tc.input))
		})
	}
}
