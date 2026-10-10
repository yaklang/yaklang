package yak

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/pcapx/pcapdb"
	"github.com/yaklang/yaklang/common/schema"
)

func preparePCAPDBYak(t *testing.T) {
	t.Helper()
	t.Setenv("YAKIT_HOME", t.TempDir())
	profile, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "profile.sqlite"))
	require.NoError(t, err)
	previous := schema.GetGormProfileDatabase()
	previousPath := ""
	if previous != nil {
		previousPath = consts.GetCurrentProfileDatabasePath()
	}
	consts.BindProfileDatabase(profile, "")
	t.Cleanup(func() {
		require.NoError(t, pcapdb.ClosePCAPDatabases())
		consts.BindProfileDatabase(previous, previousPath)
		profile.Close()
	})
}

func TestPCAPDBYakLifecycle(t *testing.T) {
	preparePCAPDBYak(t)
	var capture bytes.Buffer
	writer := pcapgo.NewWriterNanos(&capture)
	require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
	packet := make([]byte, 60)
	require.NoError(t, writer.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1700000000, 123456789), CaptureLength: len(packet), Length: len(packet)}, packet))
	input, output := filepath.Join(t.TempDir(), "input.pcap"), filepath.Join(t.TempDir(), "export.pcap")
	require.NoError(t, os.WriteFile(input, capture.Bytes(), 0600))
	_, err := NewScriptEngine(1).ExecuteWithoutCacheWithContext(context.Background(), `
capturePath = getParam("capturePath")
exportPath = getParam("exportPath")
db = pcapdb.GetOrCreatePCAPDatabase(capturePath)~
same = pcapdb.GetOrCreatePCAPDatabase(capturePath)~
assert db.ID == same.ID
entries = pcapdb.ListPCAPDatabases()~
assert len(entries) == 1
count = pcapdb.CountPCAPDatabases()~
assert count == 1
valid = pcapdb.ValidatePCAPDatabase(db.ID, true)~
assert valid.Valid
opened = pcapdb.OpenPCAPDatabase(db.ID)~
packets = opened.QueryPackets(pcapdb.limit(1))~
assert len(packets) == 1
raw = opened.ReadPacket(packets[0].ID)~
assert len(raw) == 60
exported = pcapdb.ExportFromPCAPDatabase(capturePath, exportPath)~
assert exported == exportPath
assert db.Close() == nil
`, map[string]any{"capturePath": input, "exportPath": output})
	require.NoError(t, err)
	data, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, capture.Bytes(), data)

	// A stopped Yak execution must cancel native import even if a caller
	// supplies its own background context through an import option.
	capture.Reset()
	writer = pcapgo.NewWriterNanos(&capture)
	require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
	for i := 0; i < 5; i++ {
		require.NoError(t, writer.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1700000000+int64(i), 0), CaptureLength: len(packet), Length: len(packet)}, packet))
	}
	require.NoError(t, os.WriteFile(input, capture.Bytes(), 0600))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err = NewScriptEngine(1).ExecuteWithoutCacheWithContext(ctx, `
cancelCapture = getParam("cancelCapture")
db = pcapdb.GetOrCreatePCAPDatabase(getParam("capturePath"),
    pcapdb.withContext(getParam("callerContext")), pcapdb.withBatchSize(1),
    pcapdb.onProgress(func(update) { if update.State == "indexing" && update.PacketCount > 0 { cancelCapture() } }))~
`, map[string]any{"capturePath": input, "cancelCapture": cancel, "callerContext": context.Background()})
	require.Error(t, err)
	entries, err := pcapdb.ListPCAPDatabases()
	require.NoError(t, err)
	var interrupted bool
	for _, entry := range entries {
		if entry.State == pcapdb.StateInterrupted {
			interrupted = true
		}
	}
	require.True(t, interrupted)
}

func TestPCAPDBYakProtocolFieldSearch(t *testing.T) {
	preparePCAPDBYak(t)
	ethernet := &layers.Ethernet{SrcMAC: net.HardwareAddr{1, 2, 3, 4, 5, 6}, DstMAC: net.HardwareAddr{6, 5, 4, 3, 2, 1}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, SrcIP: net.IP{10, 1, 2, 3}, DstIP: net.IP{8, 8, 8, 8}, Protocol: layers.IPProtocolUDP}
	udp := &layers.UDP{SrcPort: 30000, DstPort: 53}
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
	dns := &layers.DNS{ID: 123, RD: true, Questions: []layers.DNSQuestion{{Name: []byte("pcapdb.example"), Type: layers.DNSTypeA, Class: layers.DNSClassIN}}}
	packet := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(packet, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, ethernet, ip, udp, dns))
	var capture bytes.Buffer
	writer := pcapgo.NewWriterNanos(&capture)
	require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
	require.NoError(t, writer.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1700000000, 0), CaptureLength: len(packet.Bytes()), Length: len(packet.Bytes())}, packet.Bytes()))
	input := filepath.Join(t.TempDir(), "dns.pcap")
	require.NoError(t, os.WriteFile(input, capture.Bytes(), 0600))
	db, err := pcapdb.GetOrCreatePCAPDatabase(input, pcapdb.WithProtocols(true))
	require.NoError(t, err)
	messages, err := db.QueryProtocols(pcapdb.QueryProtocol("dns"), pcapdb.QueryWithFields(true))
	require.NoError(t, err)
	require.Len(t, messages, 1)
	// Discover a real parser path rather than relying on a guessed DNS schema.
	var findNumber func(any, string, []any) (string, json.Number, []any)
	findNumber = func(value any, path string, keys []any) (string, json.Number, []any) {
		switch value := value.(type) {
		case json.Number:
			if _, err := value.Int64(); err == nil {
				return path, value, keys
			}
		case map[string]any:
			for key, child := range value {
				if found, number, foundKeys := findNumber(child, path+"."+strconv.Quote(key), append(keys, key)); found != "" {
					return found, number, foundKeys
				}
			}
		case []any:
			for i, child := range value {
				if found, number, foundKeys := findNumber(child, fmt.Sprintf("%s[%d]", path, i), append(keys, i)); found != "" {
					return found, number, foundKeys
				}
			}
		}
		return "", "", nil
	}
	path, number, keys := findNumber(messages[0].Fields, "$", nil)
	require.NotEmpty(t, path)
	_, err = NewScriptEngine(1).ExecuteWithoutCacheWithContext(context.Background(), `
path = getParam("fieldPath")
db = pcapdb.GetOrCreatePCAPDatabase(getParam("capturePath"),
    pcapdb.withProtocols(true), pcapdb.withFieldIndex(path))~
rows = db.QueryProtocols(pcapdb.protocol("dns"),
    pcapdb.field(path, getParam("number")), pcapdb.fieldExists(path),
    pcapdb.fieldMissing("$.__absent_test_field__"), pcapdb.withFields(true))~
assert len(rows) == 1
detail = db.ProtocolDetails(rows[0].ID)~
value = detail.Fields
keys = getParam("keys")
for i = 0; i < len(keys); i++ { value = value[keys[i]] }
matched = db.QueryProtocols(pcapdb.protocol("dns"), pcapdb.field(path, value))~
assert len(matched) == 1
nullRows = db.QueryProtocols(pcapdb.field("$.__absent_test_field__", nil))~
assert len(nullRows) == 0
`, map[string]any{"capturePath": input, "fieldPath": path, "number": number, "keys": keys})
	require.NoError(t, err)
}
