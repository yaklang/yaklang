package openapi

import (
	"strings"
	"testing"
)

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
