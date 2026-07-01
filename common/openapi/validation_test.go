package openapi

import "testing"

func TestIsValidResponseStatusCode(t *testing.T) {
	cases := []struct {
		code string
		want bool
	}{
		{"200", true},
		{"404", true},
		{"default", true},
		{"aaaa", false},
		{"20", false},
		{"600", false},
		{"abc", false},
	}
	for _, c := range cases {
		if got := IsValidResponseStatusCode(c.code); got != c.want {
			t.Fatalf("IsValidResponseStatusCode(%q) = %v, want %v", c.code, got, c.want)
		}
	}
}

func TestSanitizeParsedDocumentSkipsNonStandardResponseKey(t *testing.T) {
	doc := &ParsedDocument{
		IsSwaggerV2: true,
		Operations: []OperationInfo{{
			Method: "POST",
			Path:   "/user",
			Responses: []ResponseSummary{
				{StatusCode: "default", Description: "successful operation"},
				{StatusCode: "aaaa", Description: "invalid"},
			},
		}},
	}
	sanitizeParsedDocument(doc)
	if len(doc.Operations[0].Responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(doc.Operations[0].Responses))
	}
	if doc.Operations[0].Responses[0].StatusCode != "default" {
		t.Fatalf("unexpected response: %+v", doc.Operations[0].Responses[0])
	}
	if len(doc.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(doc.Warnings))
	}
}

func TestTryLenientParseDocument(t *testing.T) {
	content := `{
  "swagger": "2.0",
  "info": {"title": "Partial API", "version": "1.0.0"},
  "host": "api.example.com",
  "paths": {
    "/users": {
      "get": {
        "summary": "List users",
        "responses": {
          "200": {"description": "OK"},
          "aaaa": {"description": "bad key"}
        }
      }
    }
  }
}`
	doc, warnings, err := tryLenientParseDocument(content, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(doc.Operations))
	}
	sanitizeParsedDocument(doc)
	if len(doc.Operations[0].Responses) != 1 {
		t.Fatalf("expected sanitized response count 1, got %d", len(doc.Operations[0].Responses))
	}
	if len(warnings) == 0 {
		t.Fatal("expected lenient warnings")
	}
}

func TestDescribeParseFailureInvalidJSON(t *testing.T) {
	err := describeParseFailure("{bad", nil, nil)
	if err == nil || err.Error() == "" {
		t.Fatal("expected syntax error")
	}
}
