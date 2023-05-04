package cybertunnel

import "testing"

func TestGetTunnelServerExternalIP(t *testing.T) {
	rsp, err := QueryExistedDNSLogEvents("127.0.0.1:64333", "abc")
	_ = err
	println(rsp)
}
