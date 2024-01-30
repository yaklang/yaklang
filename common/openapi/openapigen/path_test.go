package openapigen

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestExtractQueryParams(t *testing.T) {
	params := extractQueryParams("/api/v1/brute/123?test=1&test2=2")
	spew.Dump(params)
	if len(params) == 2 {
		t.Log("ok")
	} else {
		t.Errorf("want %v, got %v", 2, len(params))
	}
}

func TestShrinkPath(t *testing.T) {
	after, params, _, _ := shrinkPath("/api/v1/brute/123", "/api/v1/brute/112")
	spew.Dump(after, params)
	if after == "/api/v1/brute/{bruteId}" {
		t.Log("ok")
	} else {
		t.Errorf("want %v, got %v", "/api/v1/brute/{bruteId}", after)
	}

	if len(params) == 1 {
		t.Log("ok")
	} else {
		t.Errorf("want %v, got %v", 1, len(params))
	}
}

func TestShrinkPath_1(t *testing.T) {
	after, params, _, _ := shrinkPath("/api/v1/brute/123/list", "/api/v1/brute/112/list")
	spew.Dump(after, params)
	if after == "/api/v1/brute/{bruteId}/list" {
		t.Log("ok")
	} else {
		t.Errorf("want %v, got %v", "/api/v1/brute/{bruteId}/list", after)
	}

	if len(params) == 1 {
		t.Log("ok")
	} else {
		t.Errorf("want %v, got %v", 1, len(params))
	}
}

func TestShrinkPath_2(t *testing.T) {
	after, params, _, _ := shrinkPath("/api/v1/brute/123/1", "/api/v1/brute/112/2")
	spew.Dump(after, params)
	if after == "/api/v1/brute/{bruteId}/{bruteId2}" {
		t.Log("ok")
	} else {
		t.Errorf("want %v, got %v", "/api/v1/brute/{bruteId}/list", after)
	}

	if len(params) == 2 {
		t.Log("ok")
	} else {
		t.Errorf("want %v, got %v", 1, len(params))
	}
}

func TestShrinkPath2(t *testing.T) {
	after, params, isSame, _ := shrinkPath("/api/v1/brute/123", "/api/v1/brute/123")
	assert.Equal(t, isSame, true)
	test := assert.New(t)
	test.Equal(after, "/api/v1/brute/123")
	test.Equal(len(params), 0)
}
