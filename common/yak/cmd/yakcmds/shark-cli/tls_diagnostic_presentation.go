package sharkcli

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// A rejected handshake does not erase an authenticated TLS record identity.
// This projection consumes the parser's record-authentication observation; it
// does not certify the rejected message or independently authenticate a DTO.
func authenticatedTLSRecordDiagnostic(e *pcaputil.ProtocolEvent) bool {
	if e == nil || e.Protocol != "tls" || e.Transport != "tcp" || e.FlowID == 0 || e.ID == 0 ||
		e.Status != "malformed" || e.ExpertCode != string(pcaputil.ErrMalformedMessage) || !strings.HasPrefix(e.Error, e.ExpertCode+": ") {
		return false
	}
	info := eventValues(e)
	if info["Authentication Verified"] != true || info["Content Visibility"] != "authenticated-plaintext" ||
		number(info["Negotiated Version"]) != 0x304 {
		return false
	}
	inner := number(info["Inner Content Type"])
	if inner != 22 && (inner != 23 || e.Error != e.ExpertCode+": TLS post-handshake response interleaved") {
		return false
	}
	switch number(info["Cipher Suite"]) {
	case 0x1301, 0x1302, 0x1303:
	default:
		return false
	}
	wire := e.Raw
	if len(wire) < 22 || wire[0] != 23 || wire[1] != 3 || wire[2] != 3 || int(wire[3])<<8|int(wire[4]) != len(wire)-5 ||
		number(info["Record Type"]) != 23 || number(info["Record Version"]) != 0x303 || number(info["Record Length"]) != uint64(len(wire)-5) {
		return false
	}
	if e.SourceBytes.Kind != "reassembled" || len(e.SourceBytes.PacketRefs) == 0 || e.SourceBytes.SHA256 != fmt.Sprintf("%x", sha256.Sum256(wire)) {
		return false
	}
	for _, ref := range e.SourceBytes.PacketRefs {
		if ref.Number == 0 || ref.Domain != e.Domain {
			return false
		}
	}
	return true
}
