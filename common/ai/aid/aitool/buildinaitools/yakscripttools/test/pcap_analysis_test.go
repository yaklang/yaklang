package test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/pcapx/pcapdb"
	"github.com/yaklang/yaklang/common/schema"
)

// Keep the oracle independent of the parser: one TCP conversation, a request
// split across two physical packets, and one response with a two-byte body.
func pcapAnalysisCapture(t *testing.T, host string) []byte {
	t.Helper()
	var capture bytes.Buffer
	w := pcapgo.NewWriterNanos(&capture)
	require.NoError(t, w.WriteFileHeader(65535, layers.LinkTypeEthernet))
	request := []byte("GET /pcap-skill-proof HTTP/1.1\r\nHost: " + host + "\r\n\r\n")
	response := []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK")
	frames := []struct {
		reverse  bool
		seq, ack uint32
		syn, fin bool
		payload  []byte
	}{
		{false, 100, 0, true, false, nil},
		{true, 500, 101, true, false, nil},
		{false, 101, 501, false, false, nil},
		{false, 101, 501, false, false, request[:18]},
		{false, 119, 501, false, false, request[18:]},
		{true, 501, uint32(101 + len(request)), false, false, response},
		{false, uint32(101 + len(request)), uint32(501 + len(response)), false, true, nil},
		{true, uint32(501 + len(response)), uint32(102 + len(request)), false, true, nil},
	}
	for i, frame := range frames {
		src, dst := net.IP{10, 23, 0, 10}, net.IP{198, 51, 100, 20}
		sp, dp := layers.TCPPort(43210), layers.TCPPort(80)
		if frame.reverse {
			src, dst, sp, dp = dst, src, dp, sp
		}
		eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{1, 2, 3, 4, 5, 6}, DstMAC: net.HardwareAddr{6, 5, 4, 3, 2, 1}, EthernetType: layers.EthernetTypeIPv4}
		ip := &layers.IPv4{Version: 4, TTL: 64, SrcIP: src, DstIP: dst, Protocol: layers.IPProtocolTCP}
		tcp := &layers.TCP{SrcPort: sp, DstPort: dp, Seq: frame.seq, Ack: frame.ack, SYN: frame.syn, FIN: frame.fin, ACK: frame.ack > 0, PSH: len(frame.payload) > 0, Window: 65535}
		require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
		packet := gopacket.NewSerializeBuffer()
		require.NoError(t, gopacket.SerializeLayers(packet, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, tcp, gopacket.Payload(frame.payload)))
		require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1700000000, 123456789+int64(i)), CaptureLength: len(packet.Bytes()), Length: len(packet.Bytes())}, packet.Bytes()))
	}
	return capture.Bytes()
}

func pcapAnalysisLeaves(value any, path string, leaves map[string]any) {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			pcapAnalysisLeaves(child, path+"."+strconv.Quote(key), leaves)
		}
	case []any:
		for i, child := range value {
			pcapAnalysisLeaves(child, path+"["+strconv.Itoa(i)+"]", leaves)
		}
	default:
		leaves[path] = value
	}
}

