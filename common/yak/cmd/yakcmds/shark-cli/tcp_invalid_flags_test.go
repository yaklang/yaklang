package sharkcli

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func tcpInvalidFlagPacket(flags byte, profile string) (*capturedPacket, int) {
	body := tcpFailureBody(20, nil, []byte("GET /must-not-be-decoded HTTP/1.1\r\nHost: invalid.test\r\n\r\n"))
	body[13] = flags
	raw := tcpFailureRaw(body)
	at := 38
	switch profile {
	case "zero-IP-length":
		binary.BigEndian.PutUint16(raw.data[16:18], 0)
	case "VLAN":
		wire := append(bytes.Clone(raw.data[:12]), 0x81, 0, 0, 100)
		raw.data = append(wire, raw.data[12:]...)
		at += 4
	case "IPv6":
		wire := make([]byte, 14+40+len(body))
		copy(wire[:12], raw.data[:12])
		binary.BigEndian.PutUint16(wire[12:14], 0x86dd)
		wire[14], wire[20], wire[21] = 0x60, 6, 64
		binary.BigEndian.PutUint16(wire[18:20], uint16(len(body)))
		copy(wire[22:26], []byte{0x20, 1, 0x0d, 0xb8})
		wire[37], wire[53] = 1, 2
		copy(wire[38:42], []byte{0x20, 1, 0x0d, 0xb8})
		copy(wire[54:], body)
		raw.data, at = wire, 54
	}
	raw.ci.CaptureLength, raw.ci.Length = len(raw.data), len(raw.data)
	return raw, at
}

func TestTCPInvalidOpeningFlagsDiagnosticAcrossViews(t *testing.T) {
	for _, flags := range []byte{0x03, 0x06, 0x17} {
		for _, profile := range []string{"IPv4", "zero-IP-length", "VLAN", "IPv6"} {
			t.Run(fmt.Sprintf("flags%02x/%s", flags, profile), func(t *testing.T) {
				raw, at := tcpInvalidFlagPacket(flags, profile)
				require.Nil(t, tcpFailureDecoded(raw).ErrorLayer(), "complete syntactic headers do not trigger the dependency's error path")
				raw.application, raw.evidence = "TLS", "validated protocol events"
				raw.connectionStack = []string{"TLSv1.3", "HTTP/2", "gRPC"}
				raw.streamID, raw.conversationID, raw.tcpFlowID = 99, 98, 97
				row := summarize(raw)
				require.Equal(t, "TCP", row.Protocol)
				require.Equal(t, "malformed", row.ProtocolStatus)
				require.Contains(t, row.Info, "TCP SYN combined with FIN or RST")
				row = applyApplication(row, "gRPC", "prior connection", 99)
				require.Zero(t, row.StreamID)
				require.Zero(t, row.TCPFlowID)
				require.Zero(t, row.ConversationID)
				require.Empty(t, row.ConnectionStack)
				deep, quick := detail(raw), quickDetail(raw)
				require.Equal(t, deep.fields[1:], quick.fields[1:])
				require.Empty(t, deep.sources)
				requireTCPFailureRanges(t, raw, deep.fields)
				text := detailLines(deep.fields)
				require.Contains(t, text, "not admitted to reassembly")
				require.NotContains(t, text, "YAML dissector")
				require.NotContains(t, text, "HTTP FIELDS")
				var target []field
				for _, f := range deep.fields {
					if strings.HasPrefix(f.text, "TCP Flags (reserved bits excluded):") {
						target = append(target, f)
					}
				}
				require.Len(t, target, 1)
				require.True(t, target[0].hasRange)
				require.Equal(t, [2]int{at + 12, at + 14}, [2]int{target[0].start, target[0].end})
				request, err := json.Marshal(decodeRequest{Number: 1, Data: raw.data, Length: len(raw.data), Link: raw.link})
				require.NoError(t, err)
				var output bytes.Buffer
				require.NoError(t, runDecodeWorker(bytes.NewReader(request), &output))
				index := strings.Index(output.String(), decoderFrame)
				require.GreaterOrEqual(t, index, 0)
				var response decodeResponse
				require.NoError(t, json.Unmarshal(output.Bytes()[index+len(decoderFrame):], &response))
				require.Equal(t, "TCP", response.Protocol)
				require.Empty(t, response.Sources)
				require.Len(t, response.Fields, len(deep.fields))
				for i, f := range response.Fields {
					require.Equal(t, deep.fields[i].text, f.Text)
					require.Equal(t, deep.fields[i].hasRange, f.HasRange)
					require.Equal(t, [2]int{deep.fields[i].start, deep.fields[i].end}, [2]int{f.Start, f.End})
				}
			})
		}
	}
}

func TestTCPInvalidOpeningFlagsKeepOtherObservations(t *testing.T) {
	for _, flags := range []byte{0, 2, 0x10, 0x11, 0x14, 0x18, 0x12, 0xd2} {
		t.Run(fmt.Sprintf("allowed%02x", flags), func(t *testing.T) {
			raw, _ := tcpInvalidFlagPacket(flags, "IPv4")
			require.Nil(t, inspectTCPHeaderFailure(raw, tcpFailureDecoded(raw)))
		})
	}
	for _, name := range []string{"UDP", "different-physical-frame", "parent-refused"} {
		t.Run(name, func(t *testing.T) {
			raw, _ := tcpInvalidFlagPacket(3, "IPv4")
			if name == "UDP" {
				raw.data[23] = 17
			}
			p := tcpFailureDecoded(raw)
			if name == "different-physical-frame" {
				raw.data = bytes.Clone(raw.data)
				raw.data[len(raw.data)-1] ^= 1
			}
			if name == "parent-refused" {
				raw.presentation.networkRefused = true
			}
			require.Nil(t, inspectTCPHeaderFailure(raw, p))
		})
	}
}
