package yakgrpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
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

// TestExtractHTTPResponse_ResponseScopeExtractors 测试响应相关的提取器
func TestExtractHTTPResponse_ResponseScopeExtractors(t *testing.T) {
	s := &Server{}
	reqRaw := "GET /api/test HTTP/1.1\r\nHost: example.com\r\n\r\n"
	jsonResponse := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: application/json\r\n" +
		"X-Custom-Header: custom-value\r\n" +
		"Set-Cookie: session=abc123\r\n\r\n" +
		"{\"status\": \"success\", \"user\": {\"id\": 123, \"name\": \"John\"}}"

	tests := []struct {
		name      string
		rspRaw    string
		extractor *ypb.HTTPResponseExtractor
		expected  string
	}{
		{
			name:   "regex_response_header",
			rspRaw: jsonResponse,
			extractor: &ypb.HTTPResponseExtractor{
				Name:             "session_cookie",
				Type:             "regex",
				Scope:            "header",
				Groups:           []string{"session=(\\w+)"},
				RegexpMatchGroup: []int64{1},
			},
			expected: "abc123",
		},
		{
			name:   "regex_response_body",
			rspRaw: jsonResponse,
			extractor: &ypb.HTTPResponseExtractor{
				Name:             "status",
				Type:             "regex",
				Scope:            "body",
				Groups:           []string{"\"status\":\\s*\"(\\w+)\""},
				RegexpMatchGroup: []int64{1},
			},
			expected: "success",
		},
		{
			name:   "json_response_body",
			rspRaw: jsonResponse,
			extractor: &ypb.HTTPResponseExtractor{
				Name:   "user_name",
				Type:   "json",
				Scope:  "body",
				Groups: []string{".user.name"},
			},
			expected: "John",
		},
		{
			name:   "json_nested_value",
			rspRaw: jsonResponse,
			extractor: &ypb.HTTPResponseExtractor{
				Name:   "user_id",
				Type:   "json",
				Scope:  "body",
				Groups: []string{".user.id"},
			},
			expected: "123",
		},
		{
			name:   "kval_response_header",
			rspRaw: jsonResponse,
			extractor: &ypb.HTTPResponseExtractor{
				Name:   "custom_header",
				Type:   "kval",
				Scope:  "header",
				Groups: []string{"x_custom_header"},
			},
			expected: "custom-value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := &ypb.ExtractHTTPResponseParams{
				HTTPResponse: tt.rspRaw,
				HTTPRequest:  reqRaw,
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

// TestExtractHTTPResponse_NucleiDSL 测试 nuclei-dsl 提取器
func TestExtractHTTPResponse_NucleiDSL(t *testing.T) {
	s := &Server{}
	reqRaw := "POST /api/login HTTP/1.1\r\n" +
		"Host: api.example.com\r\n" +
		"Content-Type: application/json\r\n\r\n" +
		"{\"username\": \"admin\"}"
	rspRaw := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: application/json\r\n\r\n" +
		"{\"token\": \"xyz789\"}"

	tests := []struct {
		name          string
		dslExpr       string
		checkContains bool
		expected      string
	}{
		{
			name:          "extract_request_url",
			dslExpr:       "request_url",
			checkContains: true,
			expected:      "/api/login",
		},
		{
			name:          "extract_request_body",
			dslExpr:       "request_body",
			checkContains: true,
			expected:      "admin",
		},
		{
			name:          "extract_response_body",
			dslExpr:       "body",
			checkContains: true,
			expected:      "xyz789",
		},
		{
			name:          "concat_request_and_response",
			dslExpr:       "concat(request_url, \" -> \", body)",
			checkContains: true,
			expected:      "/api/login",
		},
		{
			name:     "check_is_https",
			dslExpr:  "to_string(is_https)",
			expected: "false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := &ypb.ExtractHTTPResponseParams{
				HTTPResponse: rspRaw,
				HTTPRequest:  reqRaw,
				IsHTTPS:      false,
				Extractors: []*ypb.HTTPResponseExtractor{
					{
						Name:   "test_var",
						Type:   "nuclei-dsl",
						Groups: []string{tt.dslExpr},
					},
				},
			}
			res, err := s.ExtractHTTPResponse(context.Background(), params)
			require.NoError(t, err)
			require.NotNil(t, res)
			require.Len(t, res.GetValues(), 1)

			actual := res.GetValues()[0].GetValue()
			if tt.checkContains {
				assert.Contains(t, actual, tt.expected, "提取的值应该包含预期内容")
			} else {
				assert.Equal(t, tt.expected, actual, "提取的值应该完全匹配")
			}
		})
	}
}

// TestExtractHTTPResponse_MultipleExtractors 测试多个提取器同时工作
func TestExtractHTTPResponse_MultipleExtractors(t *testing.T) {
	s := &Server{}
	reqRaw := "POST /api/users?id=456 HTTP/1.1\r\n" +
		"Host: api.example.com\r\n" +
		"Authorization: Bearer token123\r\n\r\n" +
		"username=testuser"
	rspRaw := "HTTP/1.1 201 Created\r\n" +
		"Content-Type: application/json\r\n" +
		"Location: /api/users/456\r\n\r\n" +
		"{\"id\": 456, \"username\": \"testuser\"}"

	params := &ypb.ExtractHTTPResponseParams{
		HTTPResponse: rspRaw,
		HTTPRequest:  reqRaw,
		IsHTTPS:      false,
		Extractors: []*ypb.HTTPResponseExtractor{
			{
				Name:             "request_token",
				Type:             "regex",
				Scope:            "request_header",
				Groups:           []string{"Bearer (\\w+)"},
				RegexpMatchGroup: []int64{1},
			},
			{
				Name:             "request_user",
				Type:             "regex",
				Scope:            "request_body",
				Groups:           []string{"username=(\\w+)"},
				RegexpMatchGroup: []int64{1},
			},
			{
				Name:   "response_id",
				Type:   "json",
				Scope:  "body",
				Groups: []string{".id"},
			},
			{
				Name:             "response_location",
				Type:             "regex",
				Scope:            "header",
				Groups:           []string{"Location: (/api/\\S+)"},
				RegexpMatchGroup: []int64{1},
			},
		},
	}

	res, err := s.ExtractHTTPResponse(context.Background(), params)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.GetValues(), 4, "应该提取到4个值")

	// 验证每个提取结果
	resultMap := make(map[string]string)
	for _, v := range res.GetValues() {
		resultMap[v.GetKey()] = v.GetValue()
	}

	assert.Equal(t, "token123", resultMap["request_token"])
	assert.Equal(t, "testuser", resultMap["request_user"])
	assert.Equal(t, "456", resultMap["response_id"])
	assert.Equal(t, "/api/users/456", resultMap["response_location"])
}

