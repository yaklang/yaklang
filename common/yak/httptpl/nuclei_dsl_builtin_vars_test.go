package httptpl

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// TestNucleiDSL_BuiltinVars_Basic tests basic built-in variables in matcher
func TestNucleiDSL_BuiltinVars_Basic(t *testing.T) {
	rsp := []byte(`HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Content-Length: 50
X-Custom-Header: custom-value
Server: TestServer/1.0

{"message": "Hello World", "status": "success"}`)

	tests := []struct {
		name       string
		dslExpr    string
		expectTrue bool
	}{
		{
			name:       "test status_code variable",
			dslExpr:    "status_code == 200",
			expectTrue: true,
		},
		{
			name:       "test content_length variable",
			dslExpr:    "content_length > 0",
			expectTrue: true,
		},
		{
			name:       "test body variable contains",
			dslExpr:    `contains(body, "Hello World")`,
			expectTrue: true,
		},
		{
			name:       "test raw variable contains",
			dslExpr:    `contains(raw, "HTTP/1.1")`,
			expectTrue: true,
		},
		{
			name:       "test all_headers variable",
			dslExpr:    `contains(all_headers, "Content-Type")`,
			expectTrue: true,
		},
		{
			name:       "test header variable (content_type)",
			dslExpr:    `contains(content_type, "application/json")`,
			expectTrue: true,
		},
		{
			name:       "test header variable (x_custom_header)",
			dslExpr:    `x_custom_header == "custom-value"`,
			expectTrue: true,
		},
		{
			name:       "test header variable (server)",
			dslExpr:    `contains(server, "TestServer")`,
			expectTrue: true,
		},
		{
			name:       "test duration variable exists",
			dslExpr:    `duration >= 0`,
			expectTrue: true,
		},
		{
			name:       "test combined expression",
			dslExpr:    `status_code == 200 && contains(body, "success")`,
			expectTrue: true,
		},
		{
			name:       "test negative case",
			dslExpr:    `status_code == 404`,
			expectTrue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := &YakMatcher{
				MatcherType: "expr",
				ExprType:    "nuclei-dsl",
				Group:       []string{tt.dslExpr},
			}

			result, err := matcher.ExecuteRawResponse(rsp, nil)
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}

			if result != tt.expectTrue {
				t.Errorf("Expected %v, got %v for expression: %s", tt.expectTrue, result, tt.dslExpr)
			}
		})
	}
}

// TestNucleiDSL_BuiltinVars_MultipleRequests tests variables with suffixes for multiple requests
func TestNucleiDSL_BuiltinVars_MultipleRequests(t *testing.T) {
	rsp1 := []byte(`HTTP/1.1 200 OK
Content-Type: text/html

<html>Response 1</html>`)

	rsp2 := []byte(`HTTP/1.1 404 Not Found
Content-Type: text/plain

Not Found`)

	// Test with suffix _1
	vars1 := LoadVarFromRawResponse(rsp1, 0.5, "_1")
	if vars1["status_code_1"] != 200 {
		t.Errorf("Expected status_code_1 to be 200, got %v", vars1["status_code_1"])
	}
	if !strings.Contains(vars1["body_1"].(string), "Response 1") {
		t.Errorf("Expected body_1 to contain 'Response 1'")
	}
	if vars1["duration_1"].(float64) != 0.5 {
		t.Errorf("Expected duration_1 to be 0.5, got %v", vars1["duration_1"])
	}

	// Test with suffix _2
	vars2 := LoadVarFromRawResponse(rsp2, 1.2, "_2")
	if vars2["status_code_2"] != 404 {
		t.Errorf("Expected status_code_2 to be 404, got %v", vars2["status_code_2"])
	}
	if !strings.Contains(vars2["body_2"].(string), "Not Found") {
		t.Errorf("Expected body_2 to contain 'Not Found'")
	}

	// Test that _1 suffix also sets non-suffixed variables
	if vars1["status_code"] != 200 {
		t.Errorf("Expected status_code to be 200 when suffix is _1, got %v", vars1["status_code"])
	}
}

