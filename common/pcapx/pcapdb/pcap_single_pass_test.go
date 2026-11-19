package pcapdb

import (
	"bytes"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

func TestPCAPDBSinglePassLinksProtocolAcrossCommits(t *testing.T) {
	request := []byte("GET /single-pass HTTP/1.1\r\nHost: pcapdb.example\r\n\r\n")
	packets := [][]byte{
		tcpPacket(t, 100, 0, "S", nil, false),
		tcpPacket(t, 500, 101, "SA", nil, true),
		tcpPacket(t, 101, 501, "A", nil, false),
		tcpPacket(t, 101, 501, "PA", request[:12], false),
		tcpPacket(t, 113, 501, "PA", request[12:], false),
	}
	var capture bytes.Buffer
	writer := pcapgo.NewWriterNanos(&capture)
	require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
	for i, packet := range packets {
		require.NoError(t, writer.WritePacket(gopacket.CaptureInfo{
			Timestamp: time.Unix(1700000000+int64(i), 0), CaptureLength: len(packet), Length: len(packet),
		}, packet))
	}

	m := testManager(t)
	var states []State
	db, err := m.GetOrCreate(writeCapture(t, capture.Bytes()),
		WithProtocols(true), WithStreams(false), WithBatchSize(1),
		WithProgress(func(progress Progress) { states = append(states, progress.State) }),
	)
	require.NoError(t, err)
	defer db.Close()
	require.NotContains(t, states, StateAnalyzing, "new imports must not run a second replay phase")

	meta, err := db.Metadata()
	require.NoError(t, err)
	require.Equal(t, StateReady, meta.State)
	require.EqualValues(t, len(packets), meta.PacketCount)
	require.EqualValues(t, 1, meta.ProtocolCount)
	messages, err := db.QueryProtocols(QueryProtocol("http"), QueryWithFields(true))
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.NotEmpty(t, messages[0].Fields)

	refs, err := db.ProtocolPacketIDs(messages[0].ID)
	require.NoError(t, err)
	require.Equal(t, []int64{4, 5}, refs)
	require.EqualValues(t, 5, messages[0].CompletedInPacketID)

	var violations int64
	require.NoError(t, db.db.Raw("SELECT count(*) FROM pragma_foreign_key_check").Row().Scan(&violations))
	require.Zero(t, violations)
}