func TestPCAPAnalysisTools(t *testing.T) {
	previous := schema.GetGormProfileDatabase()
	previousPath := ""
	if previous != nil {
		previousPath = consts.GetCurrentProfileDatabasePath()
	}
	var previousProject consts.ProjectDatabaseBinding
	if schema.GetGormProjectDatabase() != nil {
		previousProject = consts.CaptureProjectDatabaseBinding()
	}
	home := filepath.Join(t.TempDir(), "流量分析 space #100% &")
	require.NoError(t, os.MkdirAll(home, 0700))
	t.Setenv("YAKIT_HOME", home)
	consts.ResetYakitHomeOnce()
	profile, err := gorm.Open("sqlite3", filepath.Join(home, "profile.sqlite"))
	require.NoError(t, err)
	require.NoError(t, pcapdb.ClosePCAPDatabases())
	consts.BindProfileDatabase(profile, filepath.Join(home, "profile.sqlite"))
	// Bind both globals explicitly: lazy project initialization would otherwise
	// open default-yakit.db inside the fixture and leak its handle on Windows.
	consts.BindProjectDatabaseWithReader(profile, nil, filepath.Join(home, "profile.sqlite"))
	t.Cleanup(func() {
		require.NoError(t, pcapdb.ClosePCAPDatabases())
		consts.BindProfileDatabase(previous, previousPath)
		consts.BindProjectDatabaseWithReader(previousProject.Database, previousProject.ReadDatabase, previousProject.Path)
		profile.Close()
		consts.ResetYakitHomeOnce()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	tools := map[string]*aitool.Tool{}
	for _, name := range []string{"load_pcapfile", "query_pcapdb", "manage_pcapdb"} {
		source, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/pcap/" + name + ".yak")
		require.NoError(t, err)
		metadata := yakscripttools.LoadYakScriptToAiTools(name, string(source))
		require.NotNil(t, metadata)
		converted := yakscripttools.ConvertTools([]*schema.AIYakTool{metadata})
		require.Len(t, converted, 1)
		tools[name] = converted[0]
	}
	for _, params := range []map[string]any{{"id": "dataset-id"}, {"pcapfile": "capture.pcap"}} {
		valid, errs := tools["query_pcapdb"].ValidateParams(params)
		require.True(t, valid, "%v", errs)
	}
	for _, params := range []map[string]any{{}, {"id": "dataset-id", "pcapfile": "capture.pcap"}} {
		valid, _ := tools["query_pcapdb"].ValidateParams(params)
		require.False(t, valid, "identity must be exclusive in the AI tool schema too")
	}
	for _, params := range []map[string]any{{"op": "list"}, {"op": "count"}, {"op": "delete", "id": "dataset-id"}, {"op": "validate", "id": "dataset-id"}, {"op": "export", "id": "dataset-id", "output": "export.pcap"}} {
		valid, errs := tools["manage_pcapdb"].ValidateParams(params)
		require.True(t, valid, "%v", errs)
	}
	for _, params := range []map[string]any{{"op": "unknown"}, {"op": "delete"}, {"op": "export", "id": "dataset-id"}} {
		valid, _ := tools["manage_pcapdb"].ValidateParams(params)
		require.False(t, valid)
	}
	execute := func(name string, params map[string]any) (*aitool.ToolExecutionResult, error) {
		t.Helper()
		return tools[name].ExecuteToolWithCapture(ctx, params, aitool.NewToolInvokeConfig())
	}
	invoke := func(name string, params map[string]any) any {
		t.Helper()
		out, err := execute(name, params)
		require.NoError(t, err)
		require.NotNil(t, out.Result, "tools must return semantic RESULT")
		return out.Result
	}
	object := func(value any) map[string]any {
		t.Helper()
		data, err := json.Marshal(value)
		require.NoError(t, err)
		var result map[string]any
		require.NoError(t, json.Unmarshal(data, &result))
		return result
	}
	capture := pcapAnalysisCapture(t, "skill.example")
	// The live-model harness consumes the exact same bytes and known answers.
	liveFixture, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "aismoking", "live", "testdata", "pcap_analysis.pcap"))
	require.NoError(t, err)
	require.Equal(t, capture, liveFixture)
	path := filepath.Join(home, "proof.pcap")
	require.NoError(t, os.WriteFile(path, capture, 0600))
	loaded := object(invoke("load_pcapfile", map[string]any{"pcapfile": path}))
	t.Logf("load result: %v", loaded)
	id := loaded["dataset_id"].(string)
	require.NotEmpty(t, id)
	require.Equal(t, "ready", loaded["state"])
	require.EqualValues(t, 8, loaded["packet_count"])
	require.EqualValues(t, 2, loaded["protocol_count"])
	require.EqualValues(t, 1, loaded["session_count"])
	require.EqualValues(t, 2, loaded["stream_count"])
	require.Equal(t, true, loaded["protocols_indexed"])
	require.Equal(t, true, loaded["streams_indexed"])
	query := func(mode string, extra map[string]any) any {
		t.Helper()
		params := map[string]any{"id": id, "query_type": mode}
		for k, v := range extra {
			params[k] = v
		}
		return invoke("query_pcapdb", params)
	}
	t.Run("dedup_and_path_selection", func(t *testing.T) {
		copyPath := filepath.Join(home, "same-content.pcap")
		require.NoError(t, os.WriteFile(copyPath, capture, 0600))
		same := object(invoke("load_pcapfile", map[string]any{"pcapfile": copyPath}))
		require.Equal(t, id, same["dataset_id"])
		meta := object(invoke("query_pcapdb", map[string]any{"pcapfile": copyPath, "query_type": "metadata"}))
		require.Equal(t, id, meta["dataset_id"])
		forwardSlash := object(invoke("load_pcapfile", map[string]any{"pcapfile": filepath.ToSlash(copyPath)}))
		require.Equal(t, id, forwardSlash["dataset_id"])
		// Keep this input in the process working directory so its name is truly
		// relative even when the repository and TempDir are on different drives.
		relative, err := os.CreateTemp(".", "pcapdb-relative-*.pcap")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.Remove(relative.Name()) })
		_, err = relative.Write(capture)
		require.NoError(t, err)
		require.NoError(t, relative.Close())
		require.False(t, filepath.IsAbs(relative.Name()))
		relativeMeta := object(invoke("query_pcapdb", map[string]any{"pcapfile": relative.Name(), "query_type": "metadata"}))
		require.Equal(t, id, relativeMeta["dataset_id"])
		count, err := pcapdb.CountPCAPDatabases()
		require.NoError(t, err)
		require.EqualValues(t, 1, count)
	})
	var messageID uint
	t.Run("protocol_pagination_and_fields", func(t *testing.T) {
		page := query("protocols", map[string]any{"limit": 1, "protocol": "http"}).(*pcapdb.ResultPage[pcapdb.ProtocolSummary])
		require.Nil(t, page.Error)
		require.Len(t, page.Items, 1)
		require.True(t, page.HasMore)
		messageID = page.Items[0].ID
		next := query("protocols", map[string]any{"limit": 1, "protocol": "http", "after": page.NextCursor}).(*pcapdb.ResultPage[pcapdb.ProtocolSummary])
		require.Nil(t, next.Error)
		require.Len(t, next.Items, 1)
		require.False(t, next.HasMore)
		require.Greater(t, next.Items[0].ID, messageID)
		fields := query("fields", map[string]any{"protocol": "http", "result_bytes": 65536}).(*pcapdb.ResultPage[pcapdb.ProtocolField])
		require.Nil(t, fields.Error)
		require.True(t, fields.Sampled)
		require.NotEmpty(t, fields.Items)
		detailPage := query("details", map[string]any{"record_id": messageID, "result_bytes": 65536}).(*pcapdb.ResultPage[pcapdb.ProtocolDetail])
		require.Nil(t, detailPage.Error)
		require.Len(t, detailPage.Items, 1)
		detail := object(detailPage)
		require.Nil(t, detail["error"])
		encoded, err := json.Marshal(detail)
		require.NoError(t, err)
		require.Contains(t, string(encoded), "/pcap-skill-proof")
		require.Contains(t, string(encoded), "skill.example")
		// A discovered path is usable, while missing is different from JSON null.
		var path string
		for _, field := range fields.Items {
			if field.Searchable {
				path = field.Path
				break
			}
		}
		require.NotEmpty(t, path)
		exists := query("protocols", map[string]any{"field_path": path, "field_op": "exists"}).(*pcapdb.ResultPage[pcapdb.ProtocolSummary])
		require.Nil(t, exists.Error)
		require.NotEmpty(t, exists.Items)
		// Discover and verify the actual typed parser leaf before querying it.
		leaves := map[string]any{}
		pcapAnalysisLeaves(detailPage.Items[0].Fields, "$", leaves)
		matchedString := false
		for path, value := range leaves {
			text, ok := value.(string)
			if !ok || text != "/pcap-skill-proof" {
				continue
			}
			page := query("protocols", map[string]any{"field_path": path, "field_type": "string", "field_value": text}).(*pcapdb.ResultPage[pcapdb.ProtocolSummary])
			require.Nil(t, page.Error)
			var found bool
			for _, row := range page.Items {
				found = found || row.ID == messageID
			}
			require.True(t, found, "verified field %s=%s", path, text)
			matchedString = true
		}
		require.True(t, matchedString)
		for _, op := range []string{"missing", "eq"} {
			params := map[string]any{"field_path": "$.__absent_skill_field__", "field_op": op}
			if op == "eq" {
				params["field_type"] = "null"
			}
			p := query("protocols", params).(*pcapdb.ResultPage[pcapdb.ProtocolSummary])
			require.Nil(t, p.Error)
			if op == "missing" {
				require.Len(t, p.Items, 2)
			} else {
				require.Empty(t, p.Items)
			}
		}
	})
	t.Run("typed_numeric_protocol_field", func(t *testing.T) {
		eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{1, 2, 3, 4, 5, 6}, DstMAC: net.HardwareAddr{6, 5, 4, 3, 2, 1}, EthernetType: layers.EthernetTypeIPv4}
		ip := &layers.IPv4{Version: 4, TTL: 64, SrcIP: net.IP{10, 23, 0, 10}, DstIP: net.IP{198, 51, 100, 20}, Protocol: layers.IPProtocolUDP}
		udp := &layers.UDP{SrcPort: 43210, DstPort: 53}
		require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
		dns := &layers.DNS{ID: 531, RD: true, Questions: []layers.DNSQuestion{{Name: []byte("skill.example"), Type: layers.DNSTypeA, Class: layers.DNSClassIN}}}
		packet := gopacket.NewSerializeBuffer()
		require.NoError(t, gopacket.SerializeLayers(packet, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, udp, dns))
		var capture bytes.Buffer
		w := pcapgo.NewWriterNanos(&capture)
		require.NoError(t, w.WriteFileHeader(65535, layers.LinkTypeEthernet))
		require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1700000000, 0), CaptureLength: len(packet.Bytes()), Length: len(packet.Bytes())}, packet.Bytes()))
		path := filepath.Join(home, "numeric-dns.pcap")
		require.NoError(t, os.WriteFile(path, capture.Bytes(), 0600))
		page := invoke("query_pcapdb", map[string]any{"pcapfile": path, "protocol": "dns"}).(*pcapdb.ResultPage[pcapdb.ProtocolSummary])
		require.Nil(t, page.Error)
		require.Len(t, page.Items, 1)
		detail := invoke("query_pcapdb", map[string]any{"id": page.DatasetID, "query_type": "details", "record_id": page.Items[0].ID}).(*pcapdb.ResultPage[pcapdb.ProtocolDetail])
		require.Nil(t, detail.Error)
		require.Len(t, detail.Items, 1)
		leaves := map[string]any{}
		pcapAnalysisLeaves(detail.Items[0].Fields, "$", leaves)
		var matched bool
		for path, value := range leaves {
			number, ok := value.(json.Number)
			if !ok || number.String() != "531" {
				continue
			}
			typed := invoke("query_pcapdb", map[string]any{"id": page.DatasetID, "field_path": path, "field_type": "integer", "field_value": number.String()}).(*pcapdb.ResultPage[pcapdb.ProtocolSummary])
			require.Nil(t, typed.Error)
			require.Len(t, typed.Items, 1)
			require.Equal(t, page.Items[0].ID, typed.Items[0].ID)
			wrongType := invoke("query_pcapdb", map[string]any{"id": page.DatasetID, "field_path": path, "field_type": "string", "field_value": number.String()}).(*pcapdb.ResultPage[pcapdb.ProtocolSummary])
			require.Nil(t, wrongType.Error)
			require.Empty(t, wrongType.Items)
			matched = true
		}
		require.True(t, matched, "discover the DNS numeric leaf before using its path")
	})
	t.Run("precise_time_and_endpoints", func(t *testing.T) {
		page := query("packets", map[string]any{"time_start_ns": "1700000000123456792", "time_end_ns": "1700000000123456793", "destination_port": 80, "source_ip": "10.23.0.10"}).(*pcapdb.ResultPage[pcapdb.PacketSummary])
		require.Nil(t, page.Error)
		require.Len(t, page.Items, 1)
		require.EqualValues(t, 4, page.Items[0].ID)
	})
	t.Run("reassembly_and_bounded_references", func(t *testing.T) {
		sessions := query("sessions", nil).(*pcapdb.ResultPage[pcapdb.SessionSummary])
		require.Nil(t, sessions.Error)
		require.Len(t, sessions.Items, 1)
		session := sessions.Items[0]
		require.True(t, session.Complete)
		require.False(t, session.HasGaps)
		require.False(t, session.Midstream)
		streams := query("streams", map[string]any{"session_id": session.ID}).(*pcapdb.ResultPage[pcapdb.StreamSummary])
		require.Nil(t, streams.Error)
		require.Len(t, streams.Items, 2)
		for _, stream := range streams.Items {
			preview := query("stream_data", map[string]any{"record_id": stream.ID, "preview_bytes": 4, "limit": 1}).(*pcapdb.ResultPage[pcapdb.DataPreview])
			require.Nil(t, preview.Error)
			require.Len(t, preview.Items, 1)
			require.Len(t, preview.Items[0].Data, 4)
			require.True(t, preview.Items[0].Truncated)
			chunkRefs := object(query("chunk_packets", map[string]any{"record_id": preview.Items[0].ID}))
			require.Nil(t, chunkRefs["error"])
			require.NotEmpty(t, chunkRefs["items"])
			page := object(preview)
			require.Nil(t, page["error"])
			encoded, err := json.Marshal(page)
			require.NoError(t, err)
			require.LessOrEqual(t, len(encoded), 8192)
			require.Contains(t, string(encoded), "references_complete")
		}
		refs := object(query("protocol_packets", map[string]any{"record_id": messageID}))
		require.Nil(t, refs["error"])
		require.Len(t, refs["items"], 2, "request spans two captured packets")
		associations := query("packet_sessions", map[string]any{"record_id": 4}).(*pcapdb.ResultPage[pcapdb.SessionSummary])
		require.Nil(t, associations.Error)
		require.Len(t, associations.Items, 1)
		require.Equal(t, session.ID, associations.Items[0].ID)
		packet := object(query("packet_data", map[string]any{"record_id": 4, "preview_bytes": 4}))
		require.Nil(t, packet["error"])
		encoded, _ := json.Marshal(packet)
		require.Contains(t, string(encoded), "truncated\":true")
	})
	t.Run("auto_import_and_capability_errors", func(t *testing.T) {
		other := filepath.Join(home, "auto-only.pcap")
		require.NoError(t, os.WriteFile(other, pcapAnalysisCapture(t, "auto.example"), 0600))
		// A library may have been imported without analysis by a native caller.
		unindexed, err := pcapdb.GetOrCreatePCAPDatabase(other, pcapdb.WithProtocols(false), pcapdb.WithStreams(false))
		require.NoError(t, err)
		otherID := unindexed.ID
		require.NoError(t, unindexed.Close())
		failed := object(invoke("query_pcapdb", map[string]any{"id": otherID}))
		require.NotNil(t, failed["error"], "missing analysis is never zero hits")
		upgraded := invoke("query_pcapdb", map[string]any{"pcapfile": other}).(*pcapdb.ResultPage[pcapdb.ProtocolSummary])
		require.Nil(t, upgraded.Error)
		require.Equal(t, otherID, upgraded.DatasetID)
		require.Len(t, upgraded.Items, 2)
		fresh := filepath.Join(home, "fresh-auto.pcap")
		require.NoError(t, os.WriteFile(fresh, pcapAnalysisCapture(t, "fresh.example"), 0600))
		page := invoke("query_pcapdb", map[string]any{"pcapfile": fresh}).(*pcapdb.ResultPage[pcapdb.ProtocolSummary])
		require.Nil(t, page.Error)
		require.NotEqual(t, id, page.DatasetID)
		require.Len(t, page.Items, 2)
	})
	t.Run("oversized_detail_preserves_error_and_cursor", func(t *testing.T) {
		large := filepath.Join(home, "large-fields.pcap")
		require.NoError(t, os.WriteFile(large, pcapAnalysisCapture(t, strings.Repeat("x", 8192)+".example"), 0600))
		page := invoke("query_pcapdb", map[string]any{"pcapfile": large}).(*pcapdb.ResultPage[pcapdb.ProtocolSummary])
		require.Nil(t, page.Error)
		require.Len(t, page.Items, 2)
		failed := invoke("query_pcapdb", map[string]any{"id": page.DatasetID, "query_type": "details", "record_id": page.Items[0].ID, "result_bytes": 4096}).(*pcapdb.ResultPage[pcapdb.ProtocolDetail])
		require.NotNil(t, failed.Error)
		require.Equal(t, "result_too_large", failed.Error.Code)
		require.Empty(t, failed.Items)
		require.Zero(t, failed.NextCursor)
		retried := invoke("query_pcapdb", map[string]any{"id": page.DatasetID, "query_type": "details", "record_id": page.Items[0].ID, "result_bytes": 65536}).(*pcapdb.ResultPage[pcapdb.ProtocolDetail])
		require.Nil(t, retried.Error)
		require.Len(t, retried.Items, 1)
	})
	t.Run("errors_never_become_empty_success", func(t *testing.T) {
		page := object(query("details", map[string]any{"record_id": 99999}))
		require.Equal(t, "not_found", page["error"].(map[string]any)["code"])
		require.Empty(t, page["items"])
		for _, params := range []map[string]any{
			{}, {"id": id, "pcapfile": path}, {"id": "does-not-exist"},
			{"pcapfile": filepath.Join(home, "missing.pcap")},
			{"id": id, "query_type": "metadata", "protocol": "http"},
			{"id": id, "query_type": "protocols", "source_ip": "10.23.0.10"},
			{"id": id, "field_path": "$.x", "field_type": "integer", "field_value": "abc"},
			{"id": id, "field_path": "$.x", "field_type": "integer", "field_value": "9223372036854775808"},
			{"id": id, "field_path": "$.x", "field_type": "real", "field_value": "NaN"},
			{"id": id, "field_path": "$.x", "field_type": "boolean", "field_value": "yes"},
			{"id": id, "field_path": "$.x", "field_op": "exists", "field_value": "ignored"},
			{"id": id, "query_type": "details"}, {"id": id, "result_bytes": 1024},
			{"id": id, "time_start_ns": "1700000000123456789"},
		} {
			_, err := execute("query_pcapdb", params)
			require.Error(t, err, "params=%v", params)
		}
	})
	t.Run("stored_blobs_survive_source_removal", func(t *testing.T) {
		require.NoError(t, os.Remove(path))
		meta := object(query("metadata", nil))
		require.EqualValues(t, 8, meta["packet_count"])
		page := object(query("protocol_data", map[string]any{"record_id": messageID, "preview_bytes": 4096}))
		require.Nil(t, page["error"])
		require.NotEmpty(t, page["items"])
	})
	t.Run("manage_list_validate_export_delete", func(t *testing.T) {
		manage := func(op string, extra map[string]any) map[string]any {
			params := map[string]any{"op": op}
			for k, v := range extra {
				params[k] = v
			}
			return object(invoke("manage_pcapdb", params))
		}
		count := manage("count", nil)
		require.Greater(t, count["registered_count"].(float64), float64(1))
		page := manage("list", map[string]any{"limit": 1})
		require.Nil(t, page["error"])
		require.Len(t, page["items"], 1)
		require.Equal(t, true, page["has_more"])
		second := manage("list", map[string]any{"limit": 1, "after": page["next_cursor"]})
		require.Nil(t, second["error"])
		require.NotEqual(t, page["items"].([]any)[0].(map[string]any)["dataset_id"], second["items"].([]any)[0].(map[string]any)["dataset_id"])
		valid := manage("validate", map[string]any{"id": id, "deep": true})
		require.Nil(t, valid["error"])
		require.Equal(t, true, valid["valid"])
		output := filepath.Join(home, "managed-export.pcap")
		exported := manage("export", map[string]any{"id": id, "output": output})
		require.Equal(t, output, exported["output"])
		data, err := os.ReadFile(output)
		require.NoError(t, err)
		require.Equal(t, capture, data)
		_, err = execute("manage_pcapdb", map[string]any{"op": "export", "id": id, "output": output})
		require.Error(t, err)
		// An independently held reader must survive a refused deletion.
		held, err := pcapdb.OpenPCAPDatabase(id)
		require.NoError(t, err)
		busy := manage("delete", map[string]any{"id": id})
		require.Equal(t, false, busy["deleted"])
		require.Equal(t, "busy", busy["error"].(map[string]any)["code"])
		require.NoError(t, held.Close())
		deleted := manage("delete", map[string]any{"id": id})
		require.Nil(t, deleted["error"])
		require.Equal(t, true, deleted["deleted"])
		require.Equal(t, false, deleted["cleanup_pending"])
		require.FileExists(t, output)
		_, err = execute("query_pcapdb", map[string]any{"id": id})
		require.Error(t, err)
		require.Nil(t, manage("delete", map[string]any{"id": id})["error"])
		for _, params := range []map[string]any{
			{"op": "delete"}, {"op": "delete", "id": "1"}, {"op": "list", "id": id},
			{"op": "export", "id": id}, {"op": "count", "after": 1}, {"op": "delete", "id": id, "deep": true},
		} {
			_, err := execute("manage_pcapdb", params)
			require.Error(t, err, "params=%v", params)
		}
	})
}
