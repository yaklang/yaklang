package yakgrpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestExtractHTTPResponse_RequestScopeExtractors(t *testing.T) {
	s := &Server{}
	reqRaw := "GET /api/test?token=secret HTTP/1.1\r\n" +
		"Host: example.com\r\n" +
		"Authorization: Bearer abc123\r\n" +
		"Content-Type: application/x-www-form-urlencoded\r\n\r\n" +
		"username=admin"
	rspRaw := "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK"
	params := &ypb.ExtractHTTPResponseParams{
		HTTPResponse: rspRaw,
		HTTPRequest:  reqRaw,
		IsHTTPS:      false,
		Extractors: []*ypb.HTTPResponseExtractor{
			{
				Name:             "auth_token",
				Type:             "regex",
				Scope:            "request_header",
				Groups:           []string{"Authorization: Bearer (\\w+)"},
				RegexpMatchGroup: []int64{1},
			},
			{
				Name:             "url_token",
				Type:             "regex",
				Scope:            "request_url",
				Groups:           []string{"token=(\\w+)"},
				RegexpMatchGroup: []int64{1},
			},
			{
				Name:             "username",
				Type:             "regex",
				Scope:            "request_body",
				Groups:           []string{"username=(\\w+)"},
				RegexpMatchGroup: []int64{1},
			},
		},
	}
	res, err := s.ExtractHTTPResponse(context.Background(), params)
	require.NoError(t, err)
	require.NotNil(t, res)
	resultMap := make(map[string]string)
	for _, kv := range res.GetValues() {
		resultMap[kv.GetKey()] = kv.GetValue()
	}
	require.Equal(t, "abc123", resultMap["auth_token"])
	require.Equal(t, "secret", resultMap["url_token"])
	require.Equal(t, "admin", resultMap["username"])
}
