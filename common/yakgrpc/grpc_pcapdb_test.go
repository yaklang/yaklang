package yakgrpc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/pcapx/pcapdb"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

func newPCAPTestClient(t *testing.T) (*Server, *Client) {
	t.Helper()
	profile, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "profile.sqlite"))
	require.NoError(t, err)
	profile.DB().SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, profile.Close()) })
	s := &Server{profileDatabase: profile, cacheDir: t.TempDir()}
	client, err := newInMemoryClient(s)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return s, client
}

func pcapTestCapture(t *testing.T, count int) (string, []byte, [][]byte) {
	t.Helper()
	var capture bytes.Buffer
	w := pcapgo.NewWriterNanos(&capture)
	require.NoError(t, w.WriteFileHeader(65535, layers.LinkTypeEthernet))
	var packets [][]byte
	for i := 0; i < count; i++ {
		ethernet := &layers.Ethernet{
			SrcMAC: net.HardwareAddr{1, 2, 3, 4, 5, 6}, DstMAC: net.HardwareAddr{6, 5, 4, 3, 2, 1},
			EthernetType: layers.EthernetTypeIPv4,
		}
		ip := &layers.IPv4{
			Version: 4, TTL: 64, SrcIP: net.IPv4(10, 0, 0, byte(i+1)),
			DstIP: net.IPv4(8, 8, 8, 8), Protocol: layers.IPProtocolUDP,
		}
		udp := &layers.UDP{SrcPort: layers.UDPPort(i), DstPort: 53}
		require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
		buf := gopacket.NewSerializeBuffer()
		require.NoError(t, gopacket.SerializeLayers(buf,
			gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true},
			ethernet, ip, udp, gopacket.Payload([]byte{byte(i), 0xff, 0, 0x7f})))
		raw := append([]byte(nil), buf.Bytes()...)
		packets = append(packets, raw)
		require.NoError(t, w.WritePacket(gopacket.CaptureInfo{
			Timestamp: time.Unix(1700000000+int64(i), 123456789), CaptureLength: len(raw), Length: len(raw),
		}, raw))
	}
	path := filepath.Join(t.TempDir(), "capture.pcap")
	require.NoError(t, os.WriteFile(path, capture.Bytes(), 0600))
	return path, capture.Bytes(), packets
}

func importPCAPForTest(t *testing.T, client ypb.YakClient, path string) []*ypb.ImportPCAPFileResponse {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	stream, err := client.ImportPCAPFile(ctx, &ypb.ImportPCAPFileRequest{InputFile: path})
	require.NoError(t, err)
	var events []*ypb.ImportPCAPFileResponse
	for {
		event, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		events = append(events, event)
	}
	require.NotEmpty(t, events)
	require.True(t, events[len(events)-1].GetFinished())
	return events
}

