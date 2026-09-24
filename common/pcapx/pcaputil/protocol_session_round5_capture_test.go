package pcaputil

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"os"
	"strings"
	"testing"
)

// Counts describe framed messages, not capture records or semantic completeness.
// Original corruption/gaps remain explicit failures in the unchanged captures.
func TestRound5OriginalCaptureReplay(t *testing.T) {
	corpus := "../../bin-parser/testdata/protocol-corpus/captures/"
	upstream := "testdata/protocol-sessions/upstream/"
	for _, tc := range []struct {
		path      string
		protocols []string
		success   map[string]int
		contexts  map[string]int
		errors    map[string]int
		gap       bool
	}{
		{path: corpus + "ndpi/ndpi-stun.pcap", protocols: []string{"stun", "turn"}, success: map[string]int{"stun": 56, "turn": 103}, contexts: map[string]int{"turn": 1}, errors: map[string]int{"stun/malformed": 1}},
		{path: upstream + "stun_signal_tcp.pcapng", protocols: []string{"stun", "turn"}, success: map[string]int{"turn": 291}},
		{path: upstream + "stun_tcp_multiple_msgs_same_pkt.pcap", protocols: []string{"stun", "turn"}, success: map[string]int{"turn": 6}},
		{path: corpus + "ndpi/ndpi-tftp.pcap", protocols: []string{"tftp"}, success: map[string]int{"tftp": 107}},
		{path: corpus + "ndpi/ndpi-rtsp-http.pcapng", protocols: []string{"rtsp"}, success: map[string]int{"rtsp": 1}},
		{path: upstream + "rtsp.pcap", protocols: []string{"rtsp"}, success: map[string]int{"rtsp": 65}},
		{path: corpus + "ndpi/ndpi-ipp.pcap", protocols: []string{"ipp"}, success: map[string]int{"ipp": 8}},
		{path: corpus + "ndpi/ndpi-diameter.pcap", protocols: []string{"diameter"}, success: map[string]int{"diameter": 6}},
		{path: corpus + "ndpi/ndpi-s7comm.pcap", protocols: []string{"s7comm"}, success: map[string]int{"s7comm": 170}},
		{path: corpus + "ndpi/ndpi-iec104.pcap", protocols: []string{"iec104"}, success: map[string]int{"iec104": 8}},
		{path: corpus + "ndpi/ndpi-opcua.pcap", protocols: []string{"opcua"}, success: map[string]int{"opcua": 187}},
		{path: upstream + "vnc-sample.pcap", protocols: []string{"vnc"}, success: map[string]int{"vnc": 70}, errors: map[string]int{"vnc/incomplete": 1}, gap: true},
	} {
		for _, workers := range []int{1, 2} {
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%d/%v", tc.path, workers, deferred), func(t *testing.T) {
					raw, err := os.ReadFile(tc.path)
					require.NoError(t, err)
					events, stats, err := binReplay(t, raw, workers, func(c *CaptureConfig) error { c.binParserConfig.Deferred = deferred; return nil })
					if tc.gap {
						require.ErrorContains(t, err, "unfilled sequence gap")
					} else {
						require.NoError(t, err)
					}
					counts, contexts, warnings := map[string]int{}, map[string]int{}, map[string]int{}
					foundField := false
					for _, e := range events {
						selected := false
						for _, p := range tc.protocols {
							if e.Protocol == p {
								selected = true
							}
						}
						if !selected {
							continue
						}
						if e.Status == "context-required" {
							contexts[e.Protocol]++
							require.Contains(t, e.Error, string(ErrContextRequired))
							continue
						}
						if e.Error != "" || e.Status != "decoded" && e.Status != "deferred" {
							warnings[e.Protocol+"/"+e.Status]++
							if e.Protocol == "stun" {
								require.Contains(t, e.Error, "fingerprint mismatch")
							}
							continue
						}
						counts[e.Protocol]++
						result, err := e.Decode()
						require.NoError(t, err)
						require.NotEmpty(t, result)
						require.NotNil(t, e.Session)
						switch e.Protocol {
						case "stun", "turn":
							if e.Session["Transaction ID"] != "" && e.Session["Transaction ID"] != nil {
								foundField = true
							}
						case "tftp":
							if e.Session["Transfer Complete"] == true {
								foundField = true
							}
						case "rtsp":
							if (e.Session["In Reply To"] == "DESCRIBE" && e.Session["SDP"] != nil) || (strings.Contains(tc.path, "ndpi-rtsp-http") && e.Session["Packet Name"] == "SETUP") {
								foundField = true
							}
						case "ipp":
							if e.Session["Matched"] == true && e.Session["Attributes"] != nil {
								foundField = true
							}
						case "diameter":
							require.Equal(t, uint32(272), e.Session["Command Code"])
							require.Equal(t, uint32(4), e.Session["Application ID"])
							if e.Session["Matched"] == true {
								foundField = true
							}
						case "s7comm":
							if e.Session["Function"] == byte(4) && e.Session["Data Items"] != nil {
								foundField = true
							}
						case "iec104":
							if asdu, ok := e.Session["ASDU"].(map[string]any); ok {
								require.Equal(t, byte(36), asdu["Type ID"])
								require.NotEmpty(t, asdu["Objects"])
								foundField = true
							}
						case "opcua":
							if e.Session["Message Type"] == "HEL" {
								require.Contains(t, e.Session["Endpoint URL"], "opc.tcp://")
								foundField = true
							}
						case "vnc":
							if e.Session["Phase"] == "server-init" {
								require.Equal(t, "test", e.Session["Desktop Name"])
								require.Equal(t, uint16(1024), e.Session["Width"])
								require.Equal(t, uint16(768), e.Session["Height"])
								foundField = true
							}
						}
					}
					require.Equal(t, tc.success, counts)
					expectedContexts := tc.contexts
					if expectedContexts == nil {
						expectedContexts = map[string]int{}
					}
					require.Equal(t, expectedContexts, contexts)
					expected := tc.errors
					if expected == nil {
						expected = map[string]int{}
					}
					require.Equal(t, expected, warnings)
					require.True(t, foundField, "capture must prove protocol content")
					require.Zero(t, stats.BufferedBytes)
				})
			}
		}
	}
}
func TestRound5UnsupportedUpstreamSecurityProfiles(t *testing.T) {
	for _, tc := range []struct{ file, protocol string }{{"ndpi/ndpi-vnc.pcap", "vnc"}, {"wireshark-tests/ws-opcua-signed.pcapng", "opcua"}} {
		t.Run(tc.protocol, func(t *testing.T) {
			events, stats, err := binReplay(t, binCorpusBytes(t, tc.file), 1)
			if tc.protocol == "vnc" {
				require.ErrorContains(t, err, "unfilled sequence gap")
			} else {
				require.NoError(t, err)
			}
			found := false
			for _, e := range events {
				t.Logf("%s %s %s", e.Protocol, e.Status, e.Error)
				if e.Protocol != tc.protocol {
					continue
				}
				if tc.protocol == "vnc" && strings.Contains(e.Error+e.Summary, "standard 3.3/3.7/3.8") {
					found = true
				}
				if tc.protocol == "opcua" && e.Session["Semantic Status"] == "security-context-required" {
					require.Equal(t, false, e.Session["Content Decoded"])
					found = true
				}
			}
			require.True(t, found)
			require.Zero(t, stats.BufferedBytes)
		})
	}
}
