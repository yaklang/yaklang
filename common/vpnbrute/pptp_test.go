package vpnbrute

import "testing"

func TestAAABBB(t *testing.T) {
	a := &PPTPAuth{
		Target: "172.27.167.206:1723",
	}
	a.Auth()
}