func TestPCAPGRPCImport(t *testing.T) {
	s, client := newPCAPTestClient(t)
	path, capture, _ := pcapTestCapture(t, 4)
	events := importPCAPForTest(t, client, path)
	require.Greater(t, len(events), 1)
	final := events[len(events)-1]
	require.Equal(t, pcapdb.StateReady, final.GetState())
	require.EqualValues(t, 4, final.GetPacketCount())
	require.EqualValues(t, len(capture), final.GetBytesIndexed())
	require.Equal(t, final.GetDatasetID(), final.GetDatabase().GetDatasetID())
	require.Equal(t, path, final.GetDatabase().GetSourcePath())
	for _, event := range events[:len(events)-1] {
		require.False(t, event.GetFinished())
	}
	// Reuse still has an explicit completion event and the same stable identity.
	repeated := importPCAPForTest(t, client, path)
	require.Equal(t, final.GetDatasetID(), repeated[len(repeated)-1].GetDatasetID())
	manager, err := s.getPCAPManager()
	require.NoError(t, err)
	count, err := manager.CountContext(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	meta, err := manager.Metadata(final.GetDatasetID())
	require.NoError(t, err)
	require.True(t, meta.ProtocolsIndexed)
	require.False(t, meta.StreamsIndexed)
	database := final.GetDatabase()
	require.Equal(t, pcapdb.SourceKindFile, database.GetSourceKind())
	require.Empty(t, database.GetCaptureInterface())
	require.Empty(t, database.GetCaptureBPF())
	require.Equal(t, meta.ProtocolCount, database.GetProtocolCount())
	require.Equal(t, meta.SessionCount, database.GetSessionCount())
	require.Equal(t, meta.StreamCount, database.GetStreamCount())
	require.Equal(t, meta.StreamChunkCount, database.GetStreamChunkCount())
	require.Equal(t, meta.StreamsIndexed, database.GetStreamsIndexed())
	require.Equal(t, meta.CreatedAt.Unix(), database.GetCreatedAt())
	require.Positive(t, database.GetCreatedAt())
	// Import must release its reader lease instead of retaining a native owner.
	deleted, err := manager.Delete(context.Background(), final.GetDatasetID())
	require.NoError(t, err)
	require.True(t, deleted.Deleted)
	require.False(t, deleted.CleanupPending)
}

func TestPCAPGRPCImportInvalidInput(t *testing.T) {
	_, client := newPCAPTestClient(t)
	for _, tc := range []struct {
		path string
		code codes.Code
	}{
		{"", codes.InvalidArgument},
		{t.TempDir(), codes.InvalidArgument},
		{filepath.Join(t.TempDir(), "missing.pcap"), codes.NotFound},
	} {
		stream, err := client.ImportPCAPFile(context.Background(), &ypb.ImportPCAPFileRequest{InputFile: tc.path})
		require.NoError(t, err)
		_, err = stream.Recv()
		require.Equal(t, tc.code, grpcstatus.Code(err))
	}
}

type failingPCAPImportStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *failingPCAPImportStream) Context() context.Context { return s.ctx }
func (s *failingPCAPImportStream) Send(*ypb.ImportPCAPFileResponse) error {
	return grpcstatus.Error(codes.Unavailable, "test receiver disconnected")
}

func TestPCAPGRPCImportSendFailureCancelsWork(t *testing.T) {
	s, client := newPCAPTestClient(t)
	path, _, _ := pcapTestCapture(t, 4)
	err := s.ImportPCAPFile(&ypb.ImportPCAPFileRequest{InputFile: path}, &failingPCAPImportStream{ctx: context.Background()})
	require.Equal(t, codes.Unavailable, grpcstatus.Code(err))
	manager, err := s.getPCAPManager()
	require.NoError(t, err)
	// A failed receiver must not leave a busy writer or publish success.
	rows, err := manager.ListContext(context.Background())
	require.NoError(t, err)
	for _, row := range rows {
		require.NotEqual(t, pcapdb.StateReady, row.State)
	}
	importPCAPForTest(t, client, path)
}

