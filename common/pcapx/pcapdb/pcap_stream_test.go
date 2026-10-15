package pcapdb

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

func tcpPacket(t testing.TB, seq, ack uint32, flags string, payload []byte, reverse bool) []byte {
	t.Helper()
	src, dst := net.IP{10, 1, 2, 3}, net.IP{10, 9, 8, 7}
	srcPort, dstPort := layers.TCPPort(32123), layers.TCPPort(80)
	if reverse {
		src, dst = dst, src
		srcPort, dstPort = dstPort, srcPort
	}
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{1, 2, 3, 4, 5, 6}, DstMAC: net.HardwareAddr{6, 5, 4, 3, 2, 1}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, SrcIP: src, DstIP: dst, Protocol: layers.IPProtocolTCP}
	tcp := &layers.TCP{SrcPort: srcPort, DstPort: dstPort, Seq: seq, Ack: ack, Window: 65535, SYN: strings.Contains(flags, "S"), ACK: strings.Contains(flags, "A"), FIN: strings.Contains(flags, "F"), RST: strings.Contains(flags, "R"), PSH: strings.Contains(flags, "P")}
	require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
	buffer := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, tcp, gopacket.Payload(payload)))
	return buffer.Bytes()
}
func tcpCapture(t testing.TB, packets ...[]byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := pcapgo.NewWriterNanos(&buffer)
	require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
	for i, packet := range packets {
		require.NoError(t, writer.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1700000000, int64(i)*1000), CaptureLength: len(packet), Length: len(packet)}, packet))
	}
	return buffer.Bytes()
}
func TestPCAPDBTCPReassemblySessionsAndIndexedAssociations(t *testing.T) {
	capture := tcpCapture(t,
		tcpPacket(t, 100, 0, "S", nil, false), tcpPacket(t, 500, 101, "SA", nil, true), tcpPacket(t, 101, 501, "A", nil, false),
		tcpPacket(t, 106, 501, "AP", []byte("World"), false),
		tcpPacket(t, 101, 501, "AP", []byte("Hello"), false),
		tcpPacket(t, 101, 501, "AP", []byte("Hello"), false),
		tcpPacket(t, 501, 111, "AP", []byte("!"), true),
		tcpPacket(t, 111, 502, "FA", nil, false), tcpPacket(t, 502, 112, "FA", nil, true),
		tcpPacket(t, 1000, 0, "S", nil, false),
	)
	m := testManager(t)
	db, err := m.GetOrCreate(writeCapture(t, capture), WithProtocols(true), WithBatchSize(2))
	require.NoError(t, err)
	sessions, err := db.QuerySessions(QueryTransport("tcp"))
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	require.True(t, sessions[0].Complete)
	require.False(t, sessions[0].HasGaps)
	require.False(t, sessions[0].Midstream)
	require.EqualValues(t, 9, sessions[0].PacketCount)
	require.EqualValues(t, 11, sessions[0].ByteCount)
	require.False(t, sessions[1].Complete)
	require.Equal(t, "capture-end", sessions[1].CloseReason)
	streams, err := db.QueryStreams(QuerySession(sessions[0].ID))
	require.NoError(t, err)
	require.Len(t, streams, 2)
	page, err := db.ReadStream(streams[0].ID)
	require.NoError(t, err)
	require.False(t, page.HasMore)
	require.Len(t, page.Chunks, 2)
	require.Equal(t, "HelloWorld", string(append(append([]byte{}, page.Chunks[0].Data...), page.Chunks[1].Data...)))
	require.EqualValues(t, 0, page.Chunks[0].ByteOffset)
	require.EqualValues(t, 5, page.Chunks[1].ByteOffset)
	require.EqualValues(t, 101, page.Chunks[0].Sequence)
	require.EqualValues(t, 106, page.Chunks[1].Sequence)
	ids, err := db.StreamChunkPacketIDs(page.Chunks[0].ID)
	require.NoError(t, err)
	require.Equal(t, []int64{5}, ids)
	ids, err = db.StreamChunkPacketIDs(page.Chunks[1].ID)
	require.NoError(t, err)
	require.Equal(t, []int64{4}, ids)
	reverse, err := db.ReadStream(streams[1].ID)
	require.NoError(t, err)
	require.Len(t, reverse.Chunks, 1)
	require.Equal(t, "!", string(reverse.Chunks[0].Data))
	packets, err := db.QueryPackets(QuerySession(sessions[0].ID), QueryLimit(2), QueryAfter(5))
	require.NoError(t, err)
	require.Len(t, packets, 2)
	require.EqualValues(t, 6, packets[0].ID)
	for _, packet := range packets {
		require.Nil(t, packet.Data)
	}
	linked, err := db.PacketSessions(6)
	require.NoError(t, err)
	require.Len(t, linked, 1)
	require.Equal(t, sessions[0].ID, linked[0].ID)
	messages, err := db.QueryProtocols(QuerySession(sessions[0].ID))
	require.NoError(t, err)
	require.NotEmpty(t, messages)
	for _, message := range messages {
		require.NotZero(t, message.SessionID)
		require.Equal(t, sessions[0].ID, message.SessionID)
		require.NotZero(t, message.StreamID)
		require.Nil(t, message.Data)
	}
	valid, err := m.Validate(context.Background(), db.ID, true)
	require.NoError(t, err)
	require.True(t, valid.Valid, valid.Error)
	for _, check := range []struct{ query, want string }{
		{"SELECT packet_id FROM session_packets WHERE session_id=1 AND packet_id>2 ORDER BY packet_id LIMIT 10", "session_packets_cursor"},
		{"SELECT id FROM stream_chunks WHERE stream_id=1 AND id>1 ORDER BY id LIMIT 10", "chunks_stream_cursor"},
		{"SELECT id FROM protocol_messages WHERE session_id=1 AND id>1 ORDER BY id LIMIT 10", "messages_session"},
	} {
		rows, err := db.db.Raw("EXPLAIN QUERY PLAN " + check.query).Rows()
		require.NoError(t, err)
		var plans []string
		for rows.Next() {
			var id, parent, unused int
			var plan string
			require.NoError(t, rows.Scan(&id, &parent, &unused, &plan))
			plans = append(plans, plan)
		}
		require.NoError(t, rows.Close())
		require.Contains(t, strings.Join(plans, "\n"), check.want)
	}
	config, err := parseQuery([]QueryOption{QuerySession(sessions[0].ID), QueryAfter(5), QueryLimit(2)}, false)
	require.NoError(t, err)
	rows, err := db.db.Raw("EXPLAIN QUERY PLAN ?", packetAssociationQuery(db.db, config).QueryExpr()).Rows()
	require.NoError(t, err)
	var plans []string
	for rows.Next() {
		var id, parent, unused int
		var plan string
		require.NoError(t, rows.Scan(&id, &parent, &unused, &plan))
		plans = append(plans, plan)
	}
	require.NoError(t, rows.Close())
	plan := strings.Join(plans, "\n")
	require.Contains(t, plan, "SEARCH sp USING INDEX session_packets_cursor")
	require.Contains(t, plan, "SEARCH p USING INTEGER PRIMARY KEY")
	require.NotContains(t, plan, "USE TEMP B-TREE")

}
func TestPCAPDBStreamsReportGapsAndMidstream(t *testing.T) {
	m := testManager(t)
	capture := tcpCapture(t, tcpPacket(t, 100, 0, "S", nil, false), tcpPacket(t, 101, 0, "AP", []byte("Hello"), false), tcpPacket(t, 111, 0, "AP", []byte("future"), false))
	db, err := m.GetOrCreate(writeCapture(t, capture))
	require.NoError(t, err)
	sessions, err := db.QuerySessions()
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.True(t, sessions[0].HasGaps)
	require.False(t, sessions[0].Complete)
	streams, err := db.QueryStreams(QuerySession(sessions[0].ID))
	require.NoError(t, err)
	page, err := db.ReadStream(streams[0].ID)
	require.NoError(t, err)
	require.Len(t, page.Chunks, 1)
	require.Equal(t, "Hello", string(page.Chunks[0].Data))
	mid, err := m.GetOrCreate(writeCapture(t, tcpCapture(t, tcpPacket(t, 100, 0, "AP", []byte("midstream"), false))))
	require.NoError(t, err)
	sessions, err = mid.QuerySessions()
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.True(t, sessions[0].Midstream)
	require.False(t, sessions[0].Complete)
}
func TestPCAPDBStreamPagesBoundBytesAndCursor(t *testing.T) {
	m := testManager(t)
	packets := [][]byte{tcpPacket(t, 100, 0, "S", nil, false)}
	var expected bytes.Buffer
	sequence := uint32(101)
	for i := 0; i < 70; i++ {
		data := bytes.Repeat([]byte{byte(i)}, 32<<10)
		expected.Write(data)
		packets = append(packets, tcpPacket(t, sequence, 0, "AP", data, false))
		sequence += uint32(len(data))
	}
	db, err := m.GetOrCreate(writeCapture(t, tcpCapture(t, packets...)), WithBatchSize(3))
	require.NoError(t, err)
	streams, err := db.QueryStreams()
	require.NoError(t, err)
	require.Len(t, streams, 2)
	var cursor int64
	var actual bytes.Buffer
	pages := 0
	for {
		page, err := db.ReadStream(streams[0].ID, QueryAfter(cursor), QueryLimit(1000), QueryMaxBytes(64<<10))
		require.NoError(t, err)
		require.LessOrEqual(t, page.Bytes, 64<<10)
		require.NotEmpty(t, page.Chunks)
		for _, chunk := range page.Chunks {
			require.Greater(t, int64(chunk.ID), cursor)
			require.EqualValues(t, actual.Len(), chunk.ByteOffset)
			actual.Write(chunk.Data)
			cursor = int64(chunk.ID)
		}
		require.Equal(t, cursor, page.NextAfter)
		pages++
		if !page.HasMore {
			break
		}
	}
	require.Equal(t, 35, pages)
	require.Equal(t, expected.Bytes(), actual.Bytes())
	page, err := db.ReadStream(streams[0].ID)
	require.NoError(t, err)
	require.LessOrEqual(t, page.Bytes, 256<<10)
	require.True(t, page.HasMore)
	_, err = db.ReadStream(streams[0].ID, QueryMaxBytes(2<<20))
	require.Error(t, err)
	_, err = db.ReadStream(streams[0].ID, QueryMaxBytes(1))
	require.Error(t, err)
	_, err = db.ReadStream(9999)
	require.ErrorIs(t, err, ErrNotFound)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = db.ReadStream(streams[0].ID, QueryContext(ctx))
	require.ErrorIs(t, err, context.Canceled)
	valid, err := m.Validate(context.Background(), db.ID, true)
	require.NoError(t, err)
	require.True(t, valid.Valid, valid.Error)
}
func TestPCAPDBUDPProtocolAndSessionAssociations(t *testing.T) {
	m := testManager(t)
	db, err := m.GetOrCreate(writeCapture(t, classicCapture(t, 3)), WithProtocols(true), WithBatchSize(1))
	require.NoError(t, err)
	sessions, err := db.QuerySessions(QueryTransport("udp"))
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.EqualValues(t, 3, sessions[0].PacketCount)
	messages, err := db.QueryProtocols(QuerySession(sessions[0].ID), QueryProtocol("dns"))
	require.NoError(t, err)
	require.Len(t, messages, 3)
	streams, err := db.QueryStreams(QuerySession(sessions[0].ID))
	require.NoError(t, err)
	require.Len(t, streams, 2)
	require.Equal(t, "datagram", streams[0].Kind)
	for _, message := range messages {
		require.NotZero(t, message.StreamID)
		require.Equal(t, streams[0].ID, message.StreamID)
	}
	page, err := db.ReadStream(streams[0].ID)
	require.NoError(t, err)
	require.Len(t, page.Chunks, 3)
	for _, chunk := range page.Chunks {
		data, err := db.ReadPacket(uint(chunk.ID))
		require.NoError(t, err)
		packet := gopacket.NewPacket(data, layers.LinkTypeEthernet, gopacket.Default)
		require.Equal(t, packet.TransportLayer().LayerPayload(), chunk.Data)
	}
}
func TestPCAPDBClosedChildDatabaseIsSelfContainedAndPortable(t *testing.T) {
	m := testManager(t)
	capture := classicCapture(t, 4)
	input := writeCapture(t, capture)
	db, err := m.GetOrCreate(input, WithProtocols(true))
	require.NoError(t, err)
	meta, err := db.Metadata()
	require.NoError(t, err)
	require.NoError(t, m.Close())
	require.NoError(t, os.Remove(input))
	contents, err := os.ReadFile(meta.DatabasePath)
	require.NoError(t, err)
	root := t.TempDir()
	directory := filepath.Join(root, meta.DatasetID)
	require.NoError(t, os.Mkdir(directory, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "index.sqlite"), contents, 0600))
	ported, err := NewInstanceManager(testProfile(t), root)
	require.NoError(t, err)
	defer ported.Close()
	opened, err := ported.Open(context.Background(), meta.DatasetID)
	require.NoError(t, err)
	raw, err := opened.ReadPacket(3)
	require.NoError(t, err)
	require.Equal(t, dnsPacket(t), raw)
	messages, err := opened.QueryProtocols()
	require.NoError(t, err)
	require.Len(t, messages, 4)
	data, err := opened.ReadProtocol(messages[0].ID)
	require.NoError(t, err)
	require.NotEmpty(t, data)
	sessions, err := opened.QuerySessions()
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	output := filepath.Join(t.TempDir(), "portable.pcap")
	_, err = opened.Export(output)
	require.NoError(t, err)
	exported, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, capture, exported)
	valid, err := ported.Validate(context.Background(), meta.DatasetID, true)
	require.NoError(t, err)
	require.True(t, valid.Valid, valid.Error)
	entries, err := os.ReadDir(filepath.Dir(meta.DatabasePath))
	require.NoError(t, err)
	for _, entry := range entries {
		require.False(t, entry.IsDir())
	}
}
func TestPCAPDBReadSnapshotSurvivesConcurrentRebuild(t *testing.T) {
	m := testManager(t)
	db, err := m.GetOrCreate(writeCapture(t, classicCapture(t, 2)))
	require.NoError(t, err)
	ctx := context.Background()
	tx, meta, err := db.readSnapshot(ctx, false, true)
	require.NoError(t, err)
	defer tx.Rollback()
	lock, err := acquireFileLock(ctx, datasetLockPath(meta), false)
	require.NoError(t, err)
	defer releaseFileLock(lock)
	writer, err := openIndex(meta.DatabasePath, true, false)
	require.NoError(t, err)
	defer writer.Close()
	meta.State = StateAnalyzing
	require.NoError(t, writeManifest(ctx, writer, meta))
	var packets []Packet
	require.NoError(t, indexWithContext(ctx, tx).Find(&packets).Error)
	require.Len(t, packets, 2)
	old, err := readManifest(ctx, tx, db.ID)
	require.NoError(t, err)
	require.Equal(t, StateReady, old.State)
	_, err = db.QueryPackets()
	require.ErrorIs(t, err, ErrNotReady)
	meta.State = StateReady
	require.NoError(t, writeManifest(ctx, writer, meta))
}
func TestPCAPDBPacketOnlyImportCanUpgradeStreams(t *testing.T) {
	m := testManager(t)
	input := writeCapture(t, classicCapture(t, 2))
	db, err := m.GetOrCreate(input, WithStreams(false))
	require.NoError(t, err)
	_, err = db.QuerySessions()
	require.ErrorContains(t, err, "have not been indexed")
	upgraded, err := m.GetOrCreate(input, WithStreams(true))
	require.NoError(t, err)
	require.Same(t, db, upgraded)
	sessions, err := db.QuerySessions()
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	meta, err := db.Metadata()
	require.NoError(t, err)
	require.True(t, meta.StreamsIndexed)
	require.False(t, meta.ProtocolsIndexed)
	var columns int64
	require.NoError(t, db.db.Raw("SELECT count(*) FROM pragma_table_info('packets') WHERE name IN (?,?)", "data_offset", "record_offset").Row().Scan(&columns))
	require.Zero(t, columns)
	require.NoError(t, m.profile.Raw("SELECT count(*) FROM pragma_table_info('pcapfile_db_metadata') WHERE name IN (?,?,?)", "data", "fields", "raw_path").Row().Scan(&columns))
	require.Zero(t, columns)
}
func TestPCAPDBLongerBufferedRetransmissionHasExplicitProvenanceLimit(t *testing.T) {
	m := testManager(t)
	capture := tcpCapture(t, tcpPacket(t, 100, 0, "S", nil, false), tcpPacket(t, 106, 0, "AP", []byte("World"), false), tcpPacket(t, 106, 0, "AP", []byte("World!!!"), false), tcpPacket(t, 101, 0, "AP", []byte("Hello"), false))
	db, err := m.GetOrCreate(writeCapture(t, capture))
	require.NoError(t, err)
	streams, err := db.QueryStreams()
	require.NoError(t, err)
	page, err := db.ReadStream(streams[0].ID)
	require.NoError(t, err)
	require.Len(t, page.Chunks, 2)
	require.Equal(t, "HelloWorld!!!", string(append(append([]byte{}, page.Chunks[0].Data...), page.Chunks[1].Data...)))
	require.True(t, page.Chunks[0].ReferencesComplete)
	require.False(t, page.Chunks[1].ReferencesComplete)
	packets, err := db.QueryPackets(QueryStream(streams[0].ID))
	require.NoError(t, err)
	require.Len(t, packets, 4)
}

