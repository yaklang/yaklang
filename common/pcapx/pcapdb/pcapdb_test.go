package pcapdb

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

func testProfile(t testing.TB) *gorm.DB {
	t.Helper()
	profile, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "profile.sqlite"))
	require.NoError(t, err)
	profile.DB().SetMaxOpenConns(1)
	t.Cleanup(func() { profile.Close() })
	return profile
}

func testManager(t testing.TB) *InstanceManager {
	t.Helper()
	m, err := NewInstanceManager(testProfile(t), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	return m
}

func dnsPacket(t testing.TB) []byte {
	t.Helper()
	ethernet := &layers.Ethernet{SrcMAC: net.HardwareAddr{1, 2, 3, 4, 5, 6}, DstMAC: net.HardwareAddr{6, 5, 4, 3, 2, 1}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, SrcIP: net.IP{10, 1, 2, 3}, DstIP: net.IP{8, 8, 8, 8}, Protocol: layers.IPProtocolUDP}
	udp := &layers.UDP{SrcPort: 30000, DstPort: 53}
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
	dns := &layers.DNS{ID: 123, RD: true, Questions: []layers.DNSQuestion{{Name: []byte("pcapdb.example"), Type: layers.DNSTypeA, Class: layers.DNSClassIN}}}
	buffer := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, ethernet, ip, udp, dns))
	return buffer.Bytes()
}

func classicCapture(t testing.TB, count int) []byte {
	t.Helper()
	var b bytes.Buffer
	writer := pcapgo.NewWriterNanos(&b)
	require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
	packet := dnsPacket(t)
	for i := 0; i < count; i++ {
		require.NoError(t, writer.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1700000000+int64(i), 123456789), CaptureLength: len(packet), Length: len(packet)}, packet))
	}
	return b.Bytes()
}

func writeCapture(t testing.TB, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.pcap")
	require.NoError(t, os.WriteFile(path, data, 0600))
	return path
}

func TestPCAPDBImportQueryExportAndCatalogSeparation(t *testing.T) {
	m := testManager(t)
	capture := classicCapture(t, 4)
	input := writeCapture(t, capture)
	var states []State
	db, err := m.GetOrCreate(input, WithBatchSize(2), WithProgress(func(p Progress) { states = append(states, p.State) }))
	require.NoError(t, err)
	meta, err := db.Metadata()
	require.NoError(t, err)
	require.Equal(t, StateReady, meta.State)
	require.EqualValues(t, 4, meta.PacketCount)
	require.EqualValues(t, len(capture), meta.BytesIndexed)
	require.False(t, meta.ProtocolsIndexed)
	require.Contains(t, states, StateImporting)
	require.Contains(t, states, StateIndexing)
	require.Equal(t, StateReady, states[len(states)-1])
	require.True(t, m.profile.HasTable(&PCAPFileDBMetadata{}))
	for _, model := range []interface{}{&PCAPDatasetInfo{}, &PCAPCaptureInterface{}, &Packet{}, &PCAPProtocolMessage{}, &PCAPMessagePacket{}, &PCAPCaptureRecord{}, &PCAPSession{}, &PCAPStream{}, &PCAPStreamChunk{}, &PCAPSessionPacket{}, &PCAPStreamChunkPacket{}} {
		require.False(t, m.profile.HasTable(model), "profile must hold catalog only")
		require.True(t, db.db.HasTable(model), "traffic models must live in the sub-DB")
	}
	packets, err := db.QueryPackets(QuerySourceIP("10.1.2.3"), QueryDestinationPort(53), QueryLimit(2))
	require.NoError(t, err)
	require.Len(t, packets, 2)
	require.Equal(t, "udp", packets[0].Transport)
	require.EqualValues(t, 1700000000123456789, *packets[0].TimestampNS)
	page, err := db.QueryPackets(QueryAfter(int64(packets[1].ID)), QueryLimit(2))
	require.NoError(t, err)
	require.Len(t, page, 2)
	require.EqualValues(t, 3, page[0].ID)
	raw, err := db.ReadPacket(3)
	require.NoError(t, err)
	require.Equal(t, dnsPacket(t), raw)
	require.NoError(t, os.Remove(input))
	output := filepath.Join(t.TempDir(), "export.pcap")
	_, err = m.Export(context.Background(), input, output)
	require.NoError(t, err)
	exported, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, capture, exported)
	_, err = db.Export(output)
	require.ErrorIs(t, err, os.ErrExist)
	validated, err := m.Validate(context.Background(), db.ID, true)
	require.NoError(t, err)
	require.True(t, validated.Valid, validated.Error)
	require.NoError(t, db.Close())
	_, err = db.QueryPackets()
	require.ErrorIs(t, err, ErrClosed)
	reopened, err := m.Open(context.Background(), meta.DatasetID)
	require.NoError(t, err)
	require.NotSame(t, db, reopened)
}

