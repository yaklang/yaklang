package pingutil

import (
	"fmt"
	"testing"
)

func _TestTraceroute(t *testing.T) {
	rspChan, err := Traceroute("93.184.216.34")
	if err != nil {
		t.Fatal(err)
	}
	for response := range rspChan {
		fmt.Printf("hop: %d, ip: %s, rtt: %dms, reason: %s\n", response.Hop, response.IP, response.RTT, response.Reason)
	}
}
