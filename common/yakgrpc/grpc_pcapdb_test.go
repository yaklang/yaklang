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
	"google.golang.org/protobuf/proto"
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
	first, err := client.QueryPCAPDatabase(ctx, &ypb.QueryPCAPDatabaseRequest{Pagination: &ypb.Paging{Limit: 1}})
	require.NoError(t, err)
	require.Len(t, first.GetData(), 1)
	require.Equal(t, ids[0], first.GetData()[0].GetDatasetID())
	require.EqualValues(t, 2, first.GetTotal())
	require.True(t, first.GetHasMore())
	require.Equal(t, first.GetData()[0].GetId(), first.GetNextCursor())
	second, err := client.QueryPCAPDatabase(ctx, &ypb.QueryPCAPDatabaseRequest{
		Pagination: &ypb.Paging{Limit: 1, AfterId: first.GetNextCursor()},
	})
	require.NoError(t, err)
	require.Len(t, second.GetData(), 1)
	require.Equal(t, ids[1], second.GetData()[0].GetDatasetID())
	require.EqualValues(t, 2, second.GetTotal())
	require.False(t, second.GetHasMore())
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

func TestPCAPGRPCQueryPacketFullAndPagination(t *testing.T) {
	_, client := newPCAPTestClient(t)
	path, _, raw := pcapTestCapture(t, 5)
	events := importPCAPForTest(t, client, path)
	id := events[len(events)-1].GetDatasetID()
	ctx := context.Background()
	for _, full := range []bool{false, true} {
		var after int64
		var all []*ypb.PCAPPacket
		for {
			page, err := client.QueryPCAPPacket(ctx, &ypb.QueryPCAPPacketRequest{
				DatasetID: id, Full: full, Pagination: &ypb.Paging{Limit: 2, AfterId: after},
			})
			require.NoError(t, err)
			require.NotNil(t, page.Total)
			require.EqualValues(t, 5, page.GetTotal())
			all = append(all, page.GetData()...)
			if !page.GetHasMore() {
				break
			}
			require.Greater(t, page.GetNextCursor(), after)
			after = page.GetNextCursor()
		}
		require.Len(t, all, 5)
		for i, packet := range all {
			require.EqualValues(t, i+1, packet.GetId())
			require.NotNil(t, packet.TimestampNS)
			require.EqualValues(t, 1700000000123456789+int64(i)*int64(time.Second), packet.GetTimestampNS())
			require.EqualValues(t, len(raw[i]), packet.GetCapturedLength())
			require.Equal(t, "udp", packet.GetTransport())
			if full {
				require.Equal(t, raw[i], packet.GetRaw())
			} else {
				require.Empty(t, packet.GetRaw())
			}
		}
	}
	// ID + Full uses the same response/model, and Full is also valid for lists.
	detail, err := client.QueryPCAPPacket(ctx, &ypb.QueryPCAPPacketRequest{
		DatasetID: id, Filter: &ypb.PCAPPacketFilter{Id: 3}, Full: true,
	})
	require.NoError(t, err)
	require.Len(t, detail.GetData(), 1)
	require.Equal(t, raw[2], detail.GetData()[0].GetRaw())
	require.EqualValues(t, 1, detail.GetTotal())
	require.False(t, detail.GetHasMore())
	past, err := client.QueryPCAPPacket(ctx, &ypb.QueryPCAPPacketRequest{
		DatasetID: id, Filter: &ypb.PCAPPacketFilter{Id: 3}, Pagination: &ypb.Paging{AfterId: 3}, Full: true,
	})
	require.NoError(t, err)
	require.Empty(t, past.GetData())
	require.False(t, past.GetHasMore())
}

