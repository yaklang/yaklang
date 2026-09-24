package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func openWireInfoFixture(version uint32, properties []byte) []byte {
	bodyLength := openWireInfoBodyHeader + len(properties)
	wire := make([]byte, 4+bodyLength)
	binary.BigEndian.PutUint32(wire[:4], uint32(bodyLength))
	wire[4] = openWireInfoType
	copy(wire[5:13], openWireMagic)
	binary.BigEndian.PutUint32(wire[13:17], version)
	wire[17] = 1
	binary.BigEndian.PutUint32(wire[18:22], uint32(len(properties)))
	copy(wire[22:], properties)
	return wire
}

func openWireOpaqueFixture(command byte, payload []byte) []byte {
	wire := make([]byte, 5+len(payload))
	binary.BigEndian.PutUint32(wire[:4], uint32(1+len(payload)))
	wire[4] = command
	copy(wire[5:], payload)
	return wire
}

func TestOpenWireProbeRequiresCompleteValidatedWireFormatInfo(t *testing.T) {
	valid := openWireInfoFixture(10, []byte{0x01, 0x02, 0x03})
	probe := probeOpenWire(valid, openWireInfoMaxBytes)
	require.Equal(t, ProbeAccept, probe.Verdict)
	require.Equal(t, "openwire", probe.Protocol)
	nullProperties := bytes.Clone(valid[:18])
	binary.BigEndian.PutUint32(nullProperties[:4], 14)
	nullProperties[17] = 0
	require.Equal(t, ProbeAccept, probeOpenWire(nullProperties, openWireInfoMaxBytes).Verdict)
	for n := 1; n < len(valid); n++ {
		probe := probeOpenWire(valid[:n], openWireInfoMaxBytes)
		require.NotEqualf(t, ProbeAccept, probe.Verdict, "prefix length %d", n)
	}
	truncated := probeOpenWire(valid[:len(valid)-1], openWireInfoMaxBytes)
	require.Equal(t, ProbeNeedMore, truncated.Verdict)
	require.Positive(t, truncated.NeedBytes)

	invalid := map[string][]byte{
		"non-WireFormatInfo first command": func() []byte { w := bytes.Clone(valid); w[4] = 2; return w }(),
		"wrong magic":                      func() []byte { w := bytes.Clone(valid); w[5] ^= 1; return w }(),
		"zero version":                     func() []byte { w := bytes.Clone(valid); clear(w[13:17]); return w }(),
		"invalid nullable marker":          func() []byte { w := bytes.Clone(valid); w[17] = 2; return w }(),
		"property length mismatch":         func() []byte { w := bytes.Clone(valid); w[21]++; return w }(),
		"property length over bound": func() []byte {
			w := bytes.Clone(valid)
			binary.BigEndian.PutUint32(w[18:22], openWireMaxPropertyLen+1)
			return w
		}(),
		"declared frame length mismatch": func() []byte { w := bytes.Clone(valid); w[3]++; return w }(),
	}
	for name, wire := range invalid {
		t.Run(name, func(t *testing.T) {
			require.NotEqual(t, ProbeAccept, probeOpenWire(wire, openWireInfoMaxBytes).Verdict)
		})
	}

	http, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	httpProbe := http.Probe([]byte("GET / HTTP/1.1\r\nHost: example.test\r\n\r\n"))
	require.NotEqual(t, "openwire", httpProbe.Protocol)
	require.Equal(t, "http", httpProbe.Protocol)

	wrongFirstFrame, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.NotEqual(t, "openwire", wrongFirstFrame.Probe(openWireOpaqueFixture(3, []byte{0, 1})).Protocol)
}

func TestOpenWireSessionRetainsExactFramingAcrossDirections(t *testing.T) {
	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	info := openWireInfoFixture(10, []byte{0xaa, 0xbb})
	opaque := openWireOpaqueFixture(3, []byte{0x55, 0x66})
	combined := append(bytes.Clone(info), opaque...)
	probe := session.Probe(combined)
	require.Equal(t, ProbeAccept, probe.Verdict)
	require.Equal(t, "openwire", probe.Protocol)

	first := session.Feed(0, time.Unix(1, 0), combined[:11])
	require.Empty(t, first.Events)
	require.True(t, first.NeedMore)
	second := session.Feed(0, time.Unix(1, 1), combined[11:])
	require.Nil(t, second.Err, "state=%s events=%+v", second.State, second.Events)
	require.Equal(t, "openwire", second.State)
	require.Len(t, second.Events, 2)
	require.Equal(t, info, second.Events[0].Raw)
	require.Equal(t, "ActiveMQOpenWire", second.Events[0].Entry)
	require.Equal(t, opaque, second.Events[1].Raw)
	require.Equal(t, "ActiveMQOpenWireOpaque", second.Events[1].Entry)
	for _, event := range second.Events {
		require.Equal(t, "decoded", event.Status)
		require.Equal(t, 0, event.Direction)
		require.Equal(t, 4+int(binary.BigEndian.Uint32(event.Raw[:4])), len(event.Raw))
		require.Equal(t, "openwire-length-prefixed", event.Profile)
	}

	peerInfo := openWireInfoFixture(12, []byte{0x10})
	peer := session.Feed(1, time.Unix(1, 2), peerInfo)
	require.Nil(t, peer.Err)
	require.Len(t, peer.Events, 1)
	require.Equal(t, 1, peer.Events[0].Direction)
	require.Equal(t, "ActiveMQOpenWire", peer.Events[0].Entry)
}

