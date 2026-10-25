package sharkcli

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestRegisteredFieldListingWithoutCapture(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, invoke(&out, "--list-fields", "--json"))
	var schema struct {
		SchemaVersion int                               `json:"schema_version"`
		Fields        []pcaputil.DisplayFieldDescriptor `json:"fields"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &schema))
	require.Equal(t, pcaputil.DisplayFieldSchemaVersion, schema.SchemaVersion)
	require.Equal(t, pcaputil.RegisteredDisplayFields(), schema.Fields)
	out.Reset()
	require.NoError(t, invoke(&out, "--list-fields"))
	require.Contains(t, out.String(), "wg.counter")
	require.Contains(t, out.String(), "uint64")
	require.Contains(t, out.String(), "context")
}

func TestDisplayFilterValidatedBeforeOpeningCaptureOrOutput(t *testing.T) {
	output := filepath.Join(t.TempDir(), "must-not-exist.pcap")
	for _, expr := range []string{`wg.counter contains "x"`, `dns.id == 65536`, `no.such.field`, `tls.record.authenticated > false`} {
		var out bytes.Buffer
		err := invoke(&out, "-r", "does-not-exist.pcap", "-w", output, "--display-filter", expr, "--json")
		require.ErrorContains(t, err, "invalid display filter")
		_, err = os.Stat(output)
		require.True(t, os.IsNotExist(err))
	}
}

func TestDisplayFilterExportsMatchingRowsAndKeepsEveryCapturedPacket(t *testing.T) {
	input := fixture(t, "display.pcap", dnsPacket(t), udpPacket(t, 9000, []byte("unrecognized")), dnsPacket(t))
	output := filepath.Join(t.TempDir(), "all.pcap")
	var out bytes.Buffer
	require.NoError(t, invoke(&out, "-r", input, "-w", output, "-Y", `dns.id == 0x1234`, "--json"))
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	require.Len(t, lines, 2)
	for i, line := range lines {
		var row struct {
			Number uint64 `json:"number"`
			PDUs   []struct {
				Fields map[string][]json.Number      `json:"-"`
				Values pcaputil.DisplayFieldSnapshot `json:"field_values"`
			} `json:"pdus"`
		}
		decoder := json.NewDecoder(strings.NewReader(line))
		decoder.UseNumber()
		require.NoError(t, decoder.Decode(&row))
		require.Equal(t, uint64(1+i*2), row.Number)
		require.NotEmpty(t, row.PDUs)
		found := false
		for _, pdu := range row.PDUs {
			require.Equal(t, 1, pdu.Values.SchemaVersion)
			for _, v := range pdu.Values.Values {
				if v.Name == "dns.id" {
					require.Equal(t, json.Number("4660"), v.Value)
					found = true
				}
			}
		}
		require.True(t, found)
	}
	saved, err := openOffline(output, "")
	require.NoError(t, err)
	defer saved.Close()
	count := 0
	for {
		_, _, err := saved.reader.ReadPacketData()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		count++
	}
	require.Equal(t, 3, count, "display predicates never discard saved capture records")
	out.Reset()
	require.NoError(t, invoke(&out, "-r", input, "--display-filter", `dns.id == 4660`, "--plain"))
	require.Equal(t, 2, len(strings.Split(strings.TrimSpace(out.String()), "\n")))
}

func TestFieldSearchUnitesPDUsWithinOnlyOnePacket(t *testing.T) {
	tls := &pcaputil.ProtocolEvent{Protocol: "tls", Session: map[string]any{"Negotiated Version": uint16(0x304), "Authentication Verified": true}}
	http := &pcaputil.ProtocolEvent{Protocol: "http", Raw: []byte("GET / HTTP/1.1\r\n\r\n")}
	u := &tui{ring: newPacketRing(8), width: 180, height: 48}
	for i, events := range [][]*pcaputil.ProtocolEvent{{tls}, {http}, {tls, http}} {
		u.ring.add(&capturedPacket{number: uint64(i + 1), data: udpPacket(t, 9000, []byte("test")), link: layers.LinkTypeEthernet, events: events})
	}
	require.NoError(t, u.applySearch(`fields tls.version == 0x304 and http.request.method == "GET"`))
	u.refreshVisible()
	require.Equal(t, []uint64{3}, visibleNumbers(u), "neighbouring packets cannot contribute facts to one match")
}

func TestFieldSearchRefreshesFrozenContributorsAndReportsEviction(t *testing.T) {
	history, err := pcaputil.NewProtocolInspector(1, 1<<20)
	require.NoError(t, err)
	streams := newStreamStore(8, 4096)
	streams.history = history
	u := &tui{ring: newPacketRing(8), streams: streams, width: 180, height: 48}
	p := &capturedPacket{number: 1, reference: pcaputil.PacketReference{Number: 1}, physicalRecord: true, history: history, data: udpPacket(t, 9000, []byte("test")), link: layers.LinkTypeEthernet}
	u.ring.add(p)
	u.frozen = []packetEntry{*u.ring.at(0)}
	u.frozenAt = u.ring.version
	require.NoError(t, u.applySearch("fields bgp.type == 1"))
	u.refreshVisible()
	require.Empty(t, u.visible)
	e := &pcaputil.ProtocolEvent{ID: 1, Protocol: "bgp", Status: "decoded", Session: map[string]any{"Type": uint64(1)}, SourceBytes: pcaputil.ByteSource{PacketRefs: []pcaputil.PacketReference{{Number: 1}}}}
	history.OnEvent(e)
	u.refreshVisible()
	require.Equal(t, []uint64{1}, visibleNumbers(u), "late completion must update a frozen packet list")
	p.eventIDs = []uint64{1}
	other := *e
	other.ID = 2
	other.SourceBytes.PacketRefs = []pcaputil.PacketReference{{Number: 2}}
	history.OnEvent(&other)
	u.refreshVisible()
	require.Empty(t, u.visible)
	require.Contains(t, u.notice, "Field filter partial: some packets unavailable")
	f, err := pcaputil.CompileDisplayFilter("bgp.type == 1")
	require.NoError(t, err)
	result := f.EvaluateEvents(p.protocolEvents())
	require.False(t, result.Complete)
	require.False(t, result.Matched)
}

func TestProtocolHistoryScalesWithRequestedPacketWindow(t *testing.T) {
	for _, c := range []struct{ packets, messages, bytes int }{{0, 80000, 64 << 20}, {20000, 80000, 64 << 20}, {60000, 240000, 192 << 20}, {100000, 256000, 256 << 20}, {int(^uint(0) >> 1), 256000, 256 << 20}} {
		messages, bytes := protocolHistoryBudget(c.packets)
		require.Equal(t, c.messages, messages)
		require.Equal(t, c.bytes, bytes)
	}
}

func TestWorkerRetainsExactUint64InProtocolDetail(t *testing.T) {
	w := make([]byte, 32)
	w[0] = 4
	e := &pcaputil.ProtocolEvent{ID: 1, Protocol: "wireguard", Status: "decoded", Raw: w, Session: map[string]any{"Packet Name": "Transport Data", "Counter": uint64(math.MaxUint64)}}
	request := decodeRequest{Number: 1, Events: []*pcaputil.ProtocolEvent{e}, Data: udpPacket(t, 51820, w), Link: layers.LinkTypeEthernet, Length: 74}
	payload, err := json.Marshal(request)
	require.NoError(t, err)
	var output bytes.Buffer
	require.NoError(t, runDecodeWorker(bytes.NewReader(payload), &output))
	require.Contains(t, output.String(), "Counter: 18446744073709551615")
	require.NotContains(t, output.String(), "Counter: 18446744073709552000")
}