// TestNucleiDSL_BuiltinVars_HeaderNormalization tests header name normalization
func TestNucleiDSL_BuiltinVars_HeaderNormalization(t *testing.T) {
	rsp := []byte(`HTTP/1.1 200 OK
Content-Type: text/html
X-Frame-Options: DENY
X-XSS-Protection: 1; mode=block
Cache-Control: no-cache

test`)

	vars := LoadVarFromRawResponse(rsp, 0)

	tests := []struct {
		varName       string
		expectedValue string
	}{
		{"content_type", "text/html"},
		{"x_frame_options", "DENY"},
		{"x_xss_protection", "1; mode=block"},
		{"cache_control", "no-cache"},
	}

	for _, tt := range tests {
		t.Run(tt.varName, func(t *testing.T) {
			value, ok := vars[tt.varName]
			if !ok {
				t.Errorf("Variable %s not found in vars", tt.varName)
				return
			}
			if value != tt.expectedValue {
				t.Errorf("Expected %s to be %q, got %q", tt.varName, tt.expectedValue, value)
			}
		})
	}
}

// TestNucleiDSL_BuiltinFunctions tests built-in DSL functions
func TestNucleiDSL_BuiltinFunctions(t *testing.T) {
	rsp := []byte(`HTTP/1.1 200 OK
Content-Type: text/html

<html><body>Test Content</body></html>`)

	tests := []struct {
		name       string
		dslExpr    string
		expectTrue bool
	}{
		{
			name:       "test to_upper function",
			dslExpr:    `to_upper(body) == to_upper("<html><body>Test Content</body></html>")`,
			expectTrue: true,
		},
		{
			name:       "test to_lower function",
			dslExpr:    `contains(to_lower(body), "test content")`,
			expectTrue: true,
		},
		{
			name:       "test len function",
			dslExpr:    `len(body) > 0`,
			expectTrue: true,
		},
		{
			name:       "test contains function",
			dslExpr:    `contains(body, "Test", "Content")`,
			expectTrue: true,
		},
		{
			name:       "test regex function",
			dslExpr:    `regex("<body>.*</body>", body)`,
			expectTrue: true,
		},
		{
			name:       "test base64 encode",
			dslExpr:    `len(base64("test")) > 0`,
			expectTrue: true,
		},
		{
			name:       "test md5 function",
			dslExpr:    `len(md5(body)) == 32`,
			expectTrue: true,
		},
		{
			name:       "test concat function",
			dslExpr:    `concat("status:", status_code) == "status:200"`,
			expectTrue: true,
		},
		{
			name:       "test replace function",
			dslExpr:    `contains(replace(body, "Test", "Demo"), "Demo")`,
			expectTrue: true,
		},
		{
			name:       "test starts_with function",
			dslExpr:    `starts_with(body, "<html>")`,
			expectTrue: true,
		},
		{
			name:       "test ends_with function",
			dslExpr:    `ends_with(body, "</html>")`,
			expectTrue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := &YakMatcher{
				MatcherType: "expr",
				ExprType:    "nuclei-dsl",
				Group:       []string{tt.dslExpr},
			}

			result, err := matcher.ExecuteRawResponse(rsp, nil)
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}

			if result != tt.expectTrue {
				t.Errorf("Expected %v, got %v for expression: %s", tt.expectTrue, result, tt.dslExpr)
			}
		})
	}
}

// TestNucleiDSL_Extractor_BuiltinVars tests extractor with built-in variables
func TestNucleiDSL_Extractor_BuiltinVars(t *testing.T) {
	rsp := []byte(`HTTP/1.1 200 OK
Content-Type: application/json
X-Request-ID: abc123

{"token": "secret-token-123", "user": "admin"}`)

	tests := []struct {
		name          string
		dslExpr       string
		expectedValue string
	}{
		{
			name:          "extract status_code",
			dslExpr:       "status_code",
			expectedValue: "200",
		},
		{
			name:          "extract header variable",
			dslExpr:       "x_request_id",
			expectedValue: "abc123",
		},
		{
			name:          "extract with concat",
			dslExpr:       `concat("Status: ", status_code)`,
			expectedValue: "Status: 200",
		},
		{
			name:          "extract with regex on body",
			dslExpr:       `body`,
			expectedValue: `{"token": "secret-token-123", "user": "admin"}`,
		},
		{
			name:          "extract content_type",
			dslExpr:       "content_type",
			expectedValue: "application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := &YakExtractor{
				Name:   "test_var",
				Type:   "nuclei-dsl",
				Groups: []string{tt.dslExpr},
			}

			result, err := extractor.Execute(rsp)
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}

			value, ok := result["test_var"]
			if !ok {
				t.Fatalf("Expected variable 'test_var' not found in result")
			}

			if value != tt.expectedValue {
				t.Errorf("Expected %q, got %q", tt.expectedValue, value)
			}
		})
	}
}

