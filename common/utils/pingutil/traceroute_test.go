package pingutil

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
	"math/rand"
	"testing"
)

func TestTraceroute(t *testing.T) {
	expectStr := ""
	rspChan, err := Traceroute("8.8.8.8", WithSender(func(config *TracerouteConfig, host string, hop int) (*TracerouteResponse, error) {
		response := &TracerouteResponse{
			Hop:    hop,
			IP:     utils.GetRandomIPAddress(),
			RTT:    int64(rand.Intn(1000)),
			Reason: utils.RandStringBytes(1),
		}
		expectStr += fmt.Sprintf("hop: %d, ip: %s, rtt: %dms, reason: %s\n", response.Hop, response.IP, response.RTT, response.Reason)
		return response, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	actualResp := ""
	for response := range rspChan {
		actualResp += fmt.Sprintf("hop: %d, ip: %s, rtt: %dms, reason: %s\n", response.Hop, response.IP, response.RTT, response.Reason)
	}
	assert.Equal(t, expectStr, actualResp)
}
