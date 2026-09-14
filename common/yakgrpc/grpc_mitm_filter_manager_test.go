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

// TestFilterDataToMatchers_AllFilterTypesDelimiterSupport verifies that all
// MITM filter types (suffix, URI, methods, MIME) correctly split multi-value
// input on comma, semicolon, and newline — matching the UI's promise that
// "values can be separated by comma, semicolon, or newline".
func TestFilterDataToMatchers_AllFilterTypesDelimiterSupport(t *testing.T) {
	type tc struct {
		name        string
		matcherType string
		group       []string
		target      string
		expectOK    bool
	}

	// SUFFIX matcher: checks if target ends with each pattern
	suffixTests := []tc{
		{"suffix comma: match .js", httptpl.MATCHER_TYPE_SUFFIX, []string{".js,.css"}, "/test.js", true},
		{"suffix comma: match .css", httptpl.MATCHER_TYPE_SUFFIX, []string{".js,.css"}, "/test.css", true},
		{"suffix comma: no match", httptpl.MATCHER_TYPE_SUFFIX, []string{".js,.css"}, "/test.html", false},
		{"suffix semicolon: match .js", httptpl.MATCHER_TYPE_SUFFIX, []string{".js;.css"}, "/test.js", true},
		{"suffix semicolon: match .css", httptpl.MATCHER_TYPE_SUFFIX, []string{".js;.css"}, "/test.css", true},
		{"suffix newline: match .js", httptpl.MATCHER_TYPE_SUFFIX, []string{".js\n.css"}, "/test.js", true},
		{"suffix newline: match .css", httptpl.MATCHER_TYPE_SUFFIX, []string{".js\n.css"}, "/test.css", true},
		{"suffix single: match", httptpl.MATCHER_TYPE_SUFFIX, []string{".js"}, "/test.js", true},
	}

	// GLOB matcher for URI
	uriTests := []tc{
		{"uri glob comma: match /api", httptpl.MATCHER_TYPE_GLOB, []string{"/api/*,/admin/*"}, "/api/users", true},
		{"uri glob comma: match /admin", httptpl.MATCHER_TYPE_GLOB, []string{"/api/*,/admin/*"}, "/admin/x", true},
		{"uri glob comma: no match", httptpl.MATCHER_TYPE_GLOB, []string{"/api/*,/admin/*"}, "/other/x", false},
		{"uri glob semicolon: match /api", httptpl.MATCHER_TYPE_GLOB, []string{"/api/*;/admin/*"}, "/api/users", true},
		{"uri glob semicolon: match /admin", httptpl.MATCHER_TYPE_GLOB, []string{"/api/*;/admin/*"}, "/admin/x", true},
		{"uri glob newline: match /api", httptpl.MATCHER_TYPE_GLOB, []string{"/api/*\n/admin/*"}, "/api/users", true},
		{"uri glob newline: match /admin", httptpl.MATCHER_TYPE_GLOB, []string{"/api/*\n/admin/*"}, "/admin/x", true},
	}

	// WORD (contains) matcher for URI
	wordTests := []tc{
		{"uri word comma: match abc", httptpl.MATCHER_TYPE_WORD, []string{"abc,xyz"}, "/path/abc", true},
		{"uri word comma: match xyz", httptpl.MATCHER_TYPE_WORD, []string{"abc,xyz"}, "/path/xyz", true},
		{"uri word semicolon: match abc", httptpl.MATCHER_TYPE_WORD, []string{"abc;xyz"}, "/path/abc", true},
		{"uri word newline: match xyz", httptpl.MATCHER_TYPE_WORD, []string{"abc\nxyz"}, "/path/xyz", true},
	}

	// GLOB matcher for Methods
	methodTests := []tc{
		{"method glob comma: match GET", httptpl.MATCHER_TYPE_GLOB, []string{"GET,POST"}, "GET", true},
		{"method glob comma: match POST", httptpl.MATCHER_TYPE_GLOB, []string{"GET,POST"}, "POST", true},
		{"method glob comma: no match", httptpl.MATCHER_TYPE_GLOB, []string{"GET,POST"}, "DELETE", false},
		{"method glob semicolon: match GET", httptpl.MATCHER_TYPE_GLOB, []string{"GET;POST"}, "GET", true},
		{"method glob semicolon: match POST", httptpl.MATCHER_TYPE_GLOB, []string{"GET;POST"}, "POST", true},
		{"method glob newline: match GET", httptpl.MATCHER_TYPE_GLOB, []string{"GET\nPOST"}, "GET", true},
		{"method glob newline: match POST", httptpl.MATCHER_TYPE_GLOB, []string{"GET\nPOST"}, "POST", true},
	}

	// MIME matcher
	mimeTests := []tc{
		{"mime comma: match image", httptpl.MATCHER_TYPE_MIME, []string{"image/*,video/*"}, "image/png", true},
		{"mime comma: match video", httptpl.MATCHER_TYPE_MIME, []string{"image/*,video/*"}, "video/mp4", true},
		{"mime comma: no match", httptpl.MATCHER_TYPE_MIME, []string{"image/*,video/*"}, "text/html", false},
		{"mime semicolon: match image", httptpl.MATCHER_TYPE_MIME, []string{"image/*;video/*"}, "image/png", true},
		{"mime semicolon: match video", httptpl.MATCHER_TYPE_MIME, []string{"image/*;video/*"}, "video/mp4", true},
		{"mime newline: match image", httptpl.MATCHER_TYPE_MIME, []string{"image/*\nvideo/*"}, "image/png", true},
		{"mime newline: match video", httptpl.MATCHER_TYPE_MIME, []string{"image/*\nvideo/*"}, "video/mp4", true},
	}

	allTests := [][]tc{suffixTests, uriTests, wordTests, methodTests, mimeTests}
	for _, tests := range allTests {
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				matcher := FilterDataToMatchers([]*ypb.FilterDataItem{{
					MatcherType: tt.matcherType,
					Group:       tt.group,
				}}, true) // expand=true for all filter types now
				require.NotNil(t, matcher, "matcher should not be nil")

				result, err := matcher.ExecuteRaw([]byte(tt.target), nil)
				require.NoError(t, err)
				require.Equal(t, tt.expectOK, result)
			})
		}
	}
}