func TestPCAPDBProtocolsJSONBAndPacketReferences(t *testing.T) {
	m := testManager(t)
	input := writeCapture(t, classicCapture(t, 2))
	db, err := m.GetOrCreate(input)
	require.NoError(t, err)
	_, err = db.QueryProtocols()
	require.Error(t, err)
	again, err := m.GetOrCreate(input, WithProtocols(true), WithBatchSize(1))
	require.NoError(t, err)
	require.Equal(t, db.ID, again.ID)
	meta, err := db.Metadata()
	require.NoError(t, err)
	require.True(t, meta.ProtocolsIndexed)
	require.EqualValues(t, 2, meta.ProtocolCount)
	messages, err := db.QueryProtocols(QueryProtocol("dns"), QueryWithFields(true))
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.NotEmpty(t, messages[0].Fields)
	var storedType string
	require.NoError(t, db.db.Raw("SELECT typeof(fields) FROM protocol_messages LIMIT 1").Row().Scan(&storedType))
	require.Equal(t, "blob", storedType)
	ids, err := db.ProtocolPacketIDs(messages[0].ID)
	require.NoError(t, err)
	require.Equal(t, []int64{1}, ids)
	messageBytes, err := db.ReadProtocol(messages[0].ID)
	require.NoError(t, err)
	require.NotEmpty(t, messageBytes)
	// Query a discovered scalar path without assuming the parser's field names.
	var path, value string
	require.NoError(t, db.db.Raw("SELECT fullkey,CAST(value AS TEXT) FROM json_tree((SELECT fields FROM protocol_messages LIMIT 1)) WHERE type='text' LIMIT 1").Row().Scan(&path, &value))
	matched, err := db.QueryProtocols(QueryProtocol("dns"), QueryField(path, value))
	require.NoError(t, err)
	require.NotEmpty(t, matched)
	count, err := m.Count()
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	valid, err := m.Validate(context.Background(), db.ID, true)
	require.NoError(t, err)
	require.True(t, valid.Valid, valid.Error)
	_, err = m.GetOrCreate(input, WithProtocols(true))
	require.NoError(t, err)
	meta, err = db.Metadata()
	require.NoError(t, err)
	require.EqualValues(t, 2, meta.ProtocolCount, "repeat import must not append duplicate messages")
}

