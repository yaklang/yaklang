package sharkcli

import "fmt"

// Exact TLS-owned metadata keys only. A generic Version/Protocol Version or a
// similarly valued field in another protocol must retain its literal value.
func tlsMetadataVersion(name string, value any) (string, bool) {
	switch name {
	case "Certificate Status Version", "Certificate Request Version", "Certificate Version", "Certificate Verify Version", "Finished Version", "Server Key Exchange Version", "Client Key Exchange Version", "TLS Boundary Version", "TLS Gap Version":
	default:
		return "", false
	}
	code, ok := gpeHeaderInteger(value, 0xffff)
	if !ok {
		return "", false
	}
	label := map[uint64]string{0x300: "SSL 3.0", 0x301: "TLS 1.0", 0x302: "TLS 1.1", 0x303: "TLS 1.2", 0x304: "TLS 1.3"}[code]
	if label == "" {
		return "", false
	}
	return fmt.Sprintf("%s (0x%04x)", label, code), true
}
