package sharkcli

import "fmt"

// Only a long header carries these four wire bytes. Short headers must keep
// their explicit absence text, rather than projecting a remembered version.
func quicWireVersionText(v any) string {
	version := number(v)
	name := "Unknown QUIC version"
	switch version {
	case 0:
		name = "Version Negotiation"
	case 1:
		name = "QUIC v1"
	case 0x6b3343cf:
		name = "QUIC v2"
	}
	return fmt.Sprintf("%s (0x%08x)", name, version)
}