// TestExtractHTTPResponse_EdgeCases 测试边界情况
func TestExtractHTTPResponse_EdgeCases(t *testing.T) {
	s := &Server{}

	t.Run("empty_response", func(t *testing.T) {
		params := &ypb.ExtractHTTPResponseParams{
			HTTPResponse: "",
			HTTPRequest:  "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n",
			IsHTTPS:      false,
			Extractors: []*ypb.HTTPResponseExtractor{
				{
					Name:   "test",
					Type:   "regex",
					Scope:  "body",
					Groups: []string{".*"},
				},
			},
		}
		_, err := s.ExtractHTTPResponse(context.Background(), params)
		require.Error(t, err, "空响应应该返回错误")
	})

	t.Run("no_extractors", func(t *testing.T) {
		params := &ypb.ExtractHTTPResponseParams{
			HTTPResponse: "HTTP/1.1 200 OK\r\n\r\nOK",
			HTTPRequest:  "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n",
			IsHTTPS:      false,
			Extractors:   []*ypb.HTTPResponseExtractor{},
		}
		_, err := s.ExtractHTTPResponse(context.Background(), params)
		require.Error(t, err, "没有提取器应该返回错误")
	})

	t.Run("no_match", func(t *testing.T) {
		params := &ypb.ExtractHTTPResponseParams{
			HTTPResponse: "HTTP/1.1 200 OK\r\n\r\nOK",
			HTTPRequest:  "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n",
			IsHTTPS:      false,
			Extractors: []*ypb.HTTPResponseExtractor{
				{
					Name:             "not_found",
					Type:             "regex",
					Scope:            "body",
					Groups:           []string{"NOTEXIST(\\w+)"},
					RegexpMatchGroup: []int64{1},
				},
			},
		}
		res, err := s.ExtractHTTPResponse(context.Background(), params)
		require.NoError(t, err)
		require.NotNil(t, res)
		// 未匹配时应该返回空值或 nil
		require.Len(t, res.GetValues(), 1)
		// 值应该为空
		assert.Empty(t, res.GetValues()[0].GetValue())
	})

	t.Run("extract_without_request", func(t *testing.T) {
		params := &ypb.ExtractHTTPResponseParams{
			HTTPResponse: "HTTP/1.1 200 OK\r\nX-Token: abc123\r\n\r\n{\"status\":\"ok\"}",
			HTTPRequest:  "", // 没有请求
			IsHTTPS:      false,
			Extractors: []*ypb.HTTPResponseExtractor{
				{
					Name:             "token",
					Type:             "regex",
					Scope:            "header",
					Groups:           []string{"X-Token: (\\w+)"},
					RegexpMatchGroup: []int64{1},
				},
			},
		}
		res, err := s.ExtractHTTPResponse(context.Background(), params)
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Len(t, res.GetValues(), 1)
		assert.Equal(t, "abc123", res.GetValues()[0].GetValue())
	})
}

