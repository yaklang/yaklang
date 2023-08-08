package dap

import (
	"testing"

	"github.com/google/go-dap"
)

func (c *TestClient) ExpectInitializeResponse(t *testing.T) *dap.InitializeResponse {
	t.Helper()
	m := c.ExpectMessage(t)
	return c.CheckInitializeResponse(t, m)
}

func (c *TestClient) CheckInitializeResponse(t *testing.T, m dap.Message) *dap.InitializeResponse {
	t.Helper()
	r, ok := m.(*dap.InitializeResponse)
	if !ok {
		t.Fatalf("got %#v, want *dap.InitializeResponse", m)
	}
	return r
}
