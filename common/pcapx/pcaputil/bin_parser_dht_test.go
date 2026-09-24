package pcaputil

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDHTKRPCWireAdmission(t *testing.T) {
	request := []byte("d1:ad2:id20:winlab-node-5013!!!!e1:q4:ping1:t2:aa1:y1:qe")
	fields, err := decodeDHTMessage(request, 32)
	require.NoError(t, err)
	require.Equal(t, "ping", fields["Packet Name"])
	require.Equal(t, []byte("winlab-node-5013!!!!"), fields["Node ID"])

	for _, wire := range [][]byte{
		nil,
		request[:len(request)-1],          // incomplete bencode
		append(bytes.Clone(request), 'x'), // trailing bytes
		bytes.Replace(bytes.Clone(request), []byte("20:"), []byte("19:"), 1), // wrong node ID length
		bytes.Replace(bytes.Clone(request), []byte("4:ping"), []byte("4:pong"), 1),
		bytes.Replace(bytes.Clone(request), []byte("1:y1:q"), []byte("1:y1:x"), 1),
		bytes.Replace(bytes.Clone(request), []byte("1:t2:aa"), []byte("1:t0:"), 1),
		[]byte("d1:ad2:id20:winlab-node-5013!!!!e1:q4:ping1:q4:ping1:t2:aa1:y1:qe"), // duplicate key
	} {
		require.Falsef(t, validDHTMessage(wire), "%q", wire)
	}
	require.False(t, validDHTMessage(request[:len(request)-2]))
	_, err = decodeDHTMessage(request, 2)
	require.ErrorContains(t, err, "budget")
}

func TestDHTKRPCErrorResponseFromBEP5(t *testing.T) {
	// BEP 5 publishes this bencoded error packet as a wire example.
	wire := []byte("d1:eli201e23:A Generic Error Ocurrede1:t2:aa1:y1:ee")
	fields, err := decodeDHTMessage(wire, 32)
	require.NoError(t, err)
	require.Equal(t, "error", fields["Packet Name"])
	require.Equal(t, int64(201), fields["Error Code"])
	require.Equal(t, "A Generic Error Ocurred", fields["Error Message"])
	for _, invalid := range [][]byte{
		bytes.Replace(bytes.Clone(wire), []byte("i201e"), []byte("i205e"), 1),
		bytes.Replace(bytes.Clone(wire), []byte("23:"), []byte("22:"), 1),
		bytes.Replace(bytes.Clone(wire), []byte("1:y1:e"), []byte("1:y1:r"), 1),
	} {
		require.False(t, validDHTMessage(invalid))
	}
}

func TestDHTCompactPeersFromBEP5(t *testing.T) {
	// A get_peers response may carry a list of six-byte compact IPv4 peers.
	wire := []byte("d1:rd2:id20:mnopqrstuvwxyz1234566:valuesl6:abcdefee1:t2:aa1:y1:re")
	fields, err := decodeDHTMessage(wire, 32)
	require.NoError(t, err)
	require.Equal(t, 1, fields["Peer Count"])
	badPeer := bytes.Replace(bytes.Clone(wire), []byte("6:abcdef"), []byte("5:abcde"), 1)
	require.False(t, validDHTMessage(badPeer))
}

func TestWinlab5013DHTEightDatagrams(t *testing.T) {
	events := replayWinlab5013Protocols(t, "16-bittorrent-dht.pcapng")
	require.Len(t, events, 8)
	for i, event := range events {
		require.Equal(t, "bittorrent-dht", event.Protocol, "datagram %d", i)
		require.Equal(t, "decoded", event.Status, "datagram %d: %s", i, event.Error)
	}
	require.Equal(t, "ping", events[0].Fields["Packet Name"])
	require.Equal(t, []byte("winlab-node-5013!!!!"), events[0].Fields["Node ID"])
	require.Equal(t, "find_node", events[2].Fields["Packet Name"])
	require.Equal(t, []byte("target-node-5013!!!!"), events[2].Fields["Target"])
	require.Equal(t, 26, events[3].Fields["Nodes Length"])
	require.Equal(t, "198.51.100.9", events[3].Fields["First Node IP"])
	require.Equal(t, uint16(6881), events[3].Fields["First Node Port"])
	require.Equal(t, "get_peers", events[4].Fields["Packet Name"])
	require.Equal(t, []byte("lab"), events[5].Fields["Token"])
	require.Equal(t, "announce_peer", events[6].Fields["Packet Name"])
	require.Equal(t, int64(6881), events[6].Fields["Port"])
}
