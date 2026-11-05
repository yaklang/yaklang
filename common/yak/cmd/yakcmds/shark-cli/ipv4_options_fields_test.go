package sharkcli

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestIPv4OptionsPhysicalFieldsSurviveWorker(t *testing.T) {
	for _, options := range [][]byte{{0, 7, 5, 9}, {1, 1, 0, 0}, {148, 4, 0, 0}, bytes.Repeat([]byte{1}, 40)} {
		for _, profile := range []string{"Ethernet", "VLAN", "Loopback"} {
			t.Run(fmt.Sprintf("%x/%s", options, profile), func(t *testing.T) {
				raw := tcpFailureRaw(tcpFailureBody(20, nil, nil))
				raw.data = append(append(bytes.Clone(raw.data[:34]), options...), raw.data[38:]...)
				raw.data[14] = 0x40 | byte((20+len(options))/4)
				binary.BigEndian.PutUint16(raw.data[16:18], uint16(len(raw.data)-14))
				ip := 14
				if profile == "VLAN" {
					raw.data = append(append(bytes.Clone(raw.data[:12]), 0x81, 0, 0, 100), raw.data[12:]...)
					ip += 4
				} else if profile == "Loopback" {
					raw.data = append([]byte{2, 0, 0, 0}, raw.data[14:]...)
					raw.link, ip = layers.LinkTypeNull, 4
				}
				raw.ci.CaptureLength, raw.ci.Length = len(raw.data), len(raw.data)
				d := detail(raw)
				at := ip + 20 + len(options)
				port := findField(t, d, "Source Port")
				require.Equal(t, "Source Port: 12345", port.text)
				require.Equal(t, [2]int{at, at + 2}, [2]int{port.start, port.end})
				flags := findField(t, d, "Flags")
				require.Equal(t, "Flags: 12 bits · HEX 018 · BITS 000000011000", flags.text)
				require.Equal(t, [2]int{at + 12, at + 14}, [2]int{flags.start, flags.end})
				opt := findField(t, d, "IP Header Options")
				require.True(t, opt.hasRange)
				require.Zero(t, opt.source)
				require.Equal(t, [2]int{ip + 20, at}, [2]int{opt.start, opt.end})
				require.Equal(t, options, raw.data[opt.start:opt.end])
				request, err := json.Marshal(decodeRequest{Number: raw.number, Data: raw.data, Length: raw.ci.Length, Link: raw.link})
				require.NoError(t, err)
				var output bytes.Buffer
				require.NoError(t, runDecodeWorker(bytes.NewReader(request), &output))
				worker, err := decodeWorkerOutput(output.Bytes(), raw)
				require.NoError(t, err)
				require.Equal(t, d.fields, worker.fields)
			})
		}
	}
}
