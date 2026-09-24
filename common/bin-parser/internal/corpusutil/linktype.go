// Package corpusutil contains capture metadata helpers shared by the corpus
// generator and its independent local verification tests.
package corpusutil

import "github.com/gopacket/gopacket/layers"

// LinkTypeName preserves known capture formats even when gopacket has no
// built-in decoder for them. Values are LINKTYPE numbers, not Wireshark WTAP
// encapsulation numbers.
func LinkTypeName(link layers.LinkType) string {
	switch uint32(link) {
	case 6:
		return "Token Ring"
	case 195:
		return "IEEE 802.15.4"
	case 227:
		return "SocketCAN"
	case 230:
		return "IEEE 802.15.4 no FCS"
	case 249:
		return "USBPcap"
	default:
		return link.String()
	}
}