func TestPCAPGRPCQueryDatabase(t *testing.T) {
	s, client := newPCAPTestClient(t)
	ctx := context.Background()
	empty, err := client.QueryPCAPDatabase(ctx, &ypb.QueryPCAPDatabaseRequest{})
	require.NoError(t, err)
	require.Empty(t, empty.GetData())
	require.Zero(t, empty.GetTotal())
	var ids []string
	for _, count := range []int{2, 3} {
		path, _, _ := pcapTestCapture(t, count)
		events := importPCAPForTest(t, client, path)
		ids = append(ids, events[len(events)-1].GetDatasetID())
	}
	manager, err := s.getPCAPManager()
	require.NoError(t, err)
	firstMeta, err := manager.Metadata(ids[0])
	require.NoError(t, err)
	first, err := client.QueryPCAPDatabase(ctx, &ypb.QueryPCAPDatabaseRequest{Pagination: &ypb.Paging{Limit: 1}})
	require.NoError(t, err)
	require.Len(t, first.GetData(), 1)
	require.Equal(t, ids[0], first.GetData()[0].GetDatasetID())
	require.EqualValues(t, 2, first.GetTotal())
	require.Equal(t, first.GetData()[0].GetId(), first.GetPagination().GetAfterId())
	require.Equal(t, firstMeta.SourceKind, first.GetData()[0].GetSourceKind())
	require.Equal(t, firstMeta.CaptureInterface, first.GetData()[0].GetCaptureInterface())
	require.Equal(t, firstMeta.CaptureBPF, first.GetData()[0].GetCaptureBPF())
	require.Equal(t, firstMeta.ProtocolCount, first.GetData()[0].GetProtocolCount())
	require.Equal(t, firstMeta.SessionCount, first.GetData()[0].GetSessionCount())
	require.Equal(t, firstMeta.StreamCount, first.GetData()[0].GetStreamCount())
	require.Equal(t, firstMeta.StreamChunkCount, first.GetData()[0].GetStreamChunkCount())
	require.Equal(t, firstMeta.CreatedAt.Unix(), first.GetData()[0].GetCreatedAt())
	require.Positive(t, first.GetData()[0].GetCreatedAt())
	second, err := client.QueryPCAPDatabase(ctx, &ypb.QueryPCAPDatabaseRequest{
		Pagination: &ypb.Paging{Limit: 1, AfterId: first.GetPagination().GetAfterId()},
	})
	require.NoError(t, err)
	require.Len(t, second.GetData(), 1)
	require.Equal(t, ids[1], second.GetData()[0].GetDatasetID())
	require.EqualValues(t, 2, second.GetTotal())
	require.Equal(t, second.GetData()[0].GetId(), second.GetPagination().GetAfterId())
	exhausted, err := client.QueryPCAPDatabase(ctx, &ypb.QueryPCAPDatabaseRequest{
		Pagination: &ypb.Paging{Limit: 1, AfterId: second.GetPagination().GetAfterId()},
	})
	require.NoError(t, err)
	require.Empty(t, exhausted.GetData())
	require.Equal(t, second.GetPagination().GetAfterId(), exhausted.GetPagination().GetAfterId())
	// Catalog metadata remains queryable when analysis/import is not ready.
	require.NoError(t, s.GetProfileDatabase().Model(&pcapdb.PCAPFileDBMetadata{}).
		Where("dataset_id = ?", ids[0]).Updates(map[string]any{"state": pcapdb.StateFailed, "last_error": "test failure"}).Error)
	detail, err := client.QueryPCAPDatabase(ctx, &ypb.QueryPCAPDatabaseRequest{Filter: &ypb.PCAPDatabaseFilter{DatasetID: ids[0]}})
	require.NoError(t, err)
	require.Len(t, detail.GetData(), 1)
	require.EqualValues(t, 1, detail.GetTotal())
	require.Equal(t, pcapdb.StateFailed, detail.GetData()[0].GetState())
	require.Equal(t, "test failure", detail.GetData()[0].GetLastError())
	missing, err := client.QueryPCAPDatabase(ctx, &ypb.QueryPCAPDatabaseRequest{Filter: &ypb.PCAPDatabaseFilter{DatasetID: uuid.NewString()}})
	require.NoError(t, err)
	require.Empty(t, missing.GetData())
	require.Zero(t, missing.GetTotal())
}

func TestPCAPGRPCQueryDatabaseRejectsUnsupportedParameters(t *testing.T) {
	_, client := newPCAPTestClient(t)
	for _, p := range []*ypb.Paging{
		{Page: 2}, {Limit: -1}, {Limit: 1001}, {AfterId: -1}, {BeforeId: 1},
		{Order: "desc"}, {OrderBy: "source_path"}, {RawOrder: "id desc"},
	} {
		_, err := client.QueryPCAPDatabase(context.Background(), &ypb.QueryPCAPDatabaseRequest{Pagination: p})
		require.Equal(t, codes.InvalidArgument, grpcstatus.Code(err))
	}
	_, err := client.QueryPCAPDatabase(context.Background(), &ypb.QueryPCAPDatabaseRequest{
		Filter: &ypb.PCAPDatabaseFilter{DatasetID: "/path/is/not/a/dataset/id"},
	})
	require.Equal(t, codes.InvalidArgument, grpcstatus.Code(err))
}

