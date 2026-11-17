package yakgrpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestPCAPGRPCQueryPacketsPaginationAndTotal(t *testing.T) {
	_, client := newPCAPTestClient(t)
	path, _, _ := pcapTestCapture(t, 5)
	events := importPCAPForTest(t, client, path)
	datasetID := events[len(events)-1].GetDatasetID()
	ctx := context.Background()

	first, err := client.QueryPCAPPackets(ctx, &ypb.QueryPCAPPacketsRequest{
		DatasetID:  datasetID,
		Pagination: &ypb.Paging{Limit: 2},
	})
	require.NoError(t, err)
	require.Len(t, first.GetData(), 2)
	require.EqualValues(t, []int64{1, 2}, []int64{first.GetData()[0].GetId(), first.GetData()[1].GetId()})
	require.EqualValues(t, 5, first.GetTotal())
	require.True(t, first.GetHasMore())
	require.EqualValues(t, 2, first.GetNextCursor())
	require.Equal(t, "UDP", first.GetData()[0].GetProtocol())
	require.Contains(t, first.GetData()[0].GetInfo(), "DstPort=53")

	second, err := client.QueryPCAPPackets(ctx, &ypb.QueryPCAPPacketsRequest{
		DatasetID:  datasetID,
		Pagination: &ypb.Paging{Limit: 2, AfterId: first.GetNextCursor()},
	})
	require.NoError(t, err)
	require.EqualValues(t, []int64{3, 4}, []int64{second.GetData()[0].GetId(), second.GetData()[1].GetId()})
	require.Nil(t, second.Total, "later cursor pages must make an omitted total explicitly unknown")
	require.True(t, second.GetHasMore())

	recounted, err := client.QueryPCAPPackets(ctx, &ypb.QueryPCAPPacketsRequest{
		DatasetID:    datasetID,
		Pagination:   &ypb.Paging{Limit: 2, AfterId: second.GetNextCursor()},
		IncludeTotal: proto.Bool(true),
	})
	require.NoError(t, err)
	require.EqualValues(t, 5, recounted.GetTotal())
	require.Len(t, recounted.GetData(), 1)
	require.EqualValues(t, 5, recounted.GetData()[0].GetId())
	require.False(t, recounted.GetHasMore())

	skipped, err := client.QueryPCAPPackets(ctx, &ypb.QueryPCAPPacketsRequest{
		DatasetID:    datasetID,
		IncludeTotal: proto.Bool(false),
	})
	require.NoError(t, err)
	require.Nil(t, skipped.Total)
	require.Len(t, skipped.GetData(), 5)
}

func TestPCAPGRPCQueryPacketsFilters(t *testing.T) {
	_, client := newPCAPTestClient(t)
	path, _, _ := pcapTestCapture(t, 5)
	events := importPCAPForTest(t, client, path)
	datasetID := events[len(events)-1].GetDatasetID()
	ctx := context.Background()

	page, err := client.QueryPCAPPackets(ctx, &ypb.QueryPCAPPacketsRequest{
		DatasetID: datasetID,
		Filter: &ypb.PCAPPacketsFilter{
			SourcePort:          []int32{0, 3},
			DestinationPort:     []int32{53},
			TransportLayerTypes: []string{"UDP", "udp"},
			LinkLayerTypes:      []string{"ethernet"},
			NetworkLayerTypes:   []string{"ipv4"},
		},
	})
	require.NoError(t, err)
	require.Len(t, page.GetData(), 2)
	require.EqualValues(t, []int64{1, 4}, []int64{page.GetData()[0].GetId(), page.GetData()[1].GetId()})
	require.EqualValues(t, 2, page.GetTotal())

	byIP, err := client.QueryPCAPPackets(ctx, &ypb.QueryPCAPPacketsRequest{
		DatasetID: datasetID,
		Filter:    &ypb.PCAPPacketsFilter{SourceIP: []string{"10.0.0.3"}},
	})
	require.NoError(t, err)
	require.Len(t, byIP.GetData(), 1)
	require.EqualValues(t, 3, byIP.GetData()[0].GetId())

	byKeyword, err := client.QueryPCAPPackets(ctx, &ypb.QueryPCAPPacketsRequest{
		DatasetID: datasetID,
		Filter:    &ypb.PCAPPacketsFilter{Keyword: "DstPort=53"},
	})
	require.NoError(t, err)
	require.Len(t, byKeyword.GetData(), 5)

	exact, err := client.QueryPCAPPackets(ctx, &ypb.QueryPCAPPacketsRequest{
		DatasetID: datasetID,
		Filter:    &ypb.PCAPPacketsFilter{Id: 3},
	})
	require.NoError(t, err)
	require.Len(t, exact.GetData(), 1)
	require.EqualValues(t, 3, exact.GetData()[0].GetId())
	require.EqualValues(t, 1, exact.GetTotal())

	http, err := client.QueryPCAPPackets(ctx, &ypb.QueryPCAPPacketsRequest{
		DatasetID: datasetID,
		Filter:    &ypb.PCAPPacketsFilter{ApplicationLayerTypes: []string{"http"}},
	})
	require.NoError(t, err)
	require.Empty(t, http.GetData())
	require.EqualValues(t, 0, http.GetTotal())
}

