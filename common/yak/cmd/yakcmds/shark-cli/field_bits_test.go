package sharkcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func TestPackedFieldValueUsesBitWidthAndWireRange(t *testing.T) {
	for _, word := range []uint16{0x5010, 0x5012, 0x51d0, 0x5603, 0x5ffe, 0x50fe} {
		t.Run(fmt.Sprintf("%04x", word), func(t *testing.T) {
			wire := make([]byte, 20)
			wire[0], wire[1], wire[2], wire[3] = 0x30, 0x39, 0xd4, 0x31
			wire[12], wire[13] = byte(word>>8), byte(word)
			n, err := parser.ParseBinary(bytes.NewReader(wire), "transmission_control_protocol")
			require.NoError(t, err)
			v, err := n.Result()
			require.NoError(t, err)
			var fields []field
			flattenFields(v, 0, &fields)
			f := findField(t, packetDetail{fields: fields}, "Flags")
			require.Equal(t, fmt.Sprintf("Flags: 12 bits · HEX %03x · BITS %012b", word&0xfff, word&0xfff), f.text)
			require.True(t, f.hasRange)
			require.Zero(t, f.source)
			require.Equal(t, 12, f.start)
			require.Equal(t, 14, f.end)
			require.Equal(t, []byte{byte(word >> 8), byte(word)}, wire[f.start:f.end])
		})
	}
}

func TestPackedFieldPresentationAgainstIndependentBits(t *testing.T) {
	// A literal bit cursor supplies the expected value and the parser's raw
	// result representation (full octets, then a right-aligned partial octet).
	// Byte range checks stay independent of that logical representation.
	wire := bytes.Repeat([]byte{0xa5, 0x69, 0x37, 0xd2, 0x08}, 8)
	for offset := 0; offset < 8; offset++ {
		for _, width := range []int{1, 4, 7, 8, 9, 12, 15, 16, 17, 31, 65, 127, 129, 257} {
			t.Run(fmt.Sprintf("offset%d/width%d", offset, width), func(t *testing.T) {
				var bits strings.Builder
				packed := make([]byte, (width+7)/8)
				for i := 0; i < width; i++ {
					bit := wire[(offset+i)/8] >> (7 - (offset+i)%8) & 1
					bits.WriteByte('0' + bit)
					packed[i/8] = packed[i/8]<<1 | bit
				}
				cfg := base.NewEmptyConfig()
				cfg.SetItem(base.CfgType, "raw")
				cfg.SetItem(base.CfgNodeResult, [2]uint64{uint64(offset), uint64(offset + width)})
				v := &base.NodeValue{Name: "Bits", Value: packed, Origin: &base.Node{Cfg: cfg}}
				var fields []field
				flattenFields(v, 0, &fields)
				require.Len(t, fields, 1)
				f := fields[0]
				if offset%8 == 0 && width%8 == 0 {
					require.Equal(t, "Bits: "+formatFieldValue(v.Name, v.Value), f.text)
				} else if width <= 128 {
					padded := strings.Repeat("0", (4-width%4)%4) + bits.String()
					var hex strings.Builder
					for i := 0; i < len(padded); i += 4 {
						u, err := strconv.ParseUint(padded[i:i+4], 2, 8)
						require.NoError(t, err)
						hex.WriteString(strconv.FormatUint(u, 16))
					}
					require.Equal(t, fmt.Sprintf("Bits: %d bits · HEX %s · BITS %s", width, hex.String(), bits.String()), f.text)
				} else {
					require.Contains(t, f.text, fmt.Sprintf("Bits: %d bits · BITS %s", width, bits.String()[:128]))
					require.Contains(t, f.text, "select for full bytes")
					require.NotContains(t, f.text, "HEX")
					require.Less(t, len(f.text), 240)
				}
				require.True(t, f.hasRange)
				require.Equal(t, offset/8, f.start)
				require.Equal(t, (offset+width+7)/8, f.end)
				require.Equal(t, packed, v.Value, "presentation never rewrites the raw parser value")
			})
		}
	}
}

