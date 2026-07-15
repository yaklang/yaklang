package openapi

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestParseDocument_CircularSwaggerV2Definitions(t *testing.T) {
	content := `{
  "swagger": "2.0",
  "info": {"title": "circular", "version": "1.0"},
  "host": "example.com",
  "paths": {
    "/node": {
      "post": {
        "consumes": ["application/json"],
        "parameters": [{
          "in": "body",
          "name": "body",
          "schema": {"$ref": "#/definitions/Node"}
        }],
        "responses": {"200": {"description": "ok"}}
      }
    }
  },
  "definitions": {
    "Node": {
      "type": "object",
      "properties": {
        "name": {"type": "string"},
        "child": {"$ref": "#/definitions/Node"}
      }
    }
  }
}`
	done := make(chan struct{})
	go func() {
		defer close(done)
		doc, err := ParseDocument(content, nil)
		if err != nil {
			t.Errorf("parse failed: %v", err)
			return
		}
		if len(doc.Operations) != 1 {
			t.Errorf("want 1 op, got %d", len(doc.Operations))
		}
		if doc.Operations[0].RequestBody == nil {
			t.Errorf("want request body")
			return
		}
		body := doc.Operations[0].RequestBody.Content["application/json"]
		if body == "" {
			t.Errorf("want mocked body json")
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("circular swagger schema mock hung")
	}
}

func TestParseDocument_CircularOpenAPIV3Schemas(t *testing.T) {
	content := `{
  "openapi": "3.0.0",
  "info": {"title": "circular", "version": "1.0"},
  "paths": {
    "/node": {
      "post": {
        "requestBody": {
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/Node"}
            }
          }
        },
        "responses": {"200": {"description": "ok"}}
      }
    }
  },
  "components": {
    "schemas": {
      "Node": {
        "type": "object",
        "properties": {
          "name": {"type": "string"},
          "child": {"$ref": "#/components/schemas/Node"}
        }
      }
    }
  }
}`
	done := make(chan struct{})
	go func() {
		defer close(done)
		doc, err := ParseDocument(content, nil)
		if err != nil {
			t.Errorf("parse failed: %v", err)
			return
		}
		if len(doc.Operations) != 1 {
			t.Errorf("want 1 op, got %d", len(doc.Operations))
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("circular openapi3 schema mock hung")
	}
}

func TestParseDocument_Cancel(t *testing.T) {
	content := `{
  "openapi": "3.0.0",
  "info": {"title": "cancel", "version": "1.0"},
  "paths": {
    "/a": {"get": {"responses": {"200": {"description": "ok"}}}},
    "/b": {"get": {"responses": {"200": {"description": "ok"}}}},
    "/c": {"get": {"responses": {"200": {"description": "ok"}}}}
  }
}`
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ParseDocument(content, &ParseOptions{Context: ctx})
	if err == nil {
		t.Fatal("expected cancel error")
	}
	if !strings.Contains(err.Error(), "canceled") && !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseDocument_ProgressCallback(t *testing.T) {
	content := `{
  "openapi": "3.0.0",
  "info": {"title": "progress", "version": "1.0"},
  "paths": {
    "/a": {"get": {"responses": {"200": {"description": "ok"}}}}
  }
}`
	var stages []string
	_, err := ParseDocument(content, &ParseOptions{
		OnProgress: func(p ParseProgress) {
			stages = append(stages, p.Stage)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(stages) == 0 {
		t.Fatal("expected progress callbacks")
	}
}
