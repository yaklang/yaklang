package yakgrpc

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcapdb"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

func TestPCAPGRPCReadPacketBytes(t *testing.T) {
	_, client := newPCAPTestClient(t)
	path, _, raw := pcapTestCapture(t, 2)
	events := importPCAPForTest(t, client, path)
	datasetID := events[len(events)-1].GetDatasetID()
	ctx := context.Background()

	first, err := client.ReadPCAPBytes(ctx, &ypb.ReadPCAPBytesRequest{
		DatasetID: datasetID, SourceID: "frame:1", Offset: 0, Length: 10,
	})
	require.NoError(t, err)
	require.Equal(t, raw[0][:10], first.GetData())
	require.EqualValues(t, len(raw[0]), first.GetTotalLength())
	require.False(t, first.GetEOF())

	middleOffset := int64(len(raw[1]) - 7)
	middle, err := client.ReadPCAPBytes(ctx, &ypb.ReadPCAPBytesRequest{
		DatasetID: datasetID, SourceID: "frame:2", Offset: middleOffset, Length: 100,
	})
	require.NoError(t, err)
	require.Equal(t, raw[1][middleOffset:], middle.GetData())
	require.True(t, middle.GetEOF())

	end, err := client.ReadPCAPBytes(ctx, &ypb.ReadPCAPBytesRequest{
		DatasetID: datasetID, SourceID: "frame:2", Offset: int64(len(raw[1])), Length: 1,
	})
	require.NoError(t, err)
	require.Empty(t, end.GetData())
	require.True(t, end.GetEOF())
}

func TestPCAPGRPCReadReassembledMessageBytes(t *testing.T) {
	s, client := newPCAPTestClient(t)
	path, _, response := pcapHTTPViewCapture(t)
	events := importPCAPForTest(t, client, path)
	datasetID := events[len(events)-1].GetDatasetID()
	manager, err := s.getPCAPManager()
	require.NoError(t, err)
	handle, err := manager.Acquire(context.Background(), datasetID)
	require.NoError(t, err)
	messages, err := handle.PacketMessages(6, 1<<20)
	require.NoError(t, err)
	require.NoError(t, handle.Close())
	var messageID uint
	for _, message := range messages {
		if bytes.Equal(message.Data, response) {
			messageID = message.ID
			break
		}
	}
	require.NotZero(t, messageID)

	sourceID := fmt.Sprintf("message:%d", messageID)
	var reassembled []byte
	for offset := 0; offset < len(response); offset += 17 {
		length := min(17, len(response)-offset)
		chunk, err := client.ReadPCAPBytes(context.Background(), &ypb.ReadPCAPBytesRequest{
			DatasetID: datasetID, SourceID: sourceID, Offset: int64(offset), Length: int64(length),
		})
		require.NoError(t, err)
		require.Equal(t, offset, int(chunk.GetOffset()))
		require.EqualValues(t, len(response), chunk.GetTotalLength())
		require.Equal(t, response[offset:offset+length], chunk.GetData())
		require.Equal(t, offset+length == len(response), chunk.GetEOF())
		reassembled = append(reassembled, chunk.GetData()...)
	}
	require.Equal(t, response, reassembled)
}

func TestPCAPGRPCReadBytesValidation(t *testing.T) {
	_, client := newPCAPTestClient(t)
	path, _, raw := pcapTestCapture(t, 1)
	events := importPCAPForTest(t, client, path)
	datasetID := events[len(events)-1].GetDatasetID()
	for _, tc := range []struct {
		req  *ypb.ReadPCAPBytesRequest
		code codes.Code
	}{
		{&ypb.ReadPCAPBytesRequest{DatasetID: datasetID, SourceID: "packet:1", Length: 1}, codes.InvalidArgument},
		{&ypb.ReadPCAPBytesRequest{DatasetID: datasetID, SourceID: "frame:0", Length: 1}, codes.InvalidArgument},
		{&ypb.ReadPCAPBytesRequest{DatasetID: datasetID, SourceID: "frame:1", Offset: -1, Length: 1}, codes.InvalidArgument},
		{&ypb.ReadPCAPBytesRequest{DatasetID: datasetID, SourceID: "frame:1", Length: 0}, codes.InvalidArgument},
		{&ypb.ReadPCAPBytesRequest{DatasetID: datasetID, SourceID: "frame:1", Length: pcapdb.MaxByteReadLength + 1}, codes.InvalidArgument},
		{&ypb.ReadPCAPBytesRequest{DatasetID: datasetID, SourceID: "frame:99", Length: 1}, codes.NotFound},
		{&ypb.ReadPCAPBytesRequest{DatasetID: datasetID, SourceID: "frame:1", Offset: int64(len(raw[0]) + 1), Length: 1}, codes.OutOfRange},
		{&ypb.ReadPCAPBytesRequest{DatasetID: datasetID, SourceID: "message:99999", Length: 1}, codes.NotFound},
	} {
		_, err := client.ReadPCAPBytes(context.Background(), tc.req)
		require.Equal(t, tc.code, grpcstatus.Code(err), tc.req.String())
	}
}