func TestPCAPDBInvalidationAndCorruptIndexPreservation(t *testing.T) {
	m := testManager(t)
	input := writeCapture(t, classicCapture(t, 3))
	db, err := m.GetOrCreate(input)
	require.NoError(t, err)
	meta, err := db.Metadata()
	require.NoError(t, err)
	data, err := db.ReadPacket(3)
	require.NoError(t, err)
	data[len(data)-1] ^= 1
	writer, err := openIndex(meta.DatabasePath, true, false)
	require.NoError(t, err)
	require.NoError(t, writer.Model(&Packet{}).Where("id = ?", 3).UpdateColumn("data", data).Error)
	require.NoError(t, writer.Close())
	validation, err := m.Validate(context.Background(), db.ID, true)
	require.NoError(t, err)
	require.False(t, validation.Valid)
	require.Contains(t, validation.Error, "hash mismatch")
	_, err = db.QueryPackets()
	require.ErrorIs(t, err, ErrNotReady, "open handles must see validation state changes")
	_, err = m.GetOrCreate(input)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	require.NoError(t, m.Close())
	for _, suffix := range []string{"-wal", "-shm"} {
		err := os.Remove(meta.DatabasePath + suffix)
		require.True(t, err == nil || os.IsNotExist(err))
	}
	corrupt := []byte("this is not a SQLite database")
	require.NoError(t, os.WriteFile(meta.DatabasePath, corrupt, 0600))
	restarted, err := NewInstanceManager(m.profile, m.root)
	require.NoError(t, err)
	defer restarted.Close()
	validation, err = restarted.Validate(context.Background(), meta.DatasetID, false)
	require.NoError(t, err)
	require.False(t, validation.Valid)
	actual, err := os.ReadFile(meta.DatabasePath)
	require.NoError(t, err)
	require.Equal(t, corrupt, actual, "validation must not repair or replace corrupt indexes")
}

func TestPCAPDBProgressFailureAndCanceledExport(t *testing.T) {
	m := testManager(t)
	input := writeCapture(t, classicCapture(t, 3))
	var err error
	require.NotPanics(t, func() {
		_, err = m.GetOrCreate(input, WithProgress(func(p Progress) {
			if p.State == StateIndexing {
				panic("consumer failed")
			}
		}))
	})
	require.ErrorContains(t, err, "consumer failed")
	entries, err := m.List()
	require.NoError(t, err)
	require.Equal(t, StateFailed, entries[0].State)
	require.Contains(t, entries[0].LastError, "consumer failed")
	db, err := m.GetOrCreate(input)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	output := filepath.Join(t.TempDir(), "canceled.pcap")
	_, err = m.Export(ctx, db.ID, output)
	require.ErrorIs(t, err, context.Canceled)
	_, err = os.Stat(output)
	require.ErrorIs(t, err, os.ErrNotExist)
	validation, err := m.Validate(context.Background(), db.ID, false)
	require.NoError(t, err)
	require.True(t, validation.Valid, "canceling validation/export must not invalidate the dataset")
}

func TestPCAPDBLostCatalogRegistrationRecovery(t *testing.T) {
	m := testManager(t)
	input := writeCapture(t, classicCapture(t, 3))
	db, err := m.GetOrCreate(input)
	require.NoError(t, err)
	meta, err := db.Metadata()
	require.NoError(t, err)
	require.NoError(t, m.Close())
	require.NoError(t, os.Remove(input))
	require.NoError(t, m.profile.Unscoped().Where("dataset_id = ?", meta.DatasetID).Delete(&PCAPFileDBMetadata{}).Error)
	recovered, err := NewInstanceManager(m.profile, m.root)
	require.NoError(t, err)
	defer recovered.Close()
	entries, err := recovered.List()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, meta.DatasetID, entries[0].DatasetID)
	require.EqualValues(t, 3, entries[0].PacketCount)
	require.True(t, recovered.StartupValidation()[0].Valid)
	_, err = recovered.Open(context.Background(), meta.DatasetID)
	require.NoError(t, err)
}

func TestPCAPDBCatalogRecoveryDoesNotOverwriteAnotherProfileRow(t *testing.T) {
	original := testManager(t)
	db, err := original.GetOrCreate(writeCapture(t, classicCapture(t, 3)))
	require.NoError(t, err)
	originalMeta, err := db.Metadata()
	require.NoError(t, err)
	require.NoError(t, original.Close())

	profile := testProfile(t)
	other, err := NewInstanceManager(profile, t.TempDir())
	require.NoError(t, err)
	defer other.Close()
	otherDB, err := other.GetOrCreate(writeCapture(t, classicCapture(t, 4)))
	require.NoError(t, err)
	otherMeta, err := otherDB.Metadata()
	require.NoError(t, err)
	require.Equal(t, originalMeta.Model.ID, otherMeta.Model.ID)

	recovered, err := NewInstanceManager(profile, original.root)
	require.NoError(t, err)
	defer recovered.Close()
	count, err := recovered.Count()
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
	first, err := recovered.Metadata(originalMeta.DatasetID)
	require.NoError(t, err)
	second, err := recovered.Metadata(otherMeta.DatasetID)
	require.NoError(t, err)
	require.NotEqual(t, first.Model.ID, second.Model.ID)
	require.EqualValues(t, 3, first.PacketCount)
	require.EqualValues(t, 4, second.PacketCount)
}