// TestNucleiDSL_Extractor_WithPreviousResults tests extractor using previous extraction results
func TestNucleiDSL_Extractor_WithPreviousResults(t *testing.T) {
	rsp := []byte(`HTTP/1.1 200 OK
Content-Type: text/html

<html>Test Page</html>`)

	// First extraction
	extractor1 := &YakExtractor{
		Name:   "page_type",
		Type:   "nuclei-dsl",
		Groups: []string{`"html"`},
	}

	result1, err := extractor1.Execute(rsp)
	if err != nil {
		t.Fatalf("First extraction failed: %v", err)
	}

	// Second extraction using first result
	extractor2 := &YakExtractor{
		Name:   "combined",
		Type:   "nuclei-dsl",
		Groups: []string{`concat("Type: ", page_type)`},
	}

	result2, err := extractor2.Execute(rsp, result1)
	if err != nil {
		t.Fatalf("Second extraction failed: %v", err)
	}

	combined, ok := result2["combined"]
	if !ok {
		t.Fatalf("Expected variable 'combined' not found")
	}

	if combined != "Type: html" {
		t.Errorf("Expected 'Type: html', got %q", combined)
	}
}

// TestNucleiDSL_ComplexExpressions tests complex DSL expressions
func TestNucleiDSL_ComplexExpressions(t *testing.T) {
	rsp := []byte(`HTTP/1.1 200 OK
Content-Type: application/json
Set-Cookie: session=abc123; Path=/; HttpOnly

{"status": "success", "data": {"id": 12345, "name": "test"}}`)

	tests := []struct {
		name       string
		dslExpr    string
		expectTrue bool
	}{
		{
			name:       "complex boolean expression",
			dslExpr:    `status_code == 200 && contains(content_type, "json") && contains(body, "success")`,
			expectTrue: true,
		},
		{
			name:       "nested function calls",
			dslExpr:    `contains(to_lower(body), to_lower("SUCCESS"))`,
			expectTrue: true,
		},
		{
			name:       "multiple conditions with OR",
			dslExpr:    `status_code == 404 || contains(body, "success")`,
			expectTrue: true,
		},
		{
			name:       "check cookie existence",
			dslExpr:    `contains(all_headers, "Set-Cookie") && contains(all_headers, "session=")`,
			expectTrue: true,
		},
		{
			name:       "length comparison",
			dslExpr:    `len(body) > 50 && len(body) < 200`,
			expectTrue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := &YakMatcher{
				MatcherType: "expr",
				ExprType:    "nuclei-dsl",
				Group:       []string{tt.dslExpr},
			}

			result, err := matcher.ExecuteRawResponse(rsp, nil)
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}

			if result != tt.expectTrue {
				t.Errorf("Expected %v, got %v for expression: %s", tt.expectTrue, result, tt.dslExpr)
			}
		})
	}
}

