package yakgrpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestServer_PcapX(t *testing.T) {
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