func TestPCAPDBManagerCloseCancelsActiveImport(t *testing.T) {
	m := testManager(t)
	input := writeCapture(t, classicCapture(t, 100))
	entered, release := make(chan struct{}), make(chan struct{})
	done, closed := make(chan error, 1), make(chan error, 1)
	var once sync.Once
	go func() {
		_, err := m.GetOrCreate(input, WithProgress(func(p Progress) {
			if p.State == StateIndexing {
				once.Do(func() { close(entered); <-release })
			}
		}))
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("import did not start")
	}
	go func() { closed <- m.Close() }()
	<-m.ctx.Done()
	close(release)
	require.ErrorIs(t, <-done, context.Canceled)
	require.NoError(t, <-closed)
	var entry PCAPFileDBMetadata
	require.NoError(t, m.profile.First(&entry).Error)
	require.Equal(t, StateInterrupted, entry.State)
	_, err := m.List()
	require.ErrorIs(t, err, ErrClosed)
}

func TestPCAPDBMissingIndexAndUninitializedIndexRetry(t *testing.T) {
	m := testManager(t)
	input := writeCapture(t, classicCapture(t, 2))
	db, err := m.GetOrCreate(input)
	require.NoError(t, err)
	meta, err := db.Metadata()
	require.NoError(t, err)
	require.NoError(t, db.Close())
	require.NoError(t, os.Remove(meta.DatabasePath))
	validation, err := m.Validate(context.Background(), meta.DatasetID, false)
	require.NoError(t, err)
	require.False(t, validation.Valid)
	_, err = os.Stat(meta.DatabasePath)
	require.ErrorIs(t, err, os.ErrNotExist, "validation must not create a new empty index")
	// Simulate a crash after SQLite opened a file, before the schema commit.
	writer, err := openIndex(meta.DatabasePath, true, true)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	db, err = m.GetOrCreate(input)
	require.NoError(t, err)
	require.Equal(t, meta.DatasetID, db.ID)
	packets, err := db.QueryPackets()
	require.NoError(t, err)
	require.Len(t, packets, 2)
}

func TestPCAPDBBigEndianAndSimplePacketBLOBs(t *testing.T) {
	packet := dnsPacket(t)
	order := binary.BigEndian
	classic := make([]byte, 40)
	order.PutUint32(classic[:4], 0xa1b23c4d)
	order.PutUint16(classic[4:6], 2)
	order.PutUint16(classic[6:8], 4)
	order.PutUint32(classic[16:20], 65535)
	order.PutUint32(classic[20:24], 1)
	order.PutUint32(classic[24:28], 1700000000)
	order.PutUint32(classic[28:32], 123456789)
	order.PutUint32(classic[32:36], uint32(len(packet)))
	order.PutUint32(classic[36:40], uint32(len(packet)))
	classic = append(classic, packet...)
	m := testManager(t)
	db, err := m.GetOrCreate(writeCapture(t, classic))
	require.NoError(t, err)
	rows, err := db.QueryPackets()
	require.NoError(t, err)
	require.EqualValues(t, 1700000000123456789, *rows[0].TimestampNS)
	block := func(kind uint32, body []byte) []byte {
		result := make([]byte, len(body)+12)
		order.PutUint32(result[:4], kind)
		order.PutUint32(result[4:8], uint32(len(result)))
		copy(result[8:], body)
		order.PutUint32(result[len(result)-4:], uint32(len(result)))
		return result
	}
	section := make([]byte, 16)
	order.PutUint32(section[:4], 0x1a2b3c4d)
	order.PutUint16(section[4:6], 1)
	order.PutUint64(section[8:], ^uint64(0))
	iface := make([]byte, 8)
	order.PutUint16(iface[:2], 1)
	order.PutUint32(iface[4:], 65535)
	simple := make([]byte, 4+(len(packet)+3)&^3)
	order.PutUint32(simple[:4], uint32(len(packet)))
	copy(simple[4:], packet)
	ng := append(block(0x0a0d0d0a, section), block(1, iface)...)
	ng = append(ng, block(3, simple)...)
	db, err = m.GetOrCreate(writeCapture(t, ng))
	require.NoError(t, err)
	rows, err = db.QueryPackets()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Nil(t, rows[0].TimestampNS, "SPB has no timestamp")
	raw, err := db.ReadPacket(rows[0].ID)
	require.NoError(t, err)
	require.Equal(t, packet, raw)
}