// Construct the historical table shape while using GORM for its actual rows.
type catalogBeforeBLOB struct {
	PCAPFileDBMetadata `gorm:"embedded"`
	RawPath            string `gorm:"not null"`
	RawModTimeNS       int64
	ProtocolModTimeNS  int64
}

func (catalogBeforeBLOB) TableName() string { return "pcapfile_db_metadata" }
func TestPCAPDBMetadataMigrationRemovesObsoleteRequiredPaths(t *testing.T) {
	profile := testProfile(t)
	require.NoError(t, profile.AutoMigrate(&PCAPFileDBMetadata{}).Error)
	require.NoError(t, profile.Exec("ALTER TABLE pcapfile_db_metadata ADD COLUMN raw_path TEXT NOT NULL").Error)
	require.NoError(t, profile.Exec("ALTER TABLE pcapfile_db_metadata ADD COLUMN raw_mod_time_ns INTEGER").Error)
	require.NoError(t, profile.Exec("ALTER TABLE pcapfile_db_metadata ADD COLUMN protocol_mod_time_ns INTEGER").Error)
	legacy := catalogBeforeBLOB{PCAPFileDBMetadata: PCAPFileDBMetadata{DatasetID: uuid.NewString(), DatabaseType: "sqlite", DatabasePath: filepath.Join(t.TempDir(), "missing.sqlite"), SourcePath: "/old/input.pcap", SourceAliases: "[]", Fingerprint: "old", State: StateInvalid}, RawPath: "/old/capture.pcap"}
	require.NoError(t, profile.Create(&legacy).Error)
	m, err := NewInstanceManager(profile, t.TempDir())
	require.NoError(t, err)
	defer m.Close()
	db, err := m.GetOrCreate(writeCapture(t, classicCapture(t, 1)))
	require.NoError(t, err)
	meta, err := db.Metadata()
	require.NoError(t, err)
	require.Equal(t, StateReady, meta.State)
	stored, err := m.Metadata(legacy.DatasetID)
	require.NoError(t, err)
	require.Equal(t, legacy.SourcePath, stored.SourcePath)
	for _, column := range []string{"raw_path", "raw_mod_time_ns", "protocol_mod_time_ns"} {
		require.False(t, profile.Dialect().HasColumn("pcapfile_db_metadata", column))
	}
}

