package synscanx

import (
	"sync"

	"github.com/yaklang/yaklang/common/fp"
)

type udpPayloadCacheEntry struct {
	payload []byte
}

var udpPayloadCache sync.Map

// udpProbePayload is the application data handed to the gVisor UDP endpoint.
// The stack builds the UDP, IP and link headers. An unknown port is an empty
// datagram. The first nmap probe with a payload is used; an empty best match
// does not hide a later payload probe. The rule walk is cached per port.
func udpProbePayload(port int) []byte {
	if cached, ok := udpPayloadCache.Load(port); ok {
		return cached.(udpPayloadCacheEntry).payload
	}
	config := fp.NewConfig(
		fp.WithActiveMode(true),
		fp.WithTransportProtos(fp.UDP),
		fp.WithProbesMax(3),
	)
	_, blocks, _ := fp.GetRuleBlockByConfig(port, config)
	var payload []byte
	for _, block := range blocks {
		if block == nil || block.Probe == nil || block.Probe.Payload == "" {
			continue
		}
		payload = []byte(block.Probe.Payload)
		break
	}
	udpPayloadCache.Store(port, udpPayloadCacheEntry{payload: payload})
	return payload
}
