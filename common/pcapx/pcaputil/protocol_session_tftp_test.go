package pcaputil

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTFTPTransferPortsAndOptions(t *testing.T) {
	c := NewDefaultConfig()
	require.NoError(t, WithBinParserConfig(BinParserConfig{OnEvent: func(*ProtocolEvent) {}})(c))
	require.NoError(t, c.prepareBinParser())
	a := c.binParser
	feed := func(src, dst string, w []byte) *ProtocolEvent {
		e := &ProtocolEvent{Source: src, Destination: dst, Transport: "udp", Length: len(w), Timestamp: time.Unix(10, 0)}
		require.True(t, a.decodeTFTPDatagram(e, w))
		return e
	}
	r := feed("192.0.2.1:40000", "192.0.2.2:69", []byte("\x00\x01test\x00octet\x00blksize\x001024\x00"))
	require.Empty(t, r.Error)
	o := feed("192.0.2.2:50000", "192.0.2.1:40000", []byte("\x00\x06blksize\x001024\x00"))
	require.Empty(t, o.Error)
	require.Equal(t, r.FlowID, o.FlowID)
	ack := feed("192.0.2.1:40000", "192.0.2.2:50000", []byte{0, 4, 0, 0})
	require.Empty(t, ack.Error)
	bad := feed("192.0.2.2:50001", "192.0.2.1:40000", []byte{0, 3, 0, 1, 42})
	require.Equal(t, "context-required", bad.Status)
	data := feed("192.0.2.2:50000", "192.0.2.1:40000", []byte{0, 3, 0, 1, 42})
	require.Empty(t, data.Error)
	ack = feed("192.0.2.1:40000", "192.0.2.2:50000", []byte{0, 4, 0, 1})
	require.Empty(t, ack.Error)
	require.Equal(t, true, ack.Session["Transfer Complete"])
	require.NoError(t, c.finishBinParser())
	require.Zero(t, a.stats().BufferedBytes)
}
func TestTFTPRejectMalformedAndWrongAssociation(t *testing.T) {
	s := &binTFTP{}
	_, err := s.consume(0, []byte("\x00\x01test\x00octet\x00"), 8)
	require.NoError(t, err)
	for _, tc := range []struct {
		dir int
		w   []byte
	}{{1, []byte{0, 3, 0, 2, 42}}, {0, []byte{0, 3, 0, 1, 42}}, {0, []byte{0, 4, 0, 1}}, {1, []byte("\x00\x06blksize\x001024\x00")}, {1, []byte{0, 5, 0, 1, 42}}} {
		_, err = s.consume(tc.dir, tc.w, 8)
		require.Error(t, err)
	}
	_, err = s.consume(1, []byte{0, 3, 0, 1, 42}, 8)
	require.NoError(t, err)
	_, err = s.consume(1, []byte{0, 3, 0, 1, 43}, 8)
	require.Error(t, err)
	_, err = s.consume(0, []byte{0, 4, 0, 1}, 8)
	require.NoError(t, err)
	require.True(t, s.done)
}
func TestTFTPUpstreamCaptureReplay(t *testing.T) {
	// Original nDPI TFTP transfer, provenance/hash in the shared corpus manifest.
	for _, workers := range []int{1, 2} {
		t.Run(fmt.Sprint(workers), func(t *testing.T) {
			events, stats, err := binReplay(t, binCorpusBytes(t, "ndpi/ndpi-tftp.pcap"), workers)
			require.NoError(t, err)
			counts := map[string]int{}
			complete := 0
			for _, e := range events {
				if e.Protocol == "tftp" {
					counts[e.Status]++
					if e.Error != "" {
						t.Log(e.Error)
					}
					if e.Session["Transfer Complete"] == true {
						complete++
					}
				}
			}
			require.Positive(t, counts["decoded"])
			require.Positive(t, complete)
			require.Zero(t, stats.BufferedBytes)
			t.Logf("counts %v complete %d", counts, complete)
		})
	}
}
