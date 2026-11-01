package sharkcli

import (
	"bytes"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// Byte ranges require the complete suite-qualified owned encoding, consistent
// body literals and transport authentication across the worker JSON boundary.
// The phase is contextual; neither it nor the AEAD flag verifies the TLS MAC.
func tlsFinishedDetail(m map[string]any, version, suite uint64, authenticated bool) ([]byte, string) {
	if !authenticated || m["Authenticated"] != true || number(m["Type"]) != 20 ||
		number(m["Finished Version"]) != version || number(m["Finished Cipher Suite"]) != suite ||
		version > 0xffff || suite > 0xffff {
		return nil, ""
	}
	phase := textValue(m["Finished Phase"])
	if phase != "handshake" && (phase != "application" || version != 0x304) {
		return nil, ""
	}
	wire := bytesValue(m["Encoded Finished"])
	if len(wire) < 4 || number(m["Length"]) != uint64(len(wire)-4) {
		return nil, ""
	}
	verifyData, err := pcaputil.DecodeTLSFinished(wire, uint16(version), uint16(suite))
	if err != nil || !bytes.Equal(verifyData, bytesValue(m["Finished Verify Data"])) {
		return nil, ""
	}
	return wire, "TLS Finished · authenticated handshake plaintext · frame offsets unavailable"
}
