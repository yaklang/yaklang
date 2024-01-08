package pingutil

import (
	"context"
	"fmt"
	"testing"
)

func TestTraceroute(t *testing.T) {
	rspChan, err := traceroute(context.Background(),"93.184.216.34")
	if err != nil {
		t.Fatal(err)
	}
	for rsp := range rspChan {
		if len(rsp) == 0 {
			continue
		}
		fmt.Printf("hop: %d\n", rsp[0].Hop)
		for _, response := range rsp {
			fmt.Printf("ip: %s, rtt: %dms, reason: %s\n", response.IPs, response.RTT, response.Reason)
		}
	}
}