func TestPCAPDBConcurrentDeduplicationAndStartupBusy(t *testing.T) {
	profile := testProfile(t)
	root := t.TempDir()
	m, err := NewInstanceManager(profile, root)
	require.NoError(t, err)
	defer m.Close()
	capture := classicCapture(t, 5)
	input, alias := writeCapture(t, capture), writeCapture(t, capture)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	results := make(chan *Database, 3)
	failures := make(chan error, 3)
	go func() {
		db, err := m.GetOrCreate(input, WithProgress(func(p Progress) {
			if p.State == StateIndexing {
				once.Do(func() { close(entered); <-release })
			}
		}))
		results <- db
		failures <- err
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("import did not start")
	}
	other, err := NewInstanceManager(profile, root)
	require.NoError(t, err)
	defer other.Close()
	require.Len(t, other.StartupValidation(), 1)
	require.True(t, other.StartupValidation()[0].Busy)
	entries, err := other.List()
	require.NoError(t, err)
	require.Equal(t, StateIndexing, entries[0].State, "live import must not be marked interrupted")
	go func() { db, err := other.GetOrCreate(alias); results <- db; failures <- err }()
	go func() { db, err := m.GetOrCreate(input); results <- db; failures <- err }()
	close(release)
	ids := make(map[string]bool)
	for i := 0; i < 3; i++ {
		require.NoError(t, <-failures)
		db := <-results
		require.NotNil(t, db)
		ids[db.ID] = true
	}
	require.Len(t, ids, 1)
	count, err := m.Count()
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	resolved, err := m.Metadata(alias)
	require.NoError(t, err)
	require.Contains(t, resolved.SourceAliases, alias)
}

func TestPCAPDBCancellationRecoveryAndDurableCounts(t *testing.T) {
	m := testManager(t)
	input := writeCapture(t, classicCapture(t, 7))
	ctx, cancel := context.WithCancel(context.Background())
	_, err := m.GetOrCreate(input, WithContext(ctx), WithBatchSize(2), WithProgress(func(p Progress) {
		if p.State == StateIndexing && p.PacketCount >= 2 {
			cancel()
		}
	}))
	require.ErrorIs(t, err, context.Canceled)
	entries, err := m.List()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, StateInterrupted, entries[0].State)
	require.EqualValues(t, 2, entries[0].PacketCount)
	db, err := m.GetOrCreate(input)
	require.NoError(t, err)
	require.Equal(t, entries[0].DatasetID, db.ID)
	packets, err := db.QueryPackets()
	require.NoError(t, err)
	require.Len(t, packets, 7)

	// Three complete packets and a partial header: only the first two commit.
	broken := append(classicCapture(t, 3), []byte{1, 2, 3}...)
	_, err = m.GetOrCreate(writeCapture(t, broken), WithBatchSize(2))
	require.Error(t, err)
	entries, err = m.List()
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.State == StateFailed {
			require.EqualValues(t, 2, entry.PacketCount, "uncommitted packet must not leak into checkpoint")
		}
	}
}