func TestOpenWireRejectsUnknownCommandAfterValidatedFirstFrame(t *testing.T) {
	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	first := session.Feed(0, time.Unix(2, 0), openWireInfoFixture(10, nil))
	require.Nil(t, first.Err)
	bad := session.Feed(0, time.Unix(2, 1), openWireOpaqueFixture(99, []byte{1}))
	require.Error(t, bad.Err)
	require.Len(t, bad.Events, 1)
	require.Equal(t, "malformed", bad.Events[0].Status)
	require.Contains(t, bad.Events[0].Summary, "unsupported command type 99")
	require.Contains(t, bad.Err.Error(), "unsupported command type 99")
}

func TestOpenWireIndependentCorpusReplayAndProvenance(t *testing.T) {
	const expectedSHA256 = "0846d67344609325ec6cb4afb55a30f17597420a79813798913542a33d388a76"
	wire := binCorpusBytes(t, "ndpi/ndpi-openwire.pcapng")
	var manifest struct {
		Captures []struct {
			ID          string `json:"id"`
			CaptureFile string `json:"capture_file"`
			SHA256      string `json:"sha256"`
			PacketCount int    `json:"packet_count"`
			LinkType    string `json:"link_type"`
		} `json:"captures"`
	}
	manifestBytes, err := os.ReadFile(filepath.Join("..", "..", "bin-parser", "testdata", "protocol-corpus", "manifest.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(manifestBytes, &manifest))
	var entry struct {
		ID          string
		CaptureFile string
		SHA256      string
		PacketCount int
		LinkType    string
	}
	for _, capture := range manifest.Captures {
		if capture.ID == "ndpi-openwire" {
			entry.ID, entry.CaptureFile, entry.SHA256, entry.PacketCount, entry.LinkType = capture.ID, capture.CaptureFile, capture.SHA256, capture.PacketCount, capture.LinkType
			break
		}
	}
	require.Equal(t, "ndpi-openwire", entry.ID)
	require.Equal(t, "captures/ndpi/ndpi-openwire.pcapng", entry.CaptureFile)
	require.Equal(t, expectedSHA256, entry.SHA256)
	require.Equal(t, 43, entry.PacketCount)
	require.Equal(t, "Null", entry.LinkType)
	digest := sha256.Sum256(wire)
	require.Equal(t, expectedSHA256, hex.EncodeToString(digest[:]))

	var sources struct {
		Captures []struct {
			ID           string `json:"id"`
			SourceSHA256 string `json:"source_sha256"`
		} `json:"captures"`
	}
	sourceBytes, err := os.ReadFile(filepath.Join("..", "..", "bin-parser", "testdata", "protocol-corpus", "sources.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(sourceBytes, &sources))
	var sourceSHA string
	for _, capture := range sources.Captures {
		if capture.ID == "ndpi-openwire" {
			sourceSHA = capture.SourceSHA256
			break
		}
	}
	require.Equal(t, expectedSHA256, sourceSHA)

	var events []*ProtocolEvent
	err = ReplayPcap(bytes.NewReader(wire), WithTCPReassemblyWorkers(1), WithBinParser(func(event *ProtocolEvent) {
		if event.Protocol == "openwire" {
			events = append(events, event)
		}
	}))
	require.NoError(t, err)
	require.Len(t, events, 18)
	commandCounts := make(map[byte]int)
	directions := make(map[int]int)
	for _, event := range events {
		require.Equal(t, "decoded", event.Status, "%s: %s", event.Summary, event.Error)
		require.GreaterOrEqual(t, len(event.Raw), 5)
		require.Equal(t, 4+int(binary.BigEndian.Uint32(event.Raw[:4])), len(event.Raw), "exact frame boundary")
		require.True(t, validOpenWireCommandType(event.Raw[4]))
		require.Equal(t, "openwire-length-prefixed", event.Profile)
		commandCounts[event.Raw[4]]++
		directions[event.Direction]++
		if event.Raw[4] == openWireInfoType {
			require.Equal(t, "ActiveMQOpenWire", event.Entry)
			require.Equal(t, "ActiveMQ", string(event.Raw[5:13]))
		} else {
			require.Equal(t, "ActiveMQOpenWireOpaque", event.Entry)
		}
	}
	require.Equal(t, 2, commandCounts[openWireInfoType])
	require.Equal(t, 16, len(events)-commandCounts[openWireInfoType])
	require.Contains(t, directions, 0)
	require.Contains(t, directions, 1)
}
