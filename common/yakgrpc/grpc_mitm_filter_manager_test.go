package yakgrpc

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestExpandGroupByDelimiter(t *testing.T) {
	tests := []struct {
		name  string
		group []string
		want  []string
	}{
		{
			name:  "comma separated",
			group: []string{"example.com,test.com"},
			want:  []string{"example.com", "test.com"},
		},
		{
			name:  "semicolon separated",
			group: []string{"example.com;test.com"},
			want:  []string{"example.com", "test.com"},
		},
		{
			name:  "newline separated",
			group: []string{"example.com\ntest.com"},
			want:  []string{"example.com", "test.com"},
		},
		{
			name:  "CRLF separated",
			group: []string{"example.com\r\ntest.com"},
			want:  []string{"example.com", "test.com"},
		},
		{
			name:  "comma with spaces",
			group: []string{"example.com, test.com"},
			want:  []string{"example.com", "test.com"},
		},
		{
			name:  "mixed delimiters",
			group: []string{"example.com,test.com;foo.com\nbar.com"},
			want:  []string{"example.com", "test.com", "foo.com", "bar.com"},
		},
		{
			name:  "single value no delimiter",
			group: []string{"example.com"},
			want:  []string{"example.com"},
		},
		{
			name:  "already split array",
			group: []string{"example.com", "test.com"},
			want:  []string{"example.com", "test.com"},
		},
		{
			name:  "trailing delimiter",
			group: []string{"example.com,test.com,"},
			want:  []string{"example.com", "test.com"},
		},
		{
			name:  "consecutive delimiters",
			group: []string{"example.com,,,test.com"},
			want:  []string{"example.com", "test.com"},
		},
		{
			name:  "glob pattern with comma",
			group: []string{"*.example.com,*.test.com"},
			want:  []string{"*.example.com", "*.test.com"},
		},
		{
			name:  "glob pattern with semicolon",
			group: []string{"*.example.com;*.test.com"},
			want:  []string{"*.example.com", "*.test.com"},
		},
		{
			name:  "glob pattern with newline",
			group: []string{"*.example.com\n*.test.com"},
			want:  []string{"*.example.com", "*.test.com"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandGroupByDelimiter(tt.group)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestFilterDataToMatchers_HostnamesDelimiterSupport(t *testing.T) {
	// In the real MITM flow, the hostname passed to the matcher includes
	// the port (e.g. "example.com:443"). Users typically use glob patterns
	// like "*.example.com*" to match host:port. This test verifies that
	// comma, semicolon, and newline all correctly split a single Group
	// entry into multiple independent glob patterns.
	//
	// Before the fix, only comma worked; semicolon and newline left the
	// entire string as one glob, so nothing matched and all traffic was
	// filtered out.
	tests := []struct {
		name     string
		group    []string
		host     string
		expectOK bool
	}{
		// comma — always worked
		{"comma: match first", []string{"*example.com*,*test.com*"}, "example.com:443", true},
		{"comma: match second", []string{"*example.com*,*test.com*"}, "test.com:443", true},
		{"comma: no match", []string{"*example.com*,*test.com*"}, "other.com:443", false},

		// semicolon — broken before fix
		{"semicolon: match first", []string{"*example.com*;*test.com*"}, "example.com:443", true},
		{"semicolon: match second", []string{"*example.com*;*test.com*"}, "test.com:443", true},
		{"semicolon: no match", []string{"*example.com*;*test.com*"}, "other.com:443", false},

		// newline — broken before fix
		{"newline: match first", []string{"*example.com*\n*test.com*"}, "example.com:443", true},
		{"newline: match second", []string{"*example.com*\n*test.com*"}, "test.com:443", true},
		{"newline: no match", []string{"*example.com*\n*test.com*"}, "other.com:443", false},

		// CRLF — broken before fix
		{"CRLF: match first", []string{"*example.com*\r\n*test.com*"}, "example.com:443", true},
		{"CRLF: match second", []string{"*example.com*\r\n*test.com*"}, "test.com:443", true},

		// mixed delimiters
		{"mixed: comma+semicolon+newline", []string{"*a.com*,*b.com*;*c.com*\n*d.com*"}, "c.com:443", true},
		{"mixed: match d.com", []string{"*a.com*,*b.com*;*c.com*\n*d.com*"}, "d.com:443", true},

		// plain hostname glob (no port in target)
		{"plain hostname comma", []string{"example.com,test.com"}, "example.com", true},
		{"plain hostname semicolon", []string{"example.com;test.com"}, "test.com", true},
		{"plain hostname newline", []string{"example.com\ntest.com"}, "test.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := FilterDataToMatchers([]*ypb.FilterDataItem{{
				MatcherType: httptpl.MATCHER_TYPE_GLOB,
				Group:       tt.group,
			}}, true)
			require.NotNil(t, matcher, "matcher should not be nil")

			result, err := matcher.ExecuteRaw([]byte(tt.host), nil)
			require.NoError(t, err)
			require.Equal(t, tt.expectOK, result)
		})
	}
}

// TestFilterDataToMatchers_NoExpand verifies that when expandCommaSeparated
// is false (the default for suffix/URI/method/MIME), the group is NOT split.
func TestFilterDataToMatchers_NoExpand(t *testing.T) {
	matcher := FilterDataToMatchers([]*ypb.FilterDataItem{{
		MatcherType: httptpl.MATCHER_TYPE_GLOB,
		Group:       []string{"a.com,b.com"},
	}}, false) // no expand
	require.NotNil(t, matcher)

	// Without expand, "a.com,b.com" is treated as a single glob — the comma
	// is a literal character in the glob, so it won't match "a.com" alone.
	result, err := matcher.ExecuteRaw([]byte("a.com"), nil)
	require.NoError(t, err)
	require.False(t, result, "without expand, comma is literal in glob")
}
