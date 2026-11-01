package pcaputil

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestNativeL2AndNetworkPDULengthMatchesObservedBytes(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		for _, tc := range []struct {
			protocol string
			frame    []byte
		}{
			{"stp", stpNativeFrame(stpNativeWire(), false, true)},
			{"isis", isisNativeEthernet(isisNativeWire(t, "831401001101000003010001000100001e001401"), 1)},
			{"pim", pimControlFrame(t, []byte{0x20, 0, 0, 0, 0, 1, 0, 2, 0, 105}, false)},
		} {
			t.Run(fmt.Sprintf("%s/deferred%t", tc.protocol, deferred), func(t *testing.T) {
				var events []*ProtocolEvent
				a, err := NewPacketAnalyzer(WithProtocolDeferred(deferred), WithCaptureReplayClock(), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }))
				require.NoError(t, err)
				require.NoError(t, a.Feed(tc.frame, gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), CaptureLength: len(tc.frame), Length: len(tc.frame)}, layers.LinkTypeEthernet))
				require.NoError(t, a.Close())
				require.Len(t, events, 1)
				e := events[0]
				require.Equal(t, tc.protocol, e.Protocol)
				require.NotEmpty(t, e.Raw)
				require.Equal(t, len(e.Raw), e.Length, "logical PDU excludes carrier and padding")
				f, err := CompileDisplayFilter(fmt.Sprintf("pdu.length == %d", len(e.Raw)))
				require.NoError(t, err)
				require.True(t, f.Match(e))
				wire, err := json.Marshal(e.DisplayFieldValues())
				require.NoError(t, err)
				var snapshot DisplayFieldSnapshot
				require.NoError(t, json.Unmarshal(wire, &snapshot))
				require.True(t, f.EvaluateSnapshots([]DisplayFieldSnapshot{snapshot}).Matched)
			})
		}
	}
}

func TestDatagramCompletionKeepsExplicitOmittedAndPreviewLengths(t *testing.T) {
	for _, preview := range [][]byte{nil, {1, 2, 3}} {
		a := &binParser{config: BinParserConfig{Deferred: true}}
		e := &ProtocolEvent{Length: 65536}
		a.finishProtocolDatagram(e, preview, nil, protocolError(ErrResourceExceeded, "body omitted"))
		require.Equal(t, 65536, e.Length)
		require.Equal(t, preview, e.Raw)
		require.Equal(t, "limited", e.Status)
	}
}
