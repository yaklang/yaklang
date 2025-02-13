package mcp

import "testing"

func TestMCPServer(t *testing.T) {
	s := NewMCPServer()
	if err := s.ServeSSE(":18083", "http://localhost:18083"); err != nil {
		panic(err)
	}
}