func TestPCAPGRPCExport(t *testing.T) {
	_, client := newPCAPTestClient(t)
	input, capture, packets := pcapTestCapture(t, 3)
	events := importPCAPForTest(t, client, input)
	id := events[len(events)-1].GetDatasetID()
	require.NoError(t, os.Remove(input))
	for _, tc := range []struct {
		packetID int64
		data     []byte
	}{
		{0, capture}, {2, packets[1]},
	} {
		output := filepath.Join(t.TempDir(), "export.bin")
		exported, err := client.ExportPCAPData(context.Background(), &ypb.ExportPCAPDataRequest{
			DatasetID: id, TargetPath: output, PacketID: tc.packetID,
		})
		require.NoError(t, err)
		require.Equal(t, output, exported.GetPath())
		require.EqualValues(t, len(tc.data), exported.GetBytes())
		digest := sha256.Sum256(tc.data)
		require.Equal(t, hex.EncodeToString(digest[:]), exported.GetSHA256())
		actual, err := os.ReadFile(output)
		require.NoError(t, err)
		require.Equal(t, tc.data, actual)
		_, err = client.ExportPCAPData(context.Background(), &ypb.ExportPCAPDataRequest{
			DatasetID: id, TargetPath: output, PacketID: tc.packetID,
		})
		require.Equal(t, codes.AlreadyExists, grpcstatus.Code(err))
		actual, err = os.ReadFile(output)
		require.NoError(t, err)
		require.Equal(t, tc.data, actual, "export must not overwrite the existing output")
	}
	missingOutput := filepath.Join(t.TempDir(), "missing.bin")
	_, err := client.ExportPCAPData(context.Background(), &ypb.ExportPCAPDataRequest{
		DatasetID: id, TargetPath: missingOutput, PacketID: 999,
	})
	require.Equal(t, codes.NotFound, grpcstatus.Code(err))
	require.NoFileExists(t, missingOutput)
}

func TestPCAPGRPCExportPCAPNG(t *testing.T) {
	_, client := newPCAPTestClient(t)
	_, _, packets := pcapTestCapture(t, 1)
	var capture bytes.Buffer
	w, err := pcapgo.NewNgWriter(&capture, layers.LinkTypeEthernet)
	require.NoError(t, err)
	require.NoError(t, w.WritePacket(gopacket.CaptureInfo{
		Timestamp: time.Unix(1700000000, 123456789), CaptureLength: len(packets[0]), Length: len(packets[0]),
	}, packets[0]))
	require.NoError(t, w.Flush())
	input := filepath.Join(t.TempDir(), "input.pcapng")
	require.NoError(t, os.WriteFile(input, capture.Bytes(), 0600))
	events := importPCAPForTest(t, client, input)
	final := events[len(events)-1]
	require.Equal(t, "pcapng", final.GetDatabase().GetFormat())
	output := filepath.Join(t.TempDir(), "output.pcapng")
	_, err = client.ExportPCAPData(context.Background(), &ypb.ExportPCAPDataRequest{DatasetID: final.GetDatasetID(), TargetPath: output})
	require.NoError(t, err)
	actual, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, capture.Bytes(), actual, "PCAPNG headers and options must survive export exactly")
}

