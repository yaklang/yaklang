package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

func ipv4OptionsDatagram(protocol byte, options, body []byte) []byte {
	ip := make([]byte, 20+len(options))
	ip[0], ip[8], ip[9] = 0x40|byte(len(ip)/4), 64, protocol
	binary.BigEndian.PutUint16(ip[2:4], uint16(len(ip)+len(body)))
	copy(ip[12:20], []byte{198, 51, 100, 1, 198, 51, 100, 2})
	copy(ip[20:], options)
	return append(ip, body...)
}

func ipv4OptionsBody(protocol byte) []byte {
	switch protocol {
	case 6:
		b := make([]byte, 20)
		copy(b, []byte{0x9c, 0x40, 0x9c, 0x41})
		b[12], b[13] = 0x50, 0x10
		return b
	case 17:
		return []byte{0x9c, 0x40, 0x9c, 0x41, 0, 8, 0, 0}
	case 1:
		return []byte{8, 0, 0, 0, 0x12, 0x34, 0x56, 0x78}
	case 4:
		return ipv4OptionsDatagram(17, nil, ipv4OptionsBody(17))
	case 41:
		b := make([]byte, 48)
		b[0], b[5], b[6], b[7], b[23], b[39] = 0x60, 8, 17, 64, 1, 2
		copy(b[40:], ipv4OptionsBody(17))
		return b
	default:
		return []byte{0x9c, 0x40, 0x9c, 0x41, 0, 8, 0, 0}
	}
}

func ipv4OptionsCarrier(ip []byte, link string) ([]byte, string, string, int) {
	if link == "IP" {
		return ip, "internet_protocol", "Internet Protocol", 0
	}
	ether := []byte{2, 0, 0, 0, 0, 2, 2, 0, 0, 0, 0, 1, 8, 0}
	if link == "VLAN" {
		ether = append(ether[:12], 0x81, 0, 0, 100, 8, 0)
	}
	return append(ether, ip...), "ethernet", "Ethernet", len(ether)
}

func TestProtocolCorpusIPv4OptionsPrecedeDispatch(t *testing.T) {
	for _, protocol := range []byte{6, 17, 1, 4, 41, 253} {
		for words := 5; words <= 15; words++ {
			for _, link := range []string{"IP", "Ethernet", "VLAN"} {
				t.Run(fmt.Sprintf("protocol%d/IHL%d/%s", protocol, words, link), func(t *testing.T) {
					options := bytes.Repeat([]byte{1}, (words-5)*4)
					if len(options) > 0 {
						options[len(options)-1] = 0
					}
					ip := ipv4OptionsDatagram(protocol, options, ipv4OptionsBody(protocol))
					wire, rule, entry, at := ipv4OptionsCarrier(ip, link)
					n := protocolCorpusRequireBoundedRuleParse(t, wire, rule, entry)
					if len(options) > 0 {
						protocolCorpusRequireValue(t, n, "IP Header Options", options)
						dvmrpRequireSpan(t, n, "IP Header Options", uint64(at+20)*8, uint64(at+words*4)*8)
					}
					transport := at + words*4
					if protocol == 4 {
						transport += 20
					} else if protocol == 41 {
						transport += 40
					}
					if protocol == 6 || protocol == 17 || protocol == 4 || protocol == 41 {
						protocolCorpusRequireValue(t, n, "Source Port", uint64(40000))
						protocolCorpusRequireValue(t, n, "Destination Port", uint64(40001))
						dvmrpRequireSpan(t, n, "Source Port", uint64(transport)*8, uint64(transport+2)*8)
					} else if protocol == 1 {
						icmp := protocolCorpusFindNode(n, "ICMP")
						require.NotNil(t, icmp)
						protocolCorpusRequireValue(t, icmp, "Type", uint64(8))
						dvmrpRequireSpan(t, icmp, "Type", uint64(transport)*8, uint64(transport+1)*8)
					}
				})
			}
		}
	}
}

func TestProtocolCorpusIPv4FragmentNeverDispatchesBody(t *testing.T) {
	for _, protocol := range []byte{6, 17, 1, 4, 41, 253} {
		for _, fragment := range []uint16{0x2000, 1, 0x2001} {
			t.Run(fmt.Sprintf("protocol%d/fragment%04x", protocol, fragment), func(t *testing.T) {
				body := ipv4OptionsBody(protocol)
				ip := ipv4OptionsDatagram(protocol, []byte{1, 1, 0, 0}, body)
				binary.BigEndian.PutUint16(ip[6:8], fragment)
				n := protocolCorpusRequireBoundedRuleParse(t, ip, "internet_protocol", "Internet Protocol")
				protocolCorpusRequireValue(t, n, "IP Fragment Data", body)
				dvmrpRequireSpan(t, n, "IP Fragment Data", 24*8, uint64(len(ip))*8)
				for _, name := range []string{"TCP", "UDP", "ICMP", "IPv4", "IPv6"} {
					require.Nil(t, protocolCorpusFindNode(n, name), "fragment body is not a complete child protocol")
				}
			})
		}
	}
}

func TestProtocolCorpusIPv4OptionsRejectInvalidBoundaries(t *testing.T) {
	for _, protocol := range []byte{6, 17, 1, 4, 41, 253} {
		for _, profile := range []string{"version", "short-IHL", "short-total", "truncated-options", "missing-option-length", "zero-option-length", "one-option-length", "option-overrun"} {
			t.Run(fmt.Sprintf("protocol%d/%s", protocol, profile), func(t *testing.T) {
				ip := ipv4OptionsDatagram(protocol, []byte{1, 1, 0, 0}, ipv4OptionsBody(protocol))
				switch profile {
				case "version":
					ip[0] = 0x66
				case "short-IHL":
					ip[0] = 0x44
				case "short-total":
					binary.BigEndian.PutUint16(ip[2:4], 23)
				case "truncated-options":
					ip = ip[:23]
				case "missing-option-length":
					copy(ip[20:24], []byte{1, 1, 1, 7})
				case "zero-option-length":
					copy(ip[20:24], []byte{7, 0, 0, 0})
				case "one-option-length":
					copy(ip[20:24], []byte{7, 1, 0, 0})
				case "option-overrun":
					copy(ip[20:24], []byte{7, 5, 0, 0})
				}
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(ip), "internet_protocol", "Internet Protocol")
				require.Error(t, err, "invalid enclosing headers cannot qualify a child")
			})
		}
	}
}
