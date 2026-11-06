package match

import (
	"bytes"
	"strconv"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
)

// ftpBounceMatcher implements the ftpbounce keyword: a PORT command whose
// address does not equal the source address of the FTP client is a bounce
// attempt.
func ftpBounceMatcher(c *matchContext) error {
	source := sourceIPv4(c.PK)
	if source == nil {
		c.Reject()
		return nil
	}
	payload := c.GetBuffer(modifier.Default)
	for _, line := range bytes.Split(payload, []byte("\n")) {
		fields := bytes.Fields(bytes.TrimSuffix(line, []byte("\r")))
		if len(fields) < 2 || !bytes.EqualFold(fields[0], []byte("PORT")) {
			continue
		}
		address, ok := parseFTPPortAddress(fields[1])
		if !ok {
			continue
		}
		if !bytes.Equal(address, source) {
			return nil
		}
	}
	c.Reject()
	return nil
}

func sourceIPv4(packet gopacket.Packet) []byte {
	if packet == nil {
		return nil
	}
	switch network := packet.NetworkLayer().(type) {
	case *layers.IPv4:
		return network.SrcIP
	case *layers.IPv6:
		return network.SrcIP.To4()
	default:
		return nil
	}
}

func parseFTPPortAddress(raw []byte) ([]byte, bool) {
	parts := bytes.Split(raw, []byte(","))
	if len(parts) != 6 {
		return nil, false
	}
	address := make([]byte, 4)
	for index, part := range parts {
		value, err := strconv.Atoi(string(part))
		if err != nil || value < 0 || value > 255 {
			return nil, false
		}
		if index < len(address) {
			address[index] = byte(value)
		}
	}
	return address, true
}