// TestNucleiDSL_EncodingFunctions tests encoding/decoding functions
func TestNucleiDSL_EncodingFunctions(t *testing.T) {
	rsp := []byte(`HTTP/1.1 200 OK

test`)

	tests := []struct {
		name       string
		dslExpr    string
		expectTrue bool
	}{
		{
			name:       "base64 encode and decode",
			dslExpr:    `base64_decode(base64("hello")) == "hello"`,
			expectTrue: true,
		},
		{
			name:       "url encode and decode",
			dslExpr:    `url_decode(url_encode("hello world")) == "hello world"`,
			expectTrue: true,
		},
		{
			name:       "hex encode and decode",
			dslExpr:    `hex_decode(hex_encode("test")) == "test"`,
			expectTrue: true,
		},
		{
			name:       "md5 hash length",
			dslExpr:    `len(md5("test")) == 32`,
			expectTrue: true,
		},
		{
			name:       "sha256 hash length",
			dslExpr:    `len(sha256("test")) == 64`,
			expectTrue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := &YakMatcher{
				MatcherType: "expr",
				ExprType:    "nuclei-dsl",
				Group:       []string{tt.dslExpr},
			}

			result, err := matcher.ExecuteRawResponse(rsp, nil)
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}

			if result != tt.expectTrue {
				t.Errorf("Expected %v, got %v for expression: %s", tt.expectTrue, result, tt.dslExpr)
			}
		})
	}
}

// TestNucleiDSL_WithExternalVars tests using external variables in DSL
func TestNucleiDSL_WithExternalVars(t *testing.T) {
	rsp := []byte(`HTTP/1.1 200 OK

test`)

	externalVars := map[string]interface{}{
		"custom_var":   "custom_value",
		"numeric_var":  42,
		"expected_msg": "success",
	}

	tests := []struct {
		name       string
		dslExpr    string
		expectTrue bool
	}{
		{
			name:       "use external string variable",
			dslExpr:    `custom_var == "custom_value"`,
			expectTrue: true,
		},
		{
			name:       "use external numeric variable",
			dslExpr:    `numeric_var == 42`,
			expectTrue: true,
		},
		{
			name:       "combine external and built-in variables",
			dslExpr:    `status_code == 200 && custom_var == "custom_value"`,
			expectTrue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := &YakMatcher{
				MatcherType: "expr",
				ExprType:    "nuclei-dsl",
				Group:       []string{tt.dslExpr},
			}

			result, err := matcher.ExecuteRawResponse(rsp, externalVars)
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}

			if result != tt.expectTrue {
				t.Errorf("Expected %v, got %v for expression: %s", tt.expectTrue, result, tt.dslExpr)
			}
		})
	}
}

// TestNucleiDSL_RequestVariables tests request_* variables in matcher
func TestNucleiDSL_RequestVariables(t *testing.T) {
	req := []byte(`POST /api/login HTTP/1.1
Host: example.com
Content-Type: application/json
Authorization: Bearer test-token

{"username": "admin", "password": "secret"}`)

	rsp := []byte(`HTTP/1.1 200 OK
Content-Type: application/json

{"status": "success", "token": "new-token"}`)

	tests := []struct {
		name       string
		dslExpr    string
		expectTrue bool
	}{
		{
			name:       "test request_url variable",
			dslExpr:    `contains(request_url, "/api/login")`,
			expectTrue: true,
		},
		{
			name:       "test request_body variable",
			dslExpr:    `contains(request_body, "username")`,
			expectTrue: true,
		},
		{
			name:       "test request_headers variable",
			dslExpr:    `contains(request_headers, "Authorization")`,
			expectTrue: true,
		},
		{
			name:       "test request_raw variable",
			dslExpr:    `contains(request_raw, "POST")`,
			expectTrue: true,
		},
		{
			name:       "test combined request and response",
			dslExpr:    `contains(request_body, "admin") && contains(body, "success")`,
			expectTrue: true,
		},
		{
			name:       "test request body JSON content",
			dslExpr:    `contains(request_body, "password")`,
			expectTrue: true,
		},
		{
			name:       "test request headers Authorization",
			dslExpr:    `contains(request_headers, "Bearer")`,
			expectTrue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := &YakMatcher{
				MatcherType: "expr",
				ExprType:    "nuclei-dsl",
				Group:       []string{tt.dslExpr},
			}

			result, err := matcher.Execute(&RespForMatch{
				RawPacket:     rsp,
				RequestPacket: req,
			}, nil)
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}

			if result != tt.expectTrue {
				t.Errorf("Expected %v, got %v for expression: %s", tt.expectTrue, result, tt.dslExpr)
			}
		})
	}
}

