package yakgrpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestExtractHTTPResponse_RequestScopeExtractors(t *testing.T) {
	s := &Server{}
	rspRaw := "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK"
	baseRequest := "POST /api/test?token=secret HTTP/1.1\r\n" +
		"Host: example.com\r\n" +
		"Authorization: Bearer abc123\r\n" +
		"Content-Type: application/x-www-form-urlencoded\r\n\r\n" +
		"key=value&username=admin"
	jsonRequest := "POST /api/json HTTP/1.1\r\n" +
		"Host: example.com\r\n" +
		"Content-Type: application/json\r\n\r\n" +
		"{\"info\": {\"username\": \"jsonuser\"}}"

	tests := []struct {
		name      string
		reqRaw    string
		extractor *ypb.HTTPResponseExtractor
		expected  string
	}{
		{
			name:   "regex_request_header",
			reqRaw: baseRequest,
			extractor: &ypb.HTTPResponseExtractor{
				Name:             "auth_token",
				Type:             "regex",
				Scope:            "request_header",
				Groups:           []string{"Authorization: Bearer (\\w+)"},
				RegexpMatchGroup: []int64{1},
			},
			expected: "abc123",
		},
		{
			name:   "regex_request_body",
			reqRaw: baseRequest,
			extractor: &ypb.HTTPResponseExtractor{
				Name:             "body_user",
				Type:             "regex",
				Scope:            "request_body",
				Groups:           []string{"username=(\\w+)"},
				RegexpMatchGroup: []int64{1},
			},
			expected: "admin",
		},
		{
			name:   "regex_request_url",
			reqRaw: baseRequest,
			extractor: &ypb.HTTPResponseExtractor{
				Name:             "url_token",
				Type:             "regex",
				Scope:            "request_url",
				Groups:           []string{"token=(\\w+)"},
				RegexpMatchGroup: []int64{1},
			},
			expected: "secret",
		},
		{
			name:   "regex_request_raw",
			reqRaw: baseRequest,
			extractor: &ypb.HTTPResponseExtractor{
				Name:             "path",
				Type:             "regex",
				Scope:            "request_raw",
				Groups:           []string{"POST (/api/\\w+)"},
				RegexpMatchGroup: []int64{1},
			},
			expected: "/api/test",
		},
		{
			name:   "kval_request_body",
			reqRaw: baseRequest,
			extractor: &ypb.HTTPResponseExtractor{
				Name:   "kval_key",
				Type:   "kval",
				Scope:  "request_body",
				Groups: []string{"key"},
			},
			expected: "value",
		},
		{
			name:   "json_request_body",
			reqRaw: jsonRequest,
			extractor: &ypb.HTTPResponseExtractor{
				Name:   "json_username",
				Type:   "json",
				Scope:  "request_body",
				Groups: []string{".info.username"},
			},
			expected: "jsonuser",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := &ypb.ExtractHTTPResponseParams{
				HTTPResponse: rspRaw,
				HTTPRequest:  tt.reqRaw,
				IsHTTPS:      false,
				Extractors:   []*ypb.HTTPResponseExtractor{tt.extractor},
			}
			res, err := s.ExtractHTTPResponse(context.Background(), params)
			require.NoError(t, err)
			require.NotNil(t, res)
			require.Len(t, res.GetValues(), 1)
			require.Equal(t, tt.expected, res.GetValues()[0].GetValue())
		})
	}
}
