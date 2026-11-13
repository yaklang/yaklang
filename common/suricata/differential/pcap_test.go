package differential

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

func TestUDPFlowAndPCAPAreDeterministic(t *testing.T) {
	flow := DNSResponseFirstFlow()
	first, err := flow.Frames()
	require.NoError(t, err)
	second, err := flow.Frames()
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Len(t, first, 1)

	path := filepath.Join(t.TempDir(), "dns.pcap")
	require.NoError(t, WritePCAP(path, first))
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()
	reader, err := pcapgo.NewReader(file)
	require.NoError(t, err)
	data, _, err := reader.ReadPacketData()
	require.NoError(t, err)
	require.NotEmpty(t, data)
}

func TestTCPFlowAndPCAPAreDeterministic(t *testing.T) {
	flow := NewTCPFlow(40000, 80, []byte("GET / HTTP/1.1\r\n"), []byte("Host: example.test\r\n\r\n"))
	first, err := flow.Frames()
	require.NoError(t, err)
	second, err := flow.Frames()
	require.NoError(t, err)
	require.Equal(t, first, second)

	path := filepath.Join(t.TempDir(), "flow.pcap")
	require.NoError(t, WritePCAP(path, first))

	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()
	reader, err := pcapgo.NewReader(file)
	require.NoError(t, err)
	for range first {
		_, _, err := reader.ReadPacketData()
		require.NoError(t, err)
	}
	_, _, err = reader.ReadPacketData()
	require.Error(t, err)
}
