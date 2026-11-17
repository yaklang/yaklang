package sharkcli

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type objectExportUDPDirection struct {
	clientToServer bool
	clientPort     layers.UDPPort
	serverPort     layers.UDPPort
	payload        []byte
}

func objectExportUDPPacket(t *testing.T, step objectExportUDPDirection) []byte {
	t.Helper()
	srcIP, dstIP := net.IPv4(192, 0, 2, 1), net.IPv4(192, 0, 2, 2)
	srcPort, dstPort := step.clientPort, step.serverPort
	if !step.clientToServer {
		srcIP, dstIP = dstIP, srcIP
		srcPort, dstPort = dstPort, srcPort
	}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: srcIP, DstIP: dstIP}
	udp := &layers.UDP{SrcPort: srcPort, DstPort: dstPort}
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
	buffer := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true},
		&layers.Ethernet{SrcMAC: net.HardwareAddr{0, 1, 2, 3, 4, 5}, DstMAC: net.HardwareAddr{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4},
		ip, udp, gopacket.Payload(step.payload)))
	return buffer.Bytes()
}

func tftpData(block byte, payload []byte) []byte {
	return append([]byte{0, 3, 0, block}, payload...)
}

func TestAnalysisCLIObjectExportAutoDetectedTFTPFiles(t *testing.T) {
	body := append(bytes.Repeat([]byte{'A'}, 512), []byte("tail")...)
	packets := [][]byte{
		objectExportUDPPacket(t, objectExportUDPDirection{true, 40000, 6969, []byte("\x00\x01firmware.bin\x00octet\x00")}),
		objectExportUDPPacket(t, objectExportUDPDirection{false, 40000, 50000, tftpData(1, body[:512])}),
		objectExportUDPPacket(t, objectExportUDPDirection{true, 40000, 50000, []byte{0, 4, 0, 1}}),
		objectExportUDPPacket(t, objectExportUDPDirection{false, 40000, 50000, tftpData(2, body[512:])}),
		objectExportUDPPacket(t, objectExportUDPDirection{true, 40000, 50000, []byte{0, 4, 0, 2}}),
	}
	input := fixture(t, "tftp-auto.pcap", packets...)
	directory := filepath.Join(t.TempDir(), "objects")
	var output bytes.Buffer
	require.NoError(t, invoke(&output, "-r", input, "--json", "--export-objects", directory))
	manifests := objectExportManifests(t, output.Bytes())
	require.Len(t, manifests, 1)
	manifest := manifests[0]
	require.Equal(t, "file", manifest["object_type"])
	require.Equal(t, "tftp", manifest["protocol"])
	require.Equal(t, "download", manifest["direction"])
	require.Equal(t, "192.0.2.2:50000", manifest["source"])
	require.Equal(t, "192.0.2.1:40000", manifest["destination"])
	require.Equal(t, "firmware.bin", manifest["suggested_filename"])
	require.Equal(t, true, manifest["complete"])
	require.EqualValues(t, len(body), manifest["declared_length"])
	require.EqualValues(t, len(body), manifest["observed_length"])
	require.NotZero(t, manifest["transfer_id"])
	require.Equal(t, "reassembled", manifest["byte_source"].(map[string]any)["Kind"])
	wire, err := os.ReadFile(manifest["path"].(string))
	require.NoError(t, err)
	require.Equal(t, body, wire)
}

func TestAnalysisCLIObjectExportTFTPRequiresFinalACK(t *testing.T) {
	packets := [][]byte{
		objectExportUDPPacket(t, objectExportUDPDirection{true, 40000, 69, []byte("\x00\x01incomplete.txt\x00octet\x00")}),
		objectExportUDPPacket(t, objectExportUDPDirection{false, 40000, 50000, tftpData(1, []byte("not complete"))}),
	}
	input := fixture(t, "tftp-incomplete.pcap", packets...)
	directory := filepath.Join(t.TempDir(), "objects")
	var output bytes.Buffer
	require.NoError(t, invoke(&output, "-r", input, "--json", "--export-objects", directory))
	require.Empty(t, objectExportManifests(t, output.Bytes()))
	files, err := os.ReadDir(directory)
	require.NoError(t, err)
	require.Empty(t, files)
}

func TestAnalysisCLIObjectExportTFTPZeroByteFile(t *testing.T) {
	packets := [][]byte{
		objectExportUDPPacket(t, objectExportUDPDirection{true, 40000, 69, []byte("\x00\x01empty.txt\x00octet\x00")}),
		objectExportUDPPacket(t, objectExportUDPDirection{false, 40000, 50000, tftpData(1, nil)}),
		objectExportUDPPacket(t, objectExportUDPDirection{true, 40000, 50000, []byte{0, 4, 0, 1}}),
	}
	input := fixture(t, "tftp-empty.pcap", packets...)
	directory := filepath.Join(t.TempDir(), "objects")
	var output bytes.Buffer
	require.NoError(t, invoke(&output, "-r", input, "--json", "--export-objects", directory))
	manifests := objectExportManifests(t, output.Bytes())
	require.Len(t, manifests, 1)
	require.Equal(t, "file", manifests[0]["object_type"])
	require.EqualValues(t, 0, manifests[0]["observed_length"])
	wire, err := os.ReadFile(manifests[0]["path"].(string))
	require.NoError(t, err)
	require.Empty(t, wire)
}