func TestPCAPDBStartupReconciliationAndInvalidFiles(t *testing.T) {
	profile := testProfile(t)
	root := t.TempDir()
	m, err := NewInstanceManager(profile, root)
	require.NoError(t, err)
	input := writeCapture(t, classicCapture(t, 3))
	db, err := m.GetOrCreate(input)
	require.NoError(t, err)
	meta, err := db.Metadata()
	require.NoError(t, err)
	require.NoError(t, m.Close())
	writer, err := openIndex(meta.DatabasePath, true, false)
	require.NoError(t, err)
	meta.State = StateIndexing
	require.NoError(t, writeManifest(context.Background(), writer, meta))
	require.NoError(t, writer.Close())
	recovered, err := NewInstanceManager(profile, root)
	require.NoError(t, err)
	defer recovered.Close()
	require.Equal(t, StateInterrupted, recovered.StartupValidation()[0].Metadata.State)
	db, err = recovered.GetOrCreate(input)
	require.NoError(t, err)
	require.Equal(t, meta.DatasetID, db.ID)
	require.NoError(t, db.Close())
	// A stale catalog is repaired from the committed manifest.
	require.NoError(t, profile.Model(&PCAPFileDBMetadata{}).Where("dataset_id = ?", meta.DatasetID).Update("state", StateImporting).Error)
	validation, err := recovered.Validate(context.Background(), meta.DatasetID, false)
	require.NoError(t, err)
	require.True(t, validation.Valid)
	require.Equal(t, StateReady, validation.Metadata.State)
	writer, err = openIndex(meta.DatabasePath, true, false)
	require.NoError(t, err)
	require.NoError(t, writer.Unscoped().Where("id = ?", 1).Delete(&PCAPCaptureRecord{}).Error)
	require.NoError(t, writer.Close())
	validation, err = recovered.Validate(context.Background(), meta.DatasetID, true)
	require.NoError(t, err)
	require.False(t, validation.Valid)
	require.Equal(t, StateInvalid, validation.Metadata.State)
	_, err = recovered.Open(context.Background(), meta.DatasetID)
	require.ErrorIs(t, err, ErrNotReady)
	_, err = recovered.GetOrCreate(input)
	require.NoError(t, err, "explicit reimport may repair missing BLOB framing")
}

func TestPCAPDBSampleFingerprintDoesNotDeduplicateDifferentContent(t *testing.T) {
	m := testManager(t)
	var b bytes.Buffer
	writer := pcapgo.NewWriter(&b)
	require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
	packet := make([]byte, 64000)
	for b.Len() <= FullHashLimit+1 {
		require.NoError(t, writer.WritePacket(gopacket.CaptureInfo{CaptureLength: len(packet), Length: len(packet)}, packet))
	}
	first := b.Bytes()
	second := bytes.Clone(first)
	second[(1<<20)+100] = 1 // outside the eight sampled windows
	input, changed := writeCapture(t, first), writeCapture(t, second)
	a, err := FingerprintFile(input)
	require.NoError(t, err)
	bfp, err := FingerprintFile(changed)
	require.NoError(t, err)
	require.Equal(t, "sampled-sha256", a.Mode)
	require.Equal(t, a.Hash, bfp.Hash)
	one, err := m.GetOrCreate(input)
	require.NoError(t, err)
	two, err := m.GetOrCreate(changed)
	require.NoError(t, err)
	require.NotEqual(t, one.ID, two.ID)
	count, err := m.Count()
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
}

