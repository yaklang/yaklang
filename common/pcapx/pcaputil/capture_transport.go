package pcaputil

import "github.com/gopacket/gopacket"

// CaptureTransport returns the innermost observed network/transport pair.
// gopacket's NetworkLayer/TransportLayer getters retain the outer carrier;
// mixing those addresses with an inner TCP header creates a false endpoint.
// This is a header view only, not application admission or tunnel reassembly.
func CaptureTransport(p gopacket.Packet) (gopacket.NetworkLayer, gopacket.TransportLayer) {
	var network gopacket.NetworkLayer
	var transport gopacket.TransportLayer
	for _, layer := range p.Layers() {
		if n, ok := layer.(gopacket.NetworkLayer); ok {
			network, transport = n, nil
		}
		if t, ok := layer.(gopacket.TransportLayer); ok && network != nil {
			transport = t
		}
	}
	return network, transport
}
