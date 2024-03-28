package facades

import "testing"

func TestRmiServer_Serve(t *testing.T) {
	server := NewFacadeServer("127.0.0.1", 8089, SetRmiResourceAddr("aaa", ""))
	server.Serve()
}
