package match

import (
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
)

func snmpParser(c *matchContext) error {
	return tcpOrUDPParser(c)
}

func snmpMatcher(c *matchContext) error {
	if c.Rule == nil || c.Rule.ContentRuleConfig == nil || c.Rule.ContentRuleConfig.SNMPConfig == nil {
		return nil
	}
	cfg := c.Rule.ContentRuleConfig.SNMPConfig
	payload := c.GetBuffer(modifier.Default)
	version, ok := ParseSNMPVersion(payload)
	if !c.Must(ok) {
		return nil
	}
	if cfg.Version != nil {
		c.Must(cfg.Version.Match(version))
	}
	return nil
}

// ParseSNMPVersion decodes the ASN.1 BER outer SEQUENCE and version INTEGER of
// an SNMP message, returning Suricata's 1-based version number (1=v1, 2=v2c,
// 3=v3) and whether the payload structurally matches an SNMP message.
func ParseSNMPVersion(payload []byte) (int, bool) {
	if len(payload) < 8 || payload[0] != 0x30 {
		return 0, false
	}
	seqLen, seqHdr, ok := decodeBERLength(payload[1:])
	if !ok || seqLen < 5 || 1+seqHdr+seqLen > len(payload)+64 {
		// Allow slightly truncated packets in single-packet views, but reject
		// implausibly short or malformed BER lengths.
		return 0, false
	}
	at := 1 + seqHdr
	if at+3 > len(payload) || payload[at] != 0x02 {
		return 0, false
	}
	verLen, verHdr, ok := decodeBERLength(payload[at+1:])
	if !ok || verLen < 1 || verLen > 4 {
		return 0, false
	}
	valStart := at + 1 + verHdr
	valEnd := valStart + verLen
	if valEnd >= len(payload) {
		return 0, false
	}
	// Reject negative BER integers (MSB set on first octet).
	if payload[valStart]&0x80 != 0 {
		return 0, false
	}
	var wireVersion int
	for _, b := range payload[valStart:valEnd] {
		wireVersion = (wireVersion << 8) | int(b)
	}
	// Validate the tag immediately following the version field:
	// - SNMPv1 (0) and SNMPv2c (1) carry an OCTET STRING (0x04) community.
	// - SNMPv3 (3) carries a SEQUENCE (0x30) msgGlobalData.
	nextTag := payload[valEnd]
	switch wireVersion {
	case 0:
		if nextTag != 0x04 {
			return 0, false
		}
		return 1, true
	case 1:
		if nextTag != 0x04 {
			return 0, false
		}
		return 2, true
	case 3:
		if nextTag != 0x30 {
			return 0, false
		}
		return 3, true
	default:
		return 0, false
	}
}

func decodeBERLength(b []byte) (length int, headerBytes int, ok bool) {
	if len(b) == 0 {
		return 0, 0, false
	}
	first := b[0]
	if first&0x80 == 0 {
		return int(first), 1, true
	}
	numOctets := int(first & 0x7f)
	if numOctets == 0 || numOctets > 4 || len(b) < 1+numOctets {
		return 0, 0, false
	}
	val := 0
	for i := 0; i < numOctets; i++ {
		val = (val << 8) | int(b[1+i])
	}
	return val, 1 + numOctets, true
}
