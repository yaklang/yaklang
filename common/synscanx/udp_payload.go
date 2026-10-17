package synscanx

import "github.com/yaklang/yaklang/common/fp"

// udpProbePayload is the application data handed to the gVisor UDP endpoint.
// The stack builds the UDP, IP and link headers. An unknown port is an empty
// datagram; a service-specific nmap probe is used when one is the best match.
func udpProbePayload(port int) []byte {
	config := fp.NewConfig(
		fp.WithActiveMode(true),
		fp.WithTransportProtos(fp.UDP),
		fp.WithProbesMax(3),
	)
	_, blocks, best := fp.GetRuleBlockByConfig(port, config)
	if len(blocks) == 0 || !best || blocks[0] == nil || blocks[0].Probe == nil || blocks[0].Probe.Payload == "" {
		return nil
	}
	return []byte(blocks[0].Probe.Payload)
}