func TestPCAPDBRebuildDerivedDataWithoutOriginalSource(t *testing.T) {
	m := testManager(t)
	input := writeCapture(t, classicCapture(t, 3))
	db, err := m.GetOrCreate(input, WithProtocols(true))
	require.NoError(t, err)
	meta, err := db.Metadata()
	require.NoError(t, err)
	require.NoError(t, os.Remove(input))
	writer, err := openIndex(meta.DatabasePath, true, false)
	require.NoError(t, err)
	var chunk PCAPStreamChunk
	require.NoError(t, writer.First(&chunk).Error)
	chunk.Data[0] ^= 1
	require.NoError(t, writer.Model(&PCAPStreamChunk{}).Where("id = ?", chunk.ID).UpdateColumn("data", chunk.Data).Error)
	require.NoError(t, writer.Close())
	valid, err := m.Validate(context.Background(), db.ID, true)
	require.NoError(t, err)
	require.False(t, valid.Valid)
	require.Contains(t, valid.Error, "stream BLOB hash mismatch")
	_, err = db.ReadStream(chunk.StreamID)
	require.ErrorIs(t, err, ErrNotReady)
	repaired, err := db.RebuildAnalysis(WithBatchSize(1))
	require.NoError(t, err)
	require.Same(t, db, repaired)
	valid, err = m.Validate(context.Background(), db.ID, true)
	require.NoError(t, err)
	require.True(t, valid.Valid, valid.Error)
	messages, err := db.QueryProtocols()
	require.NoError(t, err)
	require.Len(t, messages, 3)
	page, err := db.ReadStream(chunk.StreamID)
	require.NoError(t, err)
	require.Len(t, page.Chunks, 3)
	require.NotEqual(t, chunk.Data, page.Chunks[0].Data)
	// Refuse to rebuild from a modified captured packet; preserve that BLOB.
	writer, err = openIndex(meta.DatabasePath, true, false)
	require.NoError(t, err)
	var packet Packet
	require.NoError(t, writer.First(&packet).Error)
	packet.Data[0] ^= 1
	require.NoError(t, writer.Model(&Packet{}).Where("id = ?", packet.ID).UpdateColumn("data", packet.Data).Error)
	require.NoError(t, writer.Close())
	_, err = db.RebuildAnalysis()
	require.ErrorContains(t, err, "source bytes cannot be repaired")
	writer, err = openIndex(meta.DatabasePath, false, false)
	require.NoError(t, err)
	var preserved Packet
	require.NoError(t, writer.First(&preserved).Error)
	require.Equal(t, packet.Data, preserved.Data)
	require.NoError(t, writer.Close())
}