// TestExtractHTTPResponse_XPath 测试 XPath 提取器
func TestExtractHTTPResponse_XPath(t *testing.T) {
	s := &Server{}
	reqRaw := "GET /api/data HTTP/1.1\r\nHost: example.com\r\n\r\n"
	xmlResponse := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: application/xml\r\n\r\n" +
		"<?xml version=\"1.0\"?>\n" +
		"<root>\n" +
		"  <user id=\"123\">\n" +
		"    <name>Alice</name>\n" +
		"    <email>alice@example.com</email>\n" +
		"  </user>\n" +
		"</root>"

	htmlResponse := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: text/html\r\n\r\n" +
		"<html><body><div class=\"content\"><h1>Title</h1><p>Paragraph</p></div></body></html>"

	tests := []struct {
		name      string
		rspRaw    string
		extractor *ypb.HTTPResponseExtractor
		expected  string
	}{
		{
			name:   "xpath_xml_text",
			rspRaw: xmlResponse,
			extractor: &ypb.HTTPResponseExtractor{
				Name:   "user_name",
				Type:   "xpath",
				Scope:  "body",
				Groups: []string{"//user/name"},
			},
			expected: "Alice",
		},
		{
			name:   "xpath_xml_email",
			rspRaw: xmlResponse,
			extractor: &ypb.HTTPResponseExtractor{
				Name:   "user_email",
				Type:   "xpath",
				Scope:  "body",
				Groups: []string{"//user/email"},
			},
			expected: "alice@example.com",
		},
		{
			name:   "xpath_html",
			rspRaw: htmlResponse,
			extractor: &ypb.HTTPResponseExtractor{
				Name:   "title",
				Type:   "xpath",
				Scope:  "body",
				Groups: []string{"//h1"},
			},
			expected: "Title",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := &ypb.ExtractHTTPResponseParams{
				HTTPResponse: tt.rspRaw,
				HTTPRequest:  reqRaw,
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

// TestExtractHTTPResponse_ComplexScenario 测试复杂场景
func TestExtractHTTPResponse_ComplexScenario(t *testing.T) {
	s := &Server{}

	// 模拟一个完整的登录场景
	loginRequest := "POST /api/v1/auth/login HTTP/1.1\r\n" +
		"Host: api.example.com\r\n" +
		"Content-Type: application/json\r\n" +
		"User-Agent: TestClient/1.0\r\n" +
		"X-Request-ID: req-12345\r\n\r\n" +
		"{\"username\": \"admin\", \"password\": \"secret123\", \"remember\": true}"

	loginResponse := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: application/json\r\n" +
		"Set-Cookie: session=sess-abc123; Path=/; HttpOnly\r\n" +
		"X-Response-ID: resp-67890\r\n" +
		"X-Rate-Limit: 100\r\n\r\n" +
		"{\"success\": true, \"token\": \"eyJhbGc...\", \"user\": {\"id\": 42, \"role\": \"admin\"}, \"expires_in\": 3600}"

	params := &ypb.ExtractHTTPResponseParams{
		HTTPResponse: loginResponse,
		HTTPRequest:  loginRequest,
		IsHTTPS:      true,
		Extractors: []*ypb.HTTPResponseExtractor{
			// 从请求中提取
			{
				Name:   "req_username",
				Type:   "json",
				Scope:  "request_body",
				Groups: []string{".username"},
			},
			{
				Name:             "req_request_id",
				Type:             "regex",
				Scope:            "request_header",
				Groups:           []string{"X-Request-ID: (\\S+)"},
				RegexpMatchGroup: []int64{1},
			},
			// 从响应中提取
			{
				Name:   "resp_token",
				Type:   "json",
				Scope:  "body",
				Groups: []string{".token"},
			},
			{
				Name:   "resp_user_id",
				Type:   "json",
				Scope:  "body",
				Groups: []string{".user.id"},
			},
			{
				Name:             "resp_session",
				Type:             "regex",
				Scope:            "header",
				Groups:           []string{"session=(\\S+?);"},
				RegexpMatchGroup: []int64{1},
			},
			// 使用 nuclei-dsl 组合
			{
				Name:   "combined_info",
				Type:   "nuclei-dsl",
				Groups: []string{"concat(\"User: \", request_body, \" got token from \", request_url)"},
			},
		},
	}

	res, err := s.ExtractHTTPResponse(context.Background(), params)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.GetValues(), 6)

	resultMap := make(map[string]string)
	for _, v := range res.GetValues() {
		resultMap[v.GetKey()] = v.GetValue()
	}

	// 验证所有提取结果
	assert.Equal(t, "admin", resultMap["req_username"])
	assert.Equal(t, "req-12345", resultMap["req_request_id"])
	assert.Contains(t, resultMap["resp_token"], "eyJhbGc")
	assert.Equal(t, "42", resultMap["resp_user_id"])
	assert.Equal(t, "sess-abc123", resultMap["resp_session"])
	assert.Contains(t, resultMap["combined_info"], "admin")
	assert.Contains(t, resultMap["combined_info"], "/api/v1/auth/login")
}