func TestAnalysisCLIObjectExportTFTPUploadDirection(t *testing.T) {
	packets := [][]byte{
		objectExportUDPPacket(t, objectExportUDPDirection{true, 40000, 69, []byte("\x00\x02upload.txt\x00octet\x00")}),
		objectExportUDPPacket(t, objectExportUDPDirection{false, 40000, 50000, []byte{0, 4, 0, 0}}),
		objectExportUDPPacket(t, objectExportUDPDirection{true, 40000, 50000, tftpData(1, []byte("upload"))}),
		objectExportUDPPacket(t, objectExportUDPDirection{false, 40000, 50000, []byte{0, 4, 0, 1}}),
	}
	input := fixture(t, "tftp-upload.pcap", packets...)
	directory := filepath.Join(t.TempDir(), "objects")
	var output bytes.Buffer
	require.NoError(t, invoke(&output, "-r", input, "--json", "--export-objects", directory))
	manifests := objectExportManifests(t, output.Bytes())
	require.Len(t, manifests, 1)
	require.Equal(t, "upload", manifests[0]["direction"])
	require.Equal(t, "192.0.2.1:40000", manifests[0]["source"])
	require.Equal(t, "192.0.2.2:50000", manifests[0]["destination"])
}

func maskedWebSocketFrame(opcode byte, fin bool, payload []byte) []byte {
	first := opcode
	if fin {
		first |= 0x80
	}
	mask := []byte{1, 2, 3, 4}
	frame := []byte{first, 0x80 | byte(len(payload))}
	frame = append(frame, mask...)
	for i, value := range payload {
		frame = append(frame, value^mask[i%4])
	}
	return frame
}

func TestAnalysisCLIObjectExportWebSocketApplicationMessages(t *testing.T) {
	request := "GET /chat HTTP/1.1\r\nHost: example.test\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n"
	response := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=\r\n\r\n"
	serverFrame := []byte{0x81, 0x05, 'H', 'e', 'l', 'l', 'o'}
	clientFrameOne := maskedWebSocketFrame(1, false, []byte("hel"))
	clientFrameTwo := maskedWebSocketFrame(0, true, []byte("lo"))
	clientSequence := uint32(100)
	serverSequence := uint32(500)
	packets := [][]byte{
		nativeOpeningPacket(t, 1, 99, 0, "", false, true, false),
		nativeOpeningPacket(t, 2, 499, 100, "", true, true, false),
		nativeOpeningPacket(t, 3, clientSequence, serverSequence, request, false, false, false),
	}
	clientSequence += uint32(len(request))
	packets = append(packets, nativeOpeningPacket(t, 4, serverSequence, clientSequence, response, true, false, false))
	serverSequence += uint32(len(response))
	packets = append(packets, nativeOpeningPacket(t, 5, serverSequence, clientSequence, string(serverFrame), true, false, false))
	serverSequence += uint32(len(serverFrame))
	packets = append(packets, nativeOpeningPacket(t, 6, clientSequence, serverSequence, string(clientFrameOne), false, false, false))
	clientSequence += uint32(len(clientFrameOne))
	packets = append(packets, nativeOpeningPacket(t, 7, clientSequence, serverSequence, string(clientFrameTwo), false, false, false))

	input := fixture(t, "websocket-objects.pcap", packets...)
	directory := filepath.Join(t.TempDir(), "objects")
	var output bytes.Buffer
	require.NoError(t, invoke(&output, "-r", input, "--json", "--export-objects", directory))
	manifests := objectExportManifests(t, output.Bytes())
	require.Len(t, manifests, 2)
	got := map[string]map[string]any{}
	for _, manifest := range manifests {
		require.Equal(t, "message", manifest["object_type"])
		require.Equal(t, "websocket", manifest["protocol"])
		require.Equal(t, true, manifest["complete"])
		wire, err := os.ReadFile(manifest["path"].(string))
		require.NoError(t, err)
		got[manifest["direction"].(string)] = map[string]any{"manifest": manifest, "body": string(wire)}
	}
	require.Equal(t, "Hello", got["server_to_client"]["body"])
	require.Equal(t, "text", got["server_to_client"]["manifest"].(map[string]any)["opcode"])
	require.Equal(t, "hello", got["client_to_server"]["body"])
	require.Equal(t, "text", got["client_to_server"]["manifest"].(map[string]any)["opcode"])
	refs := got["client_to_server"]["manifest"].(map[string]any)["packet_refs"].([]any)
	require.Len(t, refs, 2)
}

