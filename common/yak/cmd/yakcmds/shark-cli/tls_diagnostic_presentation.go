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

// The native parser rejected a complete cleartext ServerHello declaration.
// Preserve the observed TLS layer without declaring a negotiated TLS version,
// successful handshake, authenticated record, or sticky connection identity.
func rejectedTLSServerHelloDiagnostic(e *pcaputil.ProtocolEvent) bool {
	if e == nil || e.Protocol != "tls" || e.Transport != "tcp" || e.Profile != "tls" ||
		e.ID == 0 || e.FlowID == 0 || e.TCPFlowID == 0 || e.Status != "malformed" ||
		e.ExpertCode != string(pcaputil.ErrMalformedMessage) || e.Error != "MalformedMessage: TLS 1.3 ServerHello legacy parameters" {
		return false
	}
	info := eventValues(e)
	if info["Authentication Verified"] != false || info["Content Visibility"] != "cleartext" ||
		number(info["Negotiated Version"]) != 0 || number(info["Record Type"]) != 22 ||
		number(info["Record Version"]) != 0x303 || number(info["Record Length"]) != uint64(len(e.Raw)-5) || e.Length != len(e.Raw) {
		return false
	}
	observation, err := pcaputil.ObserveTLSServerHelloRecord(e.Raw, 4096)
	if err != nil || observation.RecordVersion != 0x303 || observation.SelectedVersion != 0x304 {
		return false
	}
	switch observation.CipherSuite {
	case 0x1301, 0x1302, 0x1303:
	default:
		return false
	}
	if e.SourceBytes.Kind != "reassembled" || len(e.SourceBytes.PacketRefs) == 0 || e.SourceBytes.SHA256 != fmt.Sprintf("%x", sha256.Sum256(e.Raw)) {
		return false
	}
	for _, ref := range e.SourceBytes.PacketRefs {
		if ref.Number == 0 || ref.Domain != e.Domain {
			return false
		}
	}
	return true
}

// A stopped decoder can retain a bounded prefix of several remaining records.
// This is a stream extent, not a successfully framed or authenticated TLS PDU.
func tlsStoppedStreamRemainder(e *pcaputil.ProtocolEvent) bool {
	return e != nil && e.Protocol == "tls" && e.Transport == "tcp" && e.Profile == "tls" &&
		e.Status == "context-required" && e.ExpertCode == string(pcaputil.ErrContextRequired) &&
		e.Error == e.ExpertCode+": decode failure invalidated stream affinity" &&
		e.SourceBytes.Kind == "reassembled" && len(e.Raw) > 0 && e.Length >= len(e.Raw)
}
