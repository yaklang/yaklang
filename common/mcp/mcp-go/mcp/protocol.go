package mcp

const (
	ProtocolVersion2024_11_05 = "2024-11-05"
	ProtocolVersion2025_03_26 = "2025-03-26"
	ProtocolVersion2025_11_25 = "2025-11-25"

	// LATEST_PROTOCOL_VERSION is the most recent MCP protocol version
	// supported by this implementation.
	LATEST_PROTOCOL_VERSION = ProtocolVersion2025_11_25

	HeaderProtocolVersion = "MCP-Protocol-Version"
	HeaderSessionID       = "MCP-Session-Id"
	LegacyHeaderSessionID = "Mcp-Session-Id"
)

var SupportedProtocolVersions = []string{
	ProtocolVersion2025_11_25,
	ProtocolVersion2025_03_26,
	ProtocolVersion2024_11_05,
}

func IsSupportedProtocolVersion(version string) bool {
	for _, candidate := range SupportedProtocolVersions {
		if version == candidate {
			return true
		}
	}
	return false
}

func NegotiateProtocolVersion(requested string) (string, bool) {
	if requested == "" {
		return LATEST_PROTOCOL_VERSION, true
	}
	if IsSupportedProtocolVersion(requested) {
		return requested, true
	}
	return "", false
}
