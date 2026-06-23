package openapi

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

//go:embed openapi2/testdata/swagger_request_cases.json
var swaggerPetstoreRequestCasesJSON string

type swaggerRequestTestConfig struct {
	Document               string                         `json:"document"`
	DefaultParameterValues map[string]string              `json:"defaultParameterValues"`
	Cases                  []swaggerRequestTestCaseConfig `json:"cases"`
}

type swaggerRequestTestCaseConfig struct {
	Name            string                    `json:"name"`
	OperationID     string                    `json:"operationId"`
	Path            string                    `json:"path"`
	Method          string                    `json:"method"`
	ParameterValues map[string]string         `json:"parameterValues,omitempty"`
	Expect          swaggerRequestExpectation `json:"expect"`
}

type swaggerRequestExpectation struct {
	ExpectedRequest    string            `json:"expectedRequest,omitempty"`
	RequestLine        string            `json:"requestLine,omitempty"`
	Host               string            `json:"host,omitempty"`
	HeaderEquals       map[string]string `json:"headerEquals,omitempty"`
	HeaderPrefix       map[string]string `json:"headerPrefix,omitempty"`
	BodyMustContain    []string          `json:"bodyMustContain,omitempty"`
	BodyMustNotContain []string          `json:"bodyMustNotContain,omitempty"`
	BodyJsonKeys       []string          `json:"bodyJsonKeys,omitempty"`
	BodyJsonArray      bool              `json:"bodyJsonArray,omitempty"`
}

func TestJoinOpenAPIPath(t *testing.T) {
	cases := []struct {
		base string
		path string
		want string
	}{
		{"/", "/users", "/users"},
		{"/", "users", "/users"},
		{"/api/v1", "/users", "/api/v1/users"},
		{"/api/v1/", "/users", "/api/v1/users"},
		{"", "/users", "/users"},
	}
	for _, c := range cases {
		got := joinOpenAPIPath(c.base, c.path)
		if got != c.want {
			t.Fatalf("joinOpenAPIPath(%q, %q) = %q, want %q", c.base, c.path, got, c.want)
		}
	}
}

func TestBuildSwaggerV2OperationRequestsRootBasePath(t *testing.T) {
	content := `{
  "swagger": "2.0",
  "info": {"title": "t", "version": "1.0.0"},
  "host": "api.example.com",
  "schemes": ["https"],
  "basePath": "/",
  "paths": {
    "/users": {
      "get": {
        "responses": {"200": {"description": "OK"}}
      }
    }
  }
}`
	reqs, isHttps, err := BuildOperationRequests(content, "/users", "GET", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) == 0 {
		t.Fatal("expected request")
	}
	raw := string(reqs[0])
	if !strings.Contains(raw, "/users") {
		t.Fatalf("unexpected request path: %s", raw)
	}
	if strings.Contains(raw, "//users") {
		t.Fatalf("unexpected double slash: %s", raw)
	}
	if !isHttps {
		t.Fatal("expected https")
	}
}

func TestBuildSwaggerV2PetstoreRequestsFromConfig(t *testing.T) {
	cfg := loadSwaggerRequestTestConfig(t)

	parsed, err := ParseDocument(openapi2demo, nil)
	if err != nil {
		t.Fatal(err)
	}
	opIndex := make(map[string]OperationInfo, len(parsed.Operations))
	for _, op := range parsed.Operations {
		opIndex[op.OperationId] = op
	}

	if len(cfg.Cases) != len(parsed.Operations) {
		t.Fatalf("case count mismatch: config=%d document=%d", len(cfg.Cases), len(parsed.Operations))
	}

	seen := make(map[string]struct{}, len(cfg.Cases))
	for _, tc := range cfg.Cases {
		if tc.OperationID == "" {
			t.Fatalf("case %q missing operationId", tc.Name)
		}
		if _, ok := opIndex[tc.OperationID]; !ok {
			t.Fatalf("case %q references unknown operationId %q", tc.Name, tc.OperationID)
		}
		if tc.Path == "" || tc.Method == "" {
			t.Fatalf("case %q missing path or method", tc.Name)
		}
		key := strings.ToUpper(tc.Method) + " " + tc.Path
		if _, dup := seen[key]; dup {
			t.Fatalf("duplicate case for %s", key)
		}
		seen[key] = struct{}{}

		buildOpts := &BuildOptions{
			ParameterValues: mergeParameterValues(cfg.DefaultParameterValues, tc.ParameterValues),
		}
		reqs, isHTTPS, err := BuildOperationRequests(openapi2demo, tc.Path, tc.Method, buildOpts)
		if err != nil {
			t.Fatalf("%s (%s %s): build failed: %v", tc.Name, tc.Method, tc.Path, err)
		}
		if len(reqs) == 0 {
			t.Fatalf("%s (%s %s): no request generated", tc.Name, tc.Method, tc.Path)
		}
		if !isHTTPS {
			t.Fatalf("%s (%s %s): petstore swagger.json should use https", tc.Name, tc.Method, tc.Path)
		}

		assertSwaggerRequestExpectation(t, tc.Name, reqs[0], tc.Expect)
	}
}

