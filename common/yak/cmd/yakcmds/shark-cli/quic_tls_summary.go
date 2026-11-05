package sharkcli

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// Project complete messages already admitted by the native CRYPTO parser.
// Packet AEAD and TLS handshake authentication remain separate, particularly
// for public Initial protection. Neither label selects a negotiated version.
func quicTLSMessageSummary(e *pcaputil.ProtocolEvent) (string, int) {
	if e == nil || e.Protocol != "quic" || e.Error != "" || e.Status != "decoded" && e.Status != "deferred" {
		return "", 0
	}
	v := eventValues(e)
	if v["Native Carrier"] != true || v["Authentication Verified"] != true {
		return "", 0
	}
	space := textValue(v["Packet Number Space"])
	var names []string
	count, rank := 0, 0
	for _, message := range mapList(v["TLS Handshakes"]) {
		length, complete := detailRangeOffset(message["Length"])
		authenticated, known := message["Authenticated"].(bool)
		if !complete || length > 0xffffff || !known || authenticated != (space != "initial") {
			continue
		}
		typ := number(message["Type"])
		level := "handshake"
		switch typ {
		case 1, 2:
			level = "initial"
		case 4:
			level = "application"
		case 8, 11, 13, 15, 20, 25:
		default:
			continue // In particular, QUIC does not carry TLS KeyUpdate.
		}
		if level != space {
			continue
		}
		count++
		rank = max(rank, 1)
		if typ == 15 {
			rank = max(rank, tlsCertificateVerifyRank)
		}
		if len(names) < 8 {
			names = append(names, handshakeType(message["Type"]))
		}
	}
	if count == 0 {
		return "", 0
	}
	summary := "TLS " + strings.Join(names, ", ")
	if count > len(names) {
		summary += fmt.Sprintf(" (+%d further TLS messages)", count-len(names))
	}
	return summary, rank
}
