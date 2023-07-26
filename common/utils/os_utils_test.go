package utils

import (
	"fmt"
	"math/rand"
	"net"
	"testing"
	"time"
)

func TestIsPortAvailable(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	var port = -1
	for !IsTCPPortAvailable(port) {
		port = rand.Intn(4000) + 60000
	}

	if !IsTCPPortAvailable(port) {
		t.Logf("tcp port is not available: %v", port)
		t.FailNow()
	}

	lis, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%v", port))
	if err != nil {
		t.Logf("listen failed: %s", err)
		t.FailNow()
	}
	defer lis.Close()

	if IsTCPPortAvailable(port) {
		t.Logf("port: %v have been used", port)
		t.FailNow()
	}
}