func TestPCAPGRPCQueryPacketsValidation(t *testing.T) {
	_, client := newPCAPTestClient(t)
	datasetID := "00000000-0000-4000-8000-000000000000"
	for _, tc := range []struct {
		req  *ypb.QueryPCAPPacketsRequest
		code codes.Code
	}{
		{&ypb.QueryPCAPPacketsRequest{DatasetID: datasetID, Filter: &ypb.PCAPPacketsFilter{Id: -1}}, codes.InvalidArgument},
		{&ypb.QueryPCAPPacketsRequest{DatasetID: datasetID, Filter: &ypb.PCAPPacketsFilter{SourcePort: []int32{-1}}}, codes.InvalidArgument},
		{&ypb.QueryPCAPPacketsRequest{DatasetID: datasetID, Filter: &ypb.PCAPPacketsFilter{DestinationPort: []int32{65536}}}, codes.InvalidArgument},
		{&ypb.QueryPCAPPacketsRequest{DatasetID: datasetID, Filter: &ypb.PCAPPacketsFilter{SourceIP: []string{"10.0.0.0/24"}}}, codes.InvalidArgument},
		{&ypb.QueryPCAPPacketsRequest{DatasetID: datasetID, Filter: &ypb.PCAPPacketsFilter{NetworkLayerTypes: []string{"ipv5"}}}, codes.InvalidArgument},
		{&ypb.QueryPCAPPacketsRequest{DatasetID: datasetID, Filter: &ypb.PCAPPacketsFilter{LinkLayerTypes: []string{"bogus"}}}, codes.InvalidArgument},
		{&ypb.QueryPCAPPacketsRequest{DatasetID: datasetID, Pagination: &ypb.Paging{Page: 2}}, codes.InvalidArgument},
		{&ypb.QueryPCAPPacketsRequest{DatasetID: datasetID, Pagination: &ypb.Paging{Limit: 1001}}, codes.InvalidArgument},
		{&ypb.QueryPCAPPacketsRequest{DatasetID: "bogus"}, codes.InvalidArgument},
	} {
		_, err := client.QueryPCAPPackets(context.Background(), tc.req)
		require.Equal(t, tc.code, grpcstatus.Code(err), tc.req.String())
	}
}

func TestPCAPGRPCQueryPacketsApplicationCompletion(t *testing.T) {
	_, client := newPCAPTestClient(t)
	path, _, _ := pcapHTTPViewCapture(t)
	events := importPCAPForTest(t, client, path)
	datasetID := events[len(events)-1].GetDatasetID()
	ctx := context.Background()

	first, err := client.QueryPCAPPackets(ctx, &ypb.QueryPCAPPacketsRequest{
		DatasetID:  datasetID,
		Filter:     &ypb.PCAPPacketsFilter{ApplicationLayerTypes: []string{"http"}},
		Pagination: &ypb.Paging{Limit: 1},
	})
	require.NoError(t, err)
	require.EqualValues(t, 2, first.GetTotal())
	require.Len(t, first.GetData(), 1)
	require.EqualValues(t, 4, first.GetData()[0].GetId())
	require.Equal(t, "HTTP", first.GetData()[0].GetProtocol())
	require.Contains(t, first.GetData()[0].GetInfo(), "GET /demo")
	require.True(t, first.GetHasMore())

	second, err := client.QueryPCAPPackets(ctx, &ypb.QueryPCAPPacketsRequest{
		DatasetID:  datasetID,
		Filter:     &ypb.PCAPPacketsFilter{ApplicationLayerTypes: []string{"http"}},
		Pagination: &ypb.Paging{Limit: 1, AfterId: first.GetNextCursor()},
	})
	require.NoError(t, err)
	require.Nil(t, second.Total)
	require.Len(t, second.GetData(), 1)
	require.EqualValues(t, 6, second.GetData()[0].GetId())
	require.Equal(t, "HTTP", second.GetData()[0].GetProtocol())
	require.Contains(t, second.GetData()[0].GetInfo(), "HTTP/1.1 200 OK")
	require.False(t, second.GetHasMore())
}