func loadSwaggerRequestTestConfig(t *testing.T) *swaggerRequestTestConfig {
	t.Helper()
	var cfg swaggerRequestTestConfig
	if err := json.Unmarshal([]byte(swaggerPetstoreRequestCasesJSON), &cfg); err != nil {
		t.Fatalf("parse swagger_request_cases.json failed: %v", err)
	}
	if len(cfg.Cases) == 0 {
		t.Fatal("swagger_request_cases.json has no cases")
	}
	return &cfg
}

func mergeParameterValues(defaults, overrides map[string]string) map[string]string {
	merged := make(map[string]string, len(defaults)+len(overrides))
	for k, v := range defaults {
		merged[k] = v
	}
	for k, v := range overrides {
		merged[k] = v
	}
	return merged
}

func normalizeHTTPRequestRaw(raw []byte) string {
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(raw)))
	if err != nil {
		return strings.ReplaceAll(string(raw), "\r\n", "\n")
	}
	defer req.Body.Close()

	var buf bytes.Buffer
	buf.WriteString(strings.ToUpper(req.Method))
	buf.WriteByte(' ')
	buf.WriteString(req.URL.RequestURI())
	buf.WriteString(" HTTP/1.1\n")
	buf.WriteString("Host: ")
	buf.WriteString(req.Host)
	buf.WriteByte('\n')

	for key, values := range req.Header {
		if strings.EqualFold(key, "Content-Length") {
			continue
		}
		for _, value := range values {
			buf.WriteString(key)
			buf.WriteString(": ")
			buf.WriteString(value)
			buf.WriteByte('\n')
		}
	}
	buf.WriteByte('\n')

	body, _ := io.ReadAll(req.Body)
	if len(body) > 0 {
		buf.Write(body)
	}
	return buf.String()
}

func stripMultipartBoundary(raw string) string {
	re := regexp.MustCompile(`(?i)boundary=[^\r\n]+`)
	return re.ReplaceAllString(raw, "boundary={{BOUNDARY}}")
}

func assertSwaggerRequestExpectation(t *testing.T, caseName string, gotRaw []byte, expect swaggerRequestExpectation) {
	t.Helper()

	got := normalizeHTTPRequestRaw(gotRaw)

	if expect.ExpectedRequest != "" {
		want := strings.ReplaceAll(expect.ExpectedRequest, "\r\n", "\n")
		if got != want {
			t.Fatalf("%s: request mismatch\n--- got ---\n%s\n--- want ---\n%s", caseName, got, want)
		}
		return
	}

	lines := strings.SplitN(got, "\n\n", 2)
	head := lines[0]
	body := ""
	if len(lines) > 1 {
		body = lines[1]
	}

	headLines := strings.Split(head, "\n")
	if len(headLines) == 0 {
		t.Fatalf("%s: empty request", caseName)
	}
	if expect.RequestLine != "" && headLines[0] != expect.RequestLine {
		t.Fatalf("%s: request line mismatch\n got: %q\nwant: %q", caseName, headLines[0], expect.RequestLine)
	}

	headerMap := make(map[string]string)
	for _, line := range headLines[1:] {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) != 2 {
			continue
		}
		headerMap[parts[0]] = parts[1]
	}

	if expect.Host != "" && headerMap["Host"] != expect.Host {
		t.Fatalf("%s: host mismatch\n got: %q\nwant: %q", caseName, headerMap["Host"], expect.Host)
	}
	for key, want := range expect.HeaderEquals {
		gotVal, ok := headerMap[key]
		if !ok {
			t.Fatalf("%s: missing header %q", caseName, key)
		}
		if gotVal != want {
			t.Fatalf("%s: header %q mismatch\n got: %q\nwant: %q", caseName, key, gotVal, want)
		}
	}
	for key, prefix := range expect.HeaderPrefix {
		gotVal, ok := headerMap[key]
		if !ok {
			t.Fatalf("%s: missing header %q", caseName, key)
		}
		if !strings.HasPrefix(gotVal, prefix) {
			t.Fatalf("%s: header %q prefix mismatch\n got: %q\nwant prefix: %q", caseName, key, gotVal, prefix)
		}
	}

	bodyForCheck := stripMultipartBoundary(body)
	for _, needle := range expect.BodyMustContain {
		if !strings.Contains(bodyForCheck, needle) {
			t.Fatalf("%s: body must contain %q\nbody:\n%s", caseName, needle, body)
		}
	}
	for _, needle := range expect.BodyMustNotContain {
		if strings.Contains(bodyForCheck, needle) {
			t.Fatalf("%s: body must not contain %q\nbody:\n%s", caseName, needle, body)
		}
	}

	if len(expect.BodyJsonKeys) == 0 {
		return
	}
	if body == "" || body == "[]" {
		t.Fatalf("%s: expected non-empty json body", caseName)
	}
	var payload any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("%s: body is not valid json: %v\nbody: %s", caseName, err, body)
	}
	if expect.BodyJsonArray {
		arr, ok := payload.([]any)
		if !ok || len(arr) == 0 {
			t.Fatalf("%s: expected json array body", caseName)
		}
		payload = arr[0]
	}
	obj, ok := payload.(map[string]any)
	if !ok {
		t.Fatalf("%s: expected json object body", caseName)
	}
	for _, key := range expect.BodyJsonKeys {
		if _, ok := obj[key]; !ok {
			t.Fatalf("%s: json body missing key %q\nbody: %s", caseName, key, body)
		}
	}
}
