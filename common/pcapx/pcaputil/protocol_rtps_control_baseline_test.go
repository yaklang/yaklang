package pcaputil

import (
	"bytes"
	"encoding/binary"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRTPSControlDefaultSPDPBoundary(t *testing.T) {
	w := make([]byte, 52)
	copy(w, []byte("RTPS"))
	w[4] = 2
	w[5] = 5
	w[20] = 7
	w[21] = 1
	binary.LittleEndian.PutUint16(w[22:], 28)
	copy(w[24:], []byte{0, 0, 1, 7, 0, 0, 2, 2})
	binary.LittleEndian.PutUint32(w[36:], 1)
	binary.LittleEndian.PutUint32(w[44:], 7)
	binary.LittleEndian.PutUint32(w[48:], 1)
	f, err := decodeRTPSDatagram(w, 32)
	require.Nil(t, f)
	var pe *ProtocolError
	require.True(t, errors.As(err, &pe))
	require.Equal(t, ErrUnsupportedFeature, pe.Kind)
}

func TestRTPSControlExistingDecodeAs(t *testing.T) {
	var selected rtpsControl
	for _, c := range rtpsBatchControls(t, "rtps-controls", 256) {
		if c.ID == "v5-07-le" {
			selected = c
		}
	}
	require.NotEmpty(t, selected.ID)
	raw, want := rtpsInput(t, selected)
	var events []*ProtocolEvent
	err := ReplayPcap(bytes.NewReader(raw), WithProtocolDecodeAs("udp", 28400, "rtps-control"), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }))
	require.NoError(t, err)
	require.Len(t, events, 1)
	f, err := events[0].GetFields()
	require.NoError(t, err)
	rocEqualFields(t, want.Events[0].Fields, f)
	rocEqualFields(t, want.Events[0].Fields, events[0].Session)
	require.Equal(t, "rtps-control-native", events[0].Profile)
	require.Zero(t, events[0].ResponseTo)
	require.Zero(t, events[0].TransactionID)
}

// Document edition2.3 explicitly specifies wire2.4. Use the existing Capture
// API so an unsupported-version runtime refusal is the baseline, not a missing
// new symbol or an inferred document-to-wire version mapping.
func TestRTPSControlWire24ExistingAPI(t *testing.T) {
	var selected rtpsControl
	for _, c := range rtpsBatchControls(t, "rtps-controls", 256) {
		if c.ID == "v4-07-le" {
			selected = c
		}
	}
	require.NotEmpty(t, selected.ID)
	raw, want := rtpsInput(t, selected)
	var events []*ProtocolEvent
	require.NoError(t, ReplayPcap(bytes.NewReader(raw), WithProtocolDecodeAs("udp", 28400, "rtps-control"), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
	require.Len(t, events, 1)
	f, err := events[0].GetFields()
	require.NoError(t, err)
	rocEqualFields(t, want.Events[0].Fields, f)
	rocEqualFields(t, want.Events[0].Fields, events[0].Session)
	require.Zero(t, events[0].ResponseTo)
	require.Zero(t, events[0].TransactionID)
}
