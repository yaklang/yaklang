package yakgrpc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcapdb"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestServerGetPcapMetadata(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	rsp, err := client.GetPcapMetadata(context.Background(), &ypb.PcapMetadataRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, rsp.GetAvailablePcapDevices(), "offline hosts still expose their loopback device")
	require.NotEmpty(t, rsp.GetAvailableSessionTypes())
	require.NotEmpty(t, rsp.GetAvailableLinkLayerTypes())
	require.NotEmpty(t, rsp.GetAvailableNetworkLayerTypes())
	require.NotEmpty(t, rsp.GetAvailableTransportLayerTypes())
}

func TestPcapXLiveCaptureConfig(t *testing.T) {
	_, err := pcapXLiveCaptureConfig(&ypb.PcapXRequest{})
	require.Error(t, err)
	_, err = pcapXLiveCaptureConfig(&ypb.PcapXRequest{NetInterfaceList: []string{"eth0", "eth1"}})
	require.Error(t, err)

	config, err := pcapXLiveCaptureConfig(&ypb.PcapXRequest{
		NetInterfaceList:          []string{" eth0 "},
		BPF:                       " tcp port 80 ",
		Snaplen:                   4096,
		Promisc:                   true,
		BatchSize:                 200,
		FlushIntervalMilliseconds: 100,
		DurationMilliseconds:      1500,
	})
	require.NoError(t, err)
	require.Equal(t, pcapdb.LiveCaptureConfig{
		Interface: "eth0", BPF: "tcp port 80", Snaplen: 4096, Promisc: true,
		BatchSize: 200, FlushInterval: 100 * time.Millisecond, Duration: 1500 * time.Millisecond,
	}, config)
}

func TestPcapXStreamsLiveDataset(t *testing.T) {
	_, client := newPCAPTestClient(t)
	original := pcapXRunLiveCapture
	t.Cleanup(func() { pcapXRunLiveCapture = original })

	datasetID := uuid.NewString()
	initial := pcapdb.PCAPFileDBMetadata{
		DatasetID: datasetID, State: pcapdb.StateReady,
		PacketCount: 2, ProtocolCount: 1, SessionCount: 1, StreamCount: 2, StreamChunkCount: 3,
		BytesIndexed: 2048, SourceSize: 4096, SourceKind: pcapdb.SourceKindLive,
		CaptureInterface: "controlled", CaptureBPF: "tcp port 80",
	}
	initial.CreatedAt = time.Unix(1700000000, 0).UTC()
	pcapXRunLiveCapture = func(ctx context.Context, _ *pcapdb.InstanceManager, _ pcapdb.LiveCaptureConfig, started func(pcapdb.PCAPFileDBMetadata)) (*pcapdb.PCAPFileDBMetadata, error) {
		started(initial)
		<-ctx.Done()
		final := initial
		final.PacketCount, final.ProtocolCount = 5, 4
		final.BytesIndexed = final.SourceSize
		return &final, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := client.PcapX(ctx)
	require.NoError(t, err)
	require.NoError(t, stream.Send(&ypb.PcapXRequest{NetInterfaceList: []string{"controlled"}}))
	first, err := stream.Recv()
	require.NoError(t, err)
	require.EqualValues(t, 2, first.GetPacketFrameCounter())
	require.EqualValues(t, 3, first.GetTCPReassembledCount())
	require.EqualValues(t, 1, first.GetTrafficSessionCount())
	require.Equal(t, datasetID, first.GetDatabase().GetDatasetID())
	require.Equal(t, string(pcapdb.StateReady), first.GetDatabase().GetState())
	require.EqualValues(t, 2, first.GetDatabase().GetPacketCount())
	require.EqualValues(t, 1, first.GetDatabase().GetProtocolCount())
	require.False(t, first.GetFinished())
	require.Equal(t, pcapdb.SourceKindLive, first.GetDatabase().GetSourceKind())
	require.Equal(t, "controlled", first.GetDatabase().GetCaptureInterface())
	require.Equal(t, "tcp port 80", first.GetDatabase().GetCaptureBPF())
	require.EqualValues(t, initial.CreatedAt.Unix(), first.GetDatabase().GetCreatedAt())

	cancel()
	require.Eventually(t, func() bool {
		_, err := stream.Recv()
		return err != nil
	}, time.Second, 10*time.Millisecond)
}