func TestPCAPGRPCQueryPacketFilters(t *testing.T) {
	_, client := newPCAPTestClient(t)
	path, _, _ := pcapTestCapture(t, 4)
	events := importPCAPForTest(t, client, path)
	id := events[len(events)-1].GetDatasetID()
	ctx := context.Background()
	for _, full := range []bool{false, true} {
		for _, tc := range []struct {
			filter *ypb.PCAPPacketFilter
			ids    []int64
		}{
			{&ypb.PCAPPacketFilter{SourcePort: proto.Int32(0)}, []int64{1}},
			{&ypb.PCAPPacketFilter{SourceIP: "10.0.0.2", DestinationPort: proto.Int32(53), Transport: "UDP"}, []int64{2}},
			{&ypb.PCAPPacketFilter{TimeRange: &ypb.PCAPPacketTimeRange{StartNS: 1700000001123456789, EndNS: 1700000003123456789}}, []int64{2, 3}},
			{&ypb.PCAPPacketFilter{Id: 2, SourceIP: "10.0.0.3"}, nil},
			{&ypb.PCAPPacketFilter{Id: 999}, nil},
			{&ypb.PCAPPacketFilter{Transport: "tcp"}, nil},
		} {
			page, err := client.QueryPCAPPacket(ctx, &ypb.QueryPCAPPacketRequest{DatasetID: id, Filter: tc.filter, Full: full})
			require.NoError(t, err)
			var got []int64
			for _, packet := range page.GetData() {
				got = append(got, packet.GetId())
			}
			require.Equal(t, tc.ids, got)
			if tc.filter.GetId() == 0 {
				require.Nil(t, page.Total, "a filtered query must not claim the dataset's total")
			}
		}
	}
	otherPath, _, _ := pcapTestCapture(t, 1)
	other := importPCAPForTest(t, client, otherPath)
	page, err := client.QueryPCAPPacket(ctx, &ypb.QueryPCAPPacketRequest{
		DatasetID: other[len(other)-1].GetDatasetID(), Filter: &ypb.PCAPPacketFilter{Id: 2}, Full: true,
	})
	require.NoError(t, err)
	require.Empty(t, page.GetData(), "packet IDs belong to their dataset")
}

func TestPCAPGRPCQueryPacketErrors(t *testing.T) {
	_, client := newPCAPTestClient(t)
	id := uuid.NewString()
	for _, filter := range []*ypb.PCAPPacketFilter{
		{Id: -1}, {SourcePort: proto.Int32(-1)}, {DestinationPort: proto.Int32(65536)},
		{SourceIP: "10.0.0.0/24"}, {DestinationIP: "invalid"},
		{TimeRange: &ypb.PCAPPacketTimeRange{StartNS: 2, EndNS: 1}},
	} {
		_, err := client.QueryPCAPPacket(context.Background(), &ypb.QueryPCAPPacketRequest{DatasetID: id, Filter: filter})
		require.Equal(t, codes.InvalidArgument, grpcstatus.Code(err))
	}
	_, err := client.QueryPCAPPacket(context.Background(), &ypb.QueryPCAPPacketRequest{DatasetID: id})
	require.Equal(t, codes.NotFound, grpcstatus.Code(err))
	_, err = client.QueryPCAPPacket(context.Background(), &ypb.QueryPCAPPacketRequest{})
	require.Equal(t, codes.InvalidArgument, grpcstatus.Code(err))
}

func TestPCAPGRPCQueryPacketMaximumPage(t *testing.T) {
	_, client := newPCAPTestClient(t)
	path, _, _ := pcapTestCapture(t, 1001)
	events := importPCAPForTest(t, client, path)
	id := events[len(events)-1].GetDatasetID()
	first, err := client.QueryPCAPPacket(context.Background(), &ypb.QueryPCAPPacketRequest{
		DatasetID: id, Pagination: &ypb.Paging{Limit: 1000},
	})
	require.NoError(t, err)
	require.Len(t, first.GetData(), 1000)
	require.True(t, first.GetHasMore())
	last, err := client.QueryPCAPPacket(context.Background(), &ypb.QueryPCAPPacketRequest{
		DatasetID: id, Pagination: &ypb.Paging{Limit: 1000, AfterId: first.GetNextCursor()},
	})
	require.NoError(t, err)
	require.Len(t, last.GetData(), 1)
	require.EqualValues(t, 1001, last.GetData()[0].GetId())
	require.False(t, last.GetHasMore())
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
	_, err = client.QueryPCAPPacket(ctx, &ypb.QueryPCAPPacketRequest{DatasetID: id})
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