// TestNucleiDSL_RequestVariables_WithoutRequest tests that request variables are empty when no request provided
func TestNucleiDSL_RequestVariables_WithoutRequest(t *testing.T) {
	rsp := []byte(`HTTP/1.1 200 OK

test`)

	// Test without request packet (should not fail, just have empty request variables)
	matcher := &YakMatcher{
		MatcherType: "expr",
		ExprType:    "nuclei-dsl",
		Group:       []string{`status_code == 200`}, // Only use response variables
	}

	result, err := matcher.Execute(&RespForMatch{
		RawPacket: rsp,
	}, nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !result {
		t.Errorf("Expected true, got false")
	}
}

// TestNucleiDSL_Extractor_RequestVariables tests request variables in extractor
func TestNucleiDSL_Extractor_RequestVariables(t *testing.T) {
	req := []byte(`GET /api/users?id=123 HTTP/1.1
Host: api.example.com
User-Agent: TestClient/1.0

`)

	rsp := []byte(`HTTP/1.1 200 OK
Content-Type: application/json

{"user_id": 123, "name": "John"}`)

	tests := []struct {
		name          string
		dslExpr       string
		checkContains bool
		expectedValue string
	}{
		{
			name:          "extract request_url",
			dslExpr:       "request_url",
			expectedValue: "http://api.example.com/api/users?id=123",
		},
		{
			name:          "extract from request_headers",
			dslExpr:       `request_headers`,
			checkContains: true,
			expectedValue: "GET /api/users?id=123 HTTP/1.1",
		},
		{
			name:          "combine request and response data",
			dslExpr:       `concat(request_url, " -> ", to_string(body))`,
			expectedValue: `http://api.example.com/api/users?id=123 -> {"user_id": 123, "name": "John"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := &YakExtractor{
				Name:   "test_var",
				Type:   "nuclei-dsl",
				Groups: []string{tt.dslExpr},
			}

			result, err := extractor.ExecuteWithRequest(rsp, req, false)
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}

			value, ok := result["test_var"]
			if !ok {
				t.Fatalf("Expected variable 'test_var' not found in result")
			}

			if tt.checkContains {
				if !strings.Contains(value.(string), tt.expectedValue) {
					t.Errorf("Expected to contain %q, got %q", tt.expectedValue, value)
				}
			} else {
				if value != tt.expectedValue {
					t.Errorf("Expected %q, got %q", tt.expectedValue, value)
				}
			}
		})
	}
}

// TestNucleiDSL_RequestVariables_POST tests POST request with body
func TestNucleiDSL_RequestVariables_POST(t *testing.T) {
	req := []byte(`POST /api/data HTTP/1.1
Host: example.com
Content-Type: application/x-www-form-urlencoded
Content-Length: 27

username=test&password=123`)

	rsp := []byte(`HTTP/1.1 201 Created

{"id": 456}`)

	tests := []struct {
		name       string
		dslExpr    string
		expectTrue bool
	}{
		{
			name:       "check POST method in request_raw",
			dslExpr:    `contains(request_raw, "POST")`,
			expectTrue: true,
		},
		{
			name:       "check request body content",
			dslExpr:    `contains(request_body, "username=test")`,
			expectTrue: true,
		},
		{
			name:       "check Content-Type in request headers",
			dslExpr:    `contains(request_headers, "application/x-www-form-urlencoded")`,
			expectTrue: true,
		},
		{
			name:       "verify response status with request body",
			dslExpr:    `status_code == 201 && contains(request_body, "password")`,
			expectTrue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := &YakMatcher{
				MatcherType: "expr",
				ExprType:    "nuclei-dsl",
				Group:       []string{tt.dslExpr},
			}

			result, err := matcher.Execute(&RespForMatch{
				RawPacket:     rsp,
				RequestPacket: req,
			}, nil)
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}

			if result != tt.expectTrue {
				t.Errorf("Expected %v, got %v for expression: %s", tt.expectTrue, result, tt.dslExpr)
			}
		})
	}
}

// TestMatcher_RequestScope tests matcher with request scope
func TestMatcher_RequestScope(t *testing.T) {
	req := []byte(`POST /api/users HTTP/1.1
Host: example.com
Content-Type: application/json
Authorization: Bearer token123

{"name": "John", "age": 30}`)

	rsp := []byte(`HTTP/1.1 201 Created
Content-Type: application/json

{"id": 456, "status": "created"}`)

	tests := []struct {
		name       string
		scope      string
		matchType  string
		group      []string
		expectTrue bool
	}{
		{
			name:       "match request_header scope with word",
			scope:      "request_header",
			matchType:  "word",
			group:      []string{"Authorization"},
			expectTrue: true,
		},
		{
			name:       "match request_body scope with word",
			scope:      "request_body",
			matchType:  "word",
			group:      []string{"John"},
			expectTrue: true,
		},
		{
			name:       "match request_raw scope with regexp",
			scope:      "request_raw",
			matchType:  "regexp",
			group:      []string{`POST /api/\w+`},
			expectTrue: true,
		},
		{
			name:       "match request_url scope with word",
			scope:      "request_url",
			matchType:  "word",
			group:      []string{"/api/users"},
			expectTrue: true,
		},
		{
			name:       "match request_header with regexp",
			scope:      "request_header",
			matchType:  "regexp",
			group:      []string{`Bearer \w+`},
			expectTrue: true,
		},
		{
			name:       "match request_body with JSON content",
			scope:      "request_body",
			matchType:  "word",
			group:      []string{`"age": 30`},
			expectTrue: true,
		},
		{
			name:       "negative test - request_body not match",
			scope:      "request_body",
			matchType:  "word",
			group:      []string{"NotExist"},
			expectTrue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := &YakMatcher{
				MatcherType: tt.matchType,
				Scope:       tt.scope,
				Group:       tt.group,
			}

			result, err := matcher.Execute(&RespForMatch{
				RawPacket:     rsp,
				RequestPacket: req,
			}, nil)
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}

			if result != tt.expectTrue {
				t.Errorf("Expected %v, got %v", tt.expectTrue, result)
			}
		})
	}
}

// TestExtractor_RequestScope tests extractor with request scope
func TestExtractor_RequestScope(t *testing.T) {
	req := []byte(`GET /api/products?category=electronics&limit=10 HTTP/1.1
Host: shop.example.com
User-Agent: Mozilla/5.0
Cookie: session=abc123

`)

	rsp := []byte(`HTTP/1.1 200 OK
Content-Type: application/json

{"products": [{"id": 1, "name": "Laptop"}]}`)

	tests := []struct {
		name          string
		extractorType string
		scope         string
		groups        []string
		expectedValue string
	}{
		{
			name:          "extract from request_url",
			extractorType: "regex",
			scope:         "request_url",
			groups:        []string{`category=(\w+)`},
			expectedValue: "electronics",
		},
		{
			name:          "extract from request_header",
			extractorType: "regex",
			scope:         "request_header",
			groups:        []string{`User-Agent: ([^\r\n]+)`},
			expectedValue: "Mozilla/5.0",
		},
		{
			name:          "extract from request_raw",
			extractorType: "regex",
			scope:         "request_raw",
			groups:        []string{`GET (/api/\w+)`},
			expectedValue: "/api/products",
		},
		{
			name:          "extract cookie from request_header",
			extractorType: "regex",
			scope:         "request_header",
			groups:        []string{`session=(\w+)`},
			expectedValue: "abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := &YakExtractor{
				Name:             "test_var",
				Type:             tt.extractorType,
				Scope:            tt.scope,
				Groups:           tt.groups,
				RegexpMatchGroup: []int{1},
			}

			result, err := extractor.ExecuteWithRequest(rsp, req, false)
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}

			value, ok := result["test_var"]
			if !ok {
				t.Fatalf("Expected variable 'test_var' not found in result")
			}

			if value != tt.expectedValue {
				t.Errorf("Expected %q, got %q", tt.expectedValue, value)
			}
		})
	}
}

// TestExtractor_RequestScope_POST tests extractor with POST request
func TestExtractor_RequestScope_POST(t *testing.T) {
	req := []byte(`POST /api/login HTTP/1.1
Host: example.com
Content-Type: application/x-www-form-urlencoded

username=admin&password=secret123`)

	rsp := []byte(`HTTP/1.1 200 OK

{"token": "xyz789"}`)

	tests := []struct {
		name          string
		extractorType string
		scope         string
		groups        []string
		expectedValue string
	}{
		{
			name:          "extract username from request_body",
			extractorType: "regex",
			scope:         "request_body",
			groups:        []string{`username=(\w+)`},
			expectedValue: "admin",
		},
		{
			name:          "extract password from request_body",
			extractorType: "regex",
			scope:         "request_body",
			groups:        []string{`password=(\w+)`},
			expectedValue: "secret123",
		},
		{
			name:          "extract endpoint from request_url",
			extractorType: "regex",
			scope:         "request_url",
			groups:        []string{`(/api/\w+)`},
			expectedValue: "/api/login",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := &YakExtractor{
				Name:             "test_var",
				Type:             tt.extractorType,
				Scope:            tt.scope,
				Groups:           tt.groups,
				RegexpMatchGroup: []int{1},
			}

			result, err := extractor.ExecuteWithRequest(rsp, req, false)
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}

			value, ok := result["test_var"]
			if !ok {
				t.Fatalf("Expected variable 'test_var' not found in result")
			}

			if value != tt.expectedValue {
				t.Errorf("Expected %q, got %q", tt.expectedValue, value)
			}
		})
	}
}

// TestRequestScope_WithoutRequest tests behavior when request is not provided
func TestRequestScope_WithoutRequest(t *testing.T) {
	rsp := []byte(`HTTP/1.1 200 OK

test`)

	// Matcher should not fail, just return false or empty match
	matcher := &YakMatcher{
		MatcherType: "word",
		Scope:       "request_body",
		Group:       []string{"test"},
	}

	result, err := matcher.Execute(&RespForMatch{
		RawPacket: rsp,
	}, nil)
	if err != nil {
		t.Fatalf("Execute should not fail: %v", err)
	}

	if result {
		t.Errorf("Expected false when request is not provided")
	}

	// Extractor should not fail, just return empty
	extractor := &YakExtractor{
		Name:   "test_var",
		Type:   "regex",
		Scope:  "request_header",
		Groups: []string{`test`},
	}

	extractResult, err := extractor.Execute(rsp)
	if err != nil {
		t.Fatalf("Execute should not fail: %v", err)
	}

	value, ok := extractResult["test_var"]
	if !ok {
		t.Fatalf("Expected variable 'test_var' in result")
	}

	// Should be nil or empty
	if value != nil && value != "" {
		t.Errorf("Expected nil or empty value when request is not provided, got %v", value)
	}
}

// TestNucleiDSL_IsHttps tests is_https variable
func TestNucleiDSL_IsHttps(t *testing.T) {
	reqHTTP := []byte(`GET /api/test HTTP/1.1
Host: example.com

`)
	reqHTTPS := []byte(`GET /api/secure HTTP/1.1
Host: secure.example.com

`)
	rsp := []byte(`HTTP/1.1 200 OK

OK`)

	tests := []struct {
		name       string
		req        []byte
		isHttps    bool
		dslExpr    string
		expectTrue bool
	}{
		{
			name:       "test is_https with HTTP request",
			req:        reqHTTP,
			isHttps:    false,
			dslExpr:    `is_https == false`,
			expectTrue: true,
		},
		{
			name:       "test is_https with HTTPS request",
			req:        reqHTTPS,
			isHttps:    true,
			dslExpr:    `is_https == true`,
			expectTrue: true,
		},
		{
			name:       "test request_url with HTTP",
			req:        reqHTTP,
			isHttps:    false,
			dslExpr:    `contains(request_url, "http://example.com")`,
			expectTrue: true,
		},
		{
			name:       "test request_url with HTTPS",
			req:        reqHTTPS,
			isHttps:    true,
			dslExpr:    `contains(request_url, "https://secure.example.com")`,
			expectTrue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := &YakMatcher{
				MatcherType: "expr",
				ExprType:    "nuclei-dsl",
				Group:       []string{tt.dslExpr},
			}

			result, err := matcher.Execute(&RespForMatch{
				RawPacket:     rsp,
				RequestPacket: tt.req,
				IsHttps:       tt.isHttps,
			}, nil)
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}

			if result != tt.expectTrue {
				t.Errorf("Expected %v, got %v for expression: %s", tt.expectTrue, result, tt.dslExpr)
			}
		})
	}
}

// TestExtractor_IsHttps tests is_https in extractor
func TestExtractor_IsHttps(t *testing.T) {
	reqHTTP := []byte(`GET /path HTTP/1.1
Host: example.com

`)
	reqHTTPS := []byte(`GET /secure HTTP/1.1
Host: secure.example.com

`)
	rsp := []byte(`HTTP/1.1 200 OK

test`)

	tests := []struct {
		name          string
		req           []byte
		isHttps       bool
		dslExpr       string
		expectedValue string
	}{
		{
			name:          "extract HTTP URL",
			req:           reqHTTP,
			isHttps:       false,
			dslExpr:       `request_url`,
			expectedValue: "http://example.com/path",
		},
		{
			name:          "extract HTTPS URL",
			req:           reqHTTPS,
			isHttps:       true,
			dslExpr:       `request_url`,
			expectedValue: "https://secure.example.com/secure",
		},
		{
			name:          "check is_https false",
			req:           reqHTTP,
			isHttps:       false,
			dslExpr:       `is_https`,
			expectedValue: "false",
		},
		{
			name:          "check is_https true",
			req:           reqHTTPS,
			isHttps:       true,
			dslExpr:       `is_https`,
			expectedValue: "true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := &YakExtractor{
				Name:   "test_var",
				Type:   "nuclei-dsl",
				Groups: []string{tt.dslExpr},
			}

			result, err := extractor.ExecuteWithRequest(rsp, tt.req, tt.isHttps)
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}

			value, ok := result["test_var"]
			if !ok {
				t.Fatalf("Expected variable 'test_var' not found in result")
			}

			valueStr := fmt.Sprintf("%v", value)
			if valueStr != tt.expectedValue {
				t.Errorf("Expected %q, got %q", tt.expectedValue, valueStr)
			}
		})
	}
}

// TestNucleiDSL_RealWorldScenarios tests real-world usage scenarios
func TestNucleiDSL_RealWorldScenarios(t *testing.T) {
	tests := []struct {
		name       string
		response   []byte
		dslExpr    string
		expectTrue bool
	}{
		{
			name: "detect JSON API success response",
			response: []byte(`HTTP/1.1 200 OK
Content-Type: application/json

{"status": "success", "code": 0}`),
			dslExpr:    `status_code == 200 && contains(content_type, "json") && contains(body, "success")`,
			expectTrue: true,
		},
		{
			name: "detect error page",
			response: []byte(`HTTP/1.1 500 Internal Server Error
Content-Type: text/html

<html><body><h1>Internal Server Error</h1></body></html>`),
			dslExpr:    `status_code == 500 && contains(body, "Internal Server Error")`,
			expectTrue: true,
		},
		{
			name: "detect redirect",
			response: []byte(`HTTP/1.1 302 Found
Location: /login
Content-Length: 0

`),
			dslExpr:    `status_code == 302 && contains(all_headers, "Location")`,
			expectTrue: true,
		},
		{
			name: "detect security headers",
			response: []byte(`HTTP/1.1 200 OK
X-Frame-Options: DENY
X-Content-Type-Options: nosniff
Strict-Transport-Security: max-age=31536000

test`),
			dslExpr:    `contains(all_headers, "X-Frame-Options") && contains(all_headers, "Strict-Transport-Security")`,
			expectTrue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rsp, _, _ := lowhttp.FixHTTPResponse(tt.response)
			matcher := &YakMatcher{
				MatcherType: "expr",
				ExprType:    "nuclei-dsl",
				Group:       []string{tt.dslExpr},
			}

			result, err := matcher.ExecuteRawResponse(rsp, nil)
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}

			if result != tt.expectTrue {
				t.Errorf("Expected %v, got %v for expression: %s", tt.expectTrue, result, tt.dslExpr)
			}
		})
	}
}