func TestPCAPDBPCAPNGSectionsInterfacesAndBLOBs(t *testing.T) {
	var capture bytes.Buffer
	packet := dnsPacket(t)
	for section := 0; section < 2; section++ {
		writer, err := pcapgo.NewNgWriterInterface(&capture, pcapgo.NgInterface{Name: "ethernet", LinkType: layers.LinkTypeEthernet, SnapLength: 65535, TimestampResolution: 9}, pcapgo.DefaultNgWriterOptions)
		require.NoError(t, err)
		iface, err := writer.AddInterface(pcapgo.NgInterface{Name: "raw-ip", LinkType: layers.LinkTypeRaw, SnapLength: 65535, TimestampResolution: 9})
		require.NoError(t, err)
		require.NoError(t, writer.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1700000000, 123456789), CaptureLength: len(packet), Length: len(packet)}, packet))
		require.NoError(t, writer.WritePacket(gopacket.CaptureInfo{InterfaceIndex: iface, Timestamp: time.Unix(1700000001, 987654321), CaptureLength: len(packet) - 14, Length: len(packet) - 14}, packet[14:]))
		require.NoError(t, writer.Flush())
	}
	m := testManager(t)
	db, err := m.GetOrCreate(writeCapture(t, capture.Bytes()))
	require.NoError(t, err)
	packets, err := db.QueryPackets()
	require.NoError(t, err)
	require.Len(t, packets, 4)
	for i, p := range packets {
		require.EqualValues(t, i/2, p.Section)
		require.Equal(t, i%2, p.Interface)
		bytes, err := db.ReadPacket(p.ID)
		require.NoError(t, err)
		if i%2 == 0 {
			require.Equal(t, packet, bytes)
		} else {
			require.Equal(t, packet[14:], bytes)
		}
	}
	sessions, err := db.QuerySessions(QueryTransport("udp"))
	require.NoError(t, err)
	require.Len(t, sessions, 4, "identical endpoints on separate sections/interfaces must remain distinct sessions")
	meta, err := db.Metadata()
	require.NoError(t, err)
	require.Equal(t, "pcapng", meta.Format)
	output := filepath.Join(t.TempDir(), "export.pcapng")
	_, err = db.Export(output)
	require.NoError(t, err)
	data, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, capture.Bytes(), data)
}

func TestPCAPDBEmptyCapturesAndMalformedLengths(t *testing.T) {
	m := testManager(t)
	db, err := m.GetOrCreate(writeCapture(t, classicCapture(t, 0)), WithProtocols(true))
	require.NoError(t, err)
	meta, err := db.Metadata()
	require.NoError(t, err)
	require.Zero(t, meta.PacketCount)
	require.True(t, meta.ProtocolsIndexed)
	packets, err := db.QueryPackets()
	require.NoError(t, err)
	require.Empty(t, packets)
	malformed := classicCapture(t, 1)
	binary.LittleEndian.PutUint32(malformed[32:36], 0xffffffff)
	_, err = m.GetOrCreate(writeCapture(t, malformed))
	require.Error(t, err)
	_, err = db.QueryPackets(QueryLimit(1001))
	require.Error(t, err)
	_, err = db.QueryPackets(QuerySourcePort(-1))
	require.Error(t, err)
}

func TestPCAPDBQueryUsesCursorIndex(t *testing.T) {
	m := testManager(t)
	db, err := m.GetOrCreate(writeCapture(t, classicCapture(t, 10)))
	require.NoError(t, err)
	rows, err := db.db.Raw("EXPLAIN QUERY PLAN SELECT id FROM packets WHERE src_ip=? AND id>? ORDER BY id LIMIT ?", "10.1.2.3", 1, 3).Rows()
	require.NoError(t, err)
	defer rows.Close()
	var details []string
	for rows.Next() {
		var a, b, c int
		var detail string
		require.NoError(t, rows.Scan(&a, &b, &c, &detail))
		details = append(details, detail)
	}
	require.NoError(t, rows.Err())
	plan := strings.Join(details, " ")
	require.Contains(t, plan, "packets_src_ip")
	require.NotContains(t, plan, "TEMP B-TREE")
}

func BenchmarkPCAPDBImport(b *testing.B) {
	capture := classicCapture(b, 10000)
	input := writeCapture(b, capture)
	b.SetBytes(int64(len(capture)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		m := testManager(b)
		b.StartTimer()
		db, err := m.GetOrCreate(input)
		require.NoError(b, err)
		require.NoError(b, db.Close())
	}
}