// TestMITMFilter_IsPassed_AllFilterTypesDelimiter verifies the full
// MITMFilter.IsPassed path with multi-value delimiter input for each
// filter category, simulating the real data flow from frontend to backend.
func TestMITMFilter_IsPassed_AllFilterTypesDelimiter(t *testing.T) {
	delimiters := []struct {
		name string
		sep  string
	}{
		{"comma", ","},
		{"semicolon", ";"},
		{"newline", "\n"},
		{"CRLF", "\r\n"},
	}

	t.Run("IncludeSuffix with delimiter", func(t *testing.T) {
		for _, d := range delimiters {
			t.Run(d.name, func(t *testing.T) {
				filter := NewMITMFilter(&ypb.MITMFilterData{
					IncludeSuffix: []*ypb.FilterDataItem{{
						MatcherType: httptpl.MATCHER_TYPE_SUFFIX,
						Group:       []string{".js" + d.sep + ".css"},
					}},
				})
				// Should pass .js and .css, filter out .html
				require.True(t, filter.IsPassed("GET", "example.com:443", "/test.js", ".js"))
				require.True(t, filter.IsPassed("GET", "example.com:443", "/test.css", ".css"))
				require.False(t, filter.IsPassed("GET", "example.com:443", "/test.html", ".html"))
			})
		}
	})

	t.Run("IncludeUri with delimiter", func(t *testing.T) {
		for _, d := range delimiters {
			t.Run(d.name, func(t *testing.T) {
				filter := NewMITMFilter(&ypb.MITMFilterData{
					IncludeUri: []*ypb.FilterDataItem{{
						MatcherType: httptpl.MATCHER_TYPE_GLOB,
						Group:       []string{"/api/*" + d.sep + "/admin/*"},
					}},
				})
				require.True(t, filter.IsPassed("GET", "example.com:443", "/api/users", ""))
				require.True(t, filter.IsPassed("GET", "example.com:443", "/admin/x", ""))
				require.False(t, filter.IsPassed("GET", "example.com:443", "/other/x", ""))
			})
		}
	})

	t.Run("ExcludeMethods with delimiter", func(t *testing.T) {
		for _, d := range delimiters {
			t.Run(d.name, func(t *testing.T) {
				filter := NewMITMFilter(&ypb.MITMFilterData{
					ExcludeMethods: []*ypb.FilterDataItem{{
						MatcherType: httptpl.MATCHER_TYPE_GLOB,
						Group:       []string{"OPTIONS" + d.sep + "CONNECT"},
					}},
				})
				require.False(t, filter.IsPassed("OPTIONS", "example.com:443", "/test", ""))
				require.False(t, filter.IsPassed("CONNECT", "example.com:443", "/test", ""))
				require.True(t, filter.IsPassed("GET", "example.com:443", "/test", ""))
			})
		}
	})

	t.Run("IncludeHostnames with delimiter", func(t *testing.T) {
		for _, d := range delimiters {
			t.Run(d.name, func(t *testing.T) {
				filter := NewMITMFilter(&ypb.MITMFilterData{
					IncludeHostnames: []*ypb.FilterDataItem{{
						MatcherType: httptpl.MATCHER_TYPE_GLOB,
						Group:       []string{"*example.com*" + d.sep + "*test.com*"},
					}},
				})
				require.True(t, filter.IsPassed("GET", "example.com:443", "/test", ""))
				require.True(t, filter.IsPassed("GET", "test.com:443", "/test", ""))
				require.False(t, filter.IsPassed("GET", "other.com:443", "/test", ""))
			})
		}
	})
}