func TestPCAPGRPCExportInvalidInput(t *testing.T) {
	_, client := newPCAPTestClient(t)
	for _, req := range []*ypb.ExportPCAPDataRequest{
		{}, {DatasetID: uuid.NewString()},
		{DatasetID: uuid.NewString(), TargetPath: filepath.Join(t.TempDir(), "out"), PacketID: -1},
	} {
		_, err := client.ExportPCAPData(context.Background(), req)
		require.Equal(t, codes.InvalidArgument, grpcstatus.Code(err))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.ExportPCAPData(ctx, &ypb.ExportPCAPDataRequest{
		DatasetID: uuid.NewString(), TargetPath: filepath.Join(t.TempDir(), "canceled.pcap"),
	})
	require.Equal(t, codes.Canceled, grpcstatus.Code(err))
}

func TestPCAPGRPCDelete(t *testing.T) {
	s, client := newPCAPTestClient(t)
	input, capture, _ := pcapTestCapture(t, 3)
	events := importPCAPForTest(t, client, input)
	id := events[len(events)-1].GetDatasetID()
	ctx := context.Background()
	output := filepath.Join(t.TempDir(), "keep-export.pcap")
	_, err := client.ExportPCAPData(ctx, &ypb.ExportPCAPDataRequest{DatasetID: id, TargetPath: output})
	require.NoError(t, err)
	manager, err := s.getPCAPManager()
	require.NoError(t, err)
	reader, err := manager.Acquire(ctx, id)
	require.NoError(t, err)
	_, err = client.DeletePCAPDatabase(ctx, &ypb.DeletePCAPDatabaseRequest{DatasetID: id})
	require.Equal(t, codes.Aborted, grpcstatus.Code(err), "active readers must prevent deletion")
	_, err = reader.ReadPacket(1)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	deleted, err := client.DeletePCAPDatabase(ctx, &ypb.DeletePCAPDatabaseRequest{DatasetID: id})
	require.NoError(t, err)
	require.True(t, deleted.GetDeleted())
	require.False(t, deleted.GetCleanupPending())
	require.Empty(t, deleted.GetCleanupError())
	require.EqualValues(t, 1, deleted.GetMessage().GetEffectRows())
	require.Equal(t, DbOperationDelete, deleted.GetMessage().GetOperation())
	for _, path := range []string{input, output} {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, capture, data)
	}
	listed, err := client.QueryPCAPDatabase(ctx, &ypb.QueryPCAPDatabaseRequest{})
	require.NoError(t, err)
	require.Zero(t, listed.GetTotal())
	_, err = client.QueryPCAPPackets(ctx, &ypb.QueryPCAPPacketsRequest{DatasetID: id})
	require.Equal(t, codes.NotFound, grpcstatus.Code(err))
	repeated, err := client.DeletePCAPDatabase(ctx, &ypb.DeletePCAPDatabaseRequest{DatasetID: id})
	require.NoError(t, err)
	require.True(t, repeated.GetDeleted())
	require.Zero(t, repeated.GetMessage().GetEffectRows())
	fresh := importPCAPForTest(t, client, input)
	require.NotEqual(t, id, fresh[len(fresh)-1].GetDatasetID(), "deletion must release the content deduplication key")
}

func TestPCAPGRPCDeleteCleanupRetry(t *testing.T) {
	s, client := newPCAPTestClient(t)
	input, _, _ := pcapTestCapture(t, 1)
	events := importPCAPForTest(t, client, input)
	id := events[len(events)-1].GetDatasetID()
	manager, err := s.getPCAPManager()
	require.NoError(t, err)
	meta, err := manager.Metadata(id)
	require.NoError(t, err)
	// A nonempty directory at a sidecar path makes cleanup fail after the
	// catalog transaction, without changing permissions or affecting the source.
	journal := meta.DatabasePath + "-journal"
	require.NoError(t, os.Mkdir(journal, 0700))
	blocker := filepath.Join(journal, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("test"), 0600))
	first, err := client.DeletePCAPDatabase(context.Background(), &ypb.DeletePCAPDatabaseRequest{DatasetID: id})
	require.NoError(t, err)
	require.True(t, first.GetDeleted())
	require.True(t, first.GetCleanupPending())
	require.NotEmpty(t, first.GetCleanupError())
	require.EqualValues(t, 1, first.GetMessage().GetEffectRows())
	require.NoError(t, os.Remove(blocker))
	second, err := client.DeletePCAPDatabase(context.Background(), &ypb.DeletePCAPDatabaseRequest{DatasetID: id})
	require.NoError(t, err)
	require.True(t, second.GetDeleted())
	require.False(t, second.GetCleanupPending())
	require.Empty(t, second.GetCleanupError())
	require.Zero(t, second.GetMessage().GetEffectRows())
	require.NoFileExists(t, meta.DatabasePath)
	require.FileExists(t, input)
}

func TestPCAPGRPCDeleteInvalidInput(t *testing.T) {
	_, client := newPCAPTestClient(t)
	for _, id := range []string{"", "1", "/tmp/capture.pcap"} {
		_, err := client.DeletePCAPDatabase(context.Background(), &ypb.DeletePCAPDatabaseRequest{DatasetID: id})
		require.Equal(t, codes.InvalidArgument, grpcstatus.Code(err))
	}
	_, err := client.DeletePCAPDatabase(context.Background(), &ypb.DeletePCAPDatabaseRequest{DatasetID: uuid.NewString()})
	require.Equal(t, codes.NotFound, grpcstatus.Code(err))
}