func TestTCPPackedFieldWorkerConsistency(t *testing.T) {
	for _, word := range []uint16{0x5010, 0x51d0, 0x5ff8} {
		for _, profile := range []string{"IPv4", "VLAN", "IPv6", "Loopback"} {
			t.Run(fmt.Sprintf("%04x/%s", word, profile), func(t *testing.T) {
				raw, at := tcpInvalidFlagPacket(byte(word), profile)
				// This test isolates packed field presentation in ordinary
				// headers. The shared failure fixture has IPv4 options; their
				// independent parser-offset regression is tracked separately.
				if profile != "IPv6" {
					ip := at - 24
					raw.data = append(raw.data[:at-4], raw.data[at:]...)
					raw.data[ip] = 0x45
					total := len(raw.data) - ip
					raw.data[ip+2], raw.data[ip+3] = byte(total>>8), byte(total)
					at -= 4
					raw.ci.CaptureLength, raw.ci.Length = len(raw.data), len(raw.data)
				}
				raw.data[at+12] = byte(word >> 8)
				if profile == "Loopback" {
					raw.data = append([]byte{2, 0, 0, 0}, raw.data[14:]...)
					raw.link, at = layers.LinkTypeNull, at-10
					raw.ci.CaptureLength, raw.ci.Length = len(raw.data), len(raw.data)
				}
				d := detail(raw)
				f := findField(t, d, "Flags")
				require.Equal(t, fmt.Sprintf("Flags: 12 bits · HEX %03x · BITS %012b", word&0xfff, word&0xfff), f.text)
				require.True(t, f.hasRange)
				require.Zero(t, f.source)
				require.Equal(t, [2]int{at + 12, at + 14}, [2]int{f.start, f.end})
				req, err := json.Marshal(decodeRequest{Number: raw.number, Data: raw.data, Length: raw.ci.Length, Link: raw.link})
				require.NoError(t, err)
				var output bytes.Buffer
				require.NoError(t, runDecodeWorker(bytes.NewReader(req), &output))
				worker, err := decodeWorkerOutput(output.Bytes(), raw)
				require.NoError(t, err)
				require.Equal(t, f, findField(t, worker, "Flags"))
			})
		}
	}
}

func TestPackedFieldPresentationDoesNotGuessMetadata(t *testing.T) {
	for _, profile := range []string{"no-origin", "no-config", "no-span", "wrong-span-type", "reverse-span", "out-of-bounds", "not-raw", "numeric", "byte-aligned", "wrong-size", "bad-padding"} {
		t.Run(profile, func(t *testing.T) {
			cfg := base.NewEmptyConfig()
			cfg.SetItem(base.CfgType, "raw")
			cfg.SetItem(base.CfgNodeResult, [2]uint64{4, 16})
			v := &base.NodeValue{Name: "Flags", Value: []byte{0x0f, 0x0e}, Origin: &base.Node{Cfg: cfg}}
			switch profile {
			case "no-origin":
				v.Origin = nil
			case "no-config":
				v.Origin.Cfg = nil
			case "no-span":
				cfg.DeleteItem(base.CfgNodeResult)
			case "wrong-span-type":
				cfg.SetItem(base.CfgNodeResult, "4,16")
			case "reverse-span":
				cfg.SetItem(base.CfgNodeResult, [2]uint64{16, 4})
			case "out-of-bounds":
				cfg.SetItem(base.CfgNodeResult, [2]uint64{262144*8 - 4, 262144*8 + 8})
			case "not-raw":
				cfg.SetItem(base.CfgType, "uint16")
			case "numeric":
				v.Value = uint64(254)
			case "byte-aligned":
				cfg.SetItem(base.CfgNodeResult, [2]uint64{0, 16})
			case "wrong-size":
				v.Value = []byte{0xfe}
			case "bad-padding":
				v.Value = []byte{0x0f, 0xfe}
			}
			require.Equal(t, formatFieldValue(v.Name, v.Value), formatNodeFieldValue(v))
		})
	}
}