func TestAnalysisCLIObjectExportGRPCMessages(t *testing.T) {
	input, err := trafficfixture.Materialize(top30Sessions+"first-batch-m1/tls-h2-bidi.pcap", t.TempDir())
	require.NoError(t, err)
	keylog, err := trafficfixture.Materialize(top30Sessions+"first-batch-m1/tls-h2-bidi.keys", t.TempDir())
	require.NoError(t, err)

	directory := filepath.Join(t.TempDir(), "objects")
	var output bytes.Buffer
	require.NoError(t, invoke(&output, "-r", input, "--tls-keylog", keylog, "--json", "--export-objects", directory))
	manifests := objectExportManifests(t, output.Bytes())
	require.Len(t, manifests, 12)

	seenMessages := map[string]bool{}
	directions := map[string]int{}
	for _, manifest := range manifests {
		require.Equal(t, "message", manifest["object_type"])
		require.Equal(t, "grpc", manifest["protocol"])
		require.Equal(t, true, manifest["complete"])
		require.NotZero(t, manifest["stream_id"])
		require.NotZero(t, manifest["message_id"])
		require.NotEmpty(t, manifest["packet_refs"])
		require.Contains(t, manifest["content_type"], "application/grpc")
		require.Equal(t, "POST", manifest["method"])
		require.NotEmpty(t, manifest["uri"])
		require.NotEmpty(t, manifest["authority"])
		direction := manifest["direction"].(string)
		directions[direction]++
		key := manifest["path"].(string)
		require.False(t, seenMessages[key], key)
		seenMessages[key] = true
		wire, err := os.ReadFile(manifest["path"].(string))
		require.NoError(t, err)
		require.NotEmpty(t, wire)
	}
	require.NotZero(t, directions["client_to_server"])
	require.NotZero(t, directions["server_to_client"])
}

func TestAnalysisCLIObjectExportFTPFiles(t *testing.T) {
	input, err := trafficfixture.Materialize("../../../../bin-parser/testdata/winlab5013/captures/ctf-01-ftp.pcapng", t.TempDir())
	require.NoError(t, err)
	jsonFile := filepath.Join(t.TempDir(), "analysis.jsonl")
	directory := filepath.Join(t.TempDir(), "objects")

	var stdout bytes.Buffer
	require.NoError(t, invoke(&stdout, "-r", input, "--json-file", jsonFile, "--export-objects", directory))
	require.Empty(t, stdout.String())
	var ftpObjects []map[string]any
	for _, manifest := range objectExportManifests(t, mustReadFile(t, jsonFile)) {
		if manifest["protocol"] == "ftp" {
			ftpObjects = append(ftpObjects, manifest)
		}
	}
	require.Len(t, ftpObjects, 1)
	manifest := ftpObjects[0]
	require.Equal(t, "file", manifest["object_type"])
	require.Equal(t, "download", manifest["direction"])
	require.Equal(t, true, manifest["complete"])
	require.NotZero(t, manifest["transfer_id"])
	require.NotEmpty(t, manifest["suggested_filename"])
	require.NotEmpty(t, manifest["packet_refs"])
	wire := mustReadFile(t, manifest["path"].(string))
	require.NotEmpty(t, wire)
}

func TestObjectExporterGRPCCompressedPackedAndEmptyMessages(t *testing.T) {
	event := &pcaputil.ProtocolEvent{
		ID: 77, Protocol: "grpc", Status: "decoded", Direction: 0,
		Source: "192.0.2.1:50000", Destination: "192.0.2.2:443",
		SourceBytes: pcaputil.ByteSource{
			Kind:       "decrypted",
			ParentPDUs: []uint64{70},
			PacketRefs: []pcaputil.PacketReference{{Number: 12}, {Number: 13}},
		},
		Session: map[string]any{
			"Stream ID":         uint32(7),
			"GRPC Content Type": "application/grpc+proto",
			"GRPC Messages": []map[string]any{
				{"Index": uint64(1), "Compressed": true, "Payload": []byte("compressed"), "Decoded Payload": []byte("one")},
				{"Index": uint64(2), "Compressed": false, "Payload": []byte{}},
			},
		},
	}
	observations := observedObjects(event)
	require.Len(t, observations, 2)

	exporter, err := newObjectExporter(t.TempDir())
	require.NoError(t, err)
	first, err := exporter.process(observations[0], true)
	require.NoError(t, err)
	require.EqualValues(t, 7, first.StreamID)
	require.EqualValues(t, 1, first.MessageID)
	require.True(t, first.Compressed)
	require.Equal(t, "decompressed", first.ByteSource.Kind)
	wire, err := os.ReadFile(first.Path)
	require.NoError(t, err)
	require.Equal(t, "one", string(wire))

	second, err := exporter.process(observations[1], true)
	require.NoError(t, err)
	require.EqualValues(t, 2, second.MessageID)
	require.Zero(t, second.ObservedLength)
	wire, err = os.ReadFile(second.Path)
	require.NoError(t, err)
	require.Empty(t, wire)
}
