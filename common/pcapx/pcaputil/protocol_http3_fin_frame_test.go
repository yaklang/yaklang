package pcaputil

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func h3FINFrameFeed(q *binQUIC, dir int, frames ...map[string]any) (map[string]any, error) {
	q.eventID++
	q.eventRefs = []PacketReference{{Number: q.eventID}}
	return q.finishFrames(dir, quicSpaceApplication, frames, map[string]any{}, 1024)
}

func h3FINStream(sid, off uint64, wire []byte, fin bool) map[string]any {
	return map[string]any{"Frame Type": "STREAM", "Stream ID": sid, "Offset": off, "Stream Data": wire, "FIN": fin}
}

func TestHTTP3DrainedFINTruncatedFrameIsConnectionLayoutError(t *testing.T) {
	for _, tt := range []struct {
		name string
		wire []byte
	}{
		{"type-varint", append(h3RequestHeaders(), 0x40)},
		{"length-varint", append(h3RequestHeaders(), 0, 0x40)},
		{"payload", append(h3RequestHeaders(), 0, 3, 'x')},
		{"first-HEADERS", []byte{1, 10, 0, 0}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			q := &binQUIC{native: true, alpn: "h3", byteLimit: 65536}
			bad, other := h3FINStream(0, 0, tt.wire, true), h3FINStream(4, 0, h3RequestHeaders(), true)
			info, err := h3FINFrameFeed(q, 0, bad, other)
			var pe *ProtocolError
			require.ErrorAs(t, err, &pe)
			require.Equal(t, ErrMalformedMessage, pe.Kind, "all bytes up to a clean FIN are present; this is not capture NeedMore")
			fault, ok := info["HTTP3 Connection Error"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, "H3_FRAME_ERROR", fault["Error Name"])
			require.Equal(t, uint64(0x106), fault["Error Code"])
			require.Equal(t, "connection", fault["Error Scope"])
			require.Equal(t, uint64(1), fault["First PDU ID"])
			require.Equal(t, true, info["HTTP3"])
			require.Nil(t, info["HTTP3 Stream Errors"], "a connection layout error must not become a message stream error")
			require.NotEmpty(t, other["HTTP3 Frames"], "other authenticated QUIC STREAM bytes remain inspectable after a connection violation")
			require.NotEmpty(t, bad["HTTP3 Frame Error"])
			require.NotEmpty(t, bad["Byte Source"])
			require.Equal(t, "unverified-frame-layout", bad["HTTP3 Message Conformance"].(map[string]any)["Length Check"])
		})
	}
}

func TestHTTP3FINFrameFailureSuppressesRequestAssociationAndChargesFault(t *testing.T) {
	q := &binQUIC{native: true, alpn: "h3", byteLimit: 65536}
	_, err := h3FINFrameFeed(q, 0, h3FINStream(0, 0, append(h3RequestHeaders(), 0, 3, 'x'), true))
	require.Error(t, err)
	fault, charged := q.h3Fault, q.initialMemory()
	q.h3Fault = nil
	require.Equal(t, int64(128), charged-q.initialMemory())
	q.h3Fault = fault
	response := h3FINStream(0, 0, h3LiteralFields([2]string{":status", "200"}), true)
	info, err := h3FINFrameFeed(q, 1, response)
	require.NoError(t, err)
	require.NotNil(t, info["HTTP3 Connection Error"])
	require.NotEqual(t, true, response["HTTP3 Frames"].([]map[string]any)[0]["Associated Request"])
	require.Zero(t, response["Request PDU ID"])
}

func TestHTTP3ConnectionErrorFieldProjectionUsesOwnedObservation(t *testing.T) {
	pdu := uint64(1<<60) + 13
	fault := map[string]any{"Error Code": uint64(0x106), "First PDU ID": pdu, "Stream ID": uint64(4), "Direction": uint64(0)}
	e := &ProtocolEvent{Protocol: "http3", nativeQUICObservation: true, semanticSession: map[string]any{"HTTP3 Connection Error": fault}, Session: map[string]any{"HTTP3 Connection Error": map[string]any{"Error Code": uint64(0)}}}
	s := e.DisplayFieldValues()
	require.Equal(t, []any{uint64(0x106)}, s.FieldMap()["http3.connection.error_code"])
	require.Equal(t, []any{pdu}, s.FieldMap()["http3.connection.error_pdu"])
	for _, v := range s.Values {
		if v.Name == "http3.connection.error_pdu" {
			require.Equal(t, "1152921504606846989", v.ExactValue)
			require.Nil(t, v.Range, "connection history is not a byte span in the current ciphertext packet")
		}
	}
	e.Session = nil
	require.Equal(t, []any{uint64(0x106)}, e.DisplayFields()["http3.connection.error_code"])
	e.semanticSession = nil
	e.Session = map[string]any{"HTTP3 Connection Error": fault}
	require.Nil(t, e.DisplayFields()["http3.connection.error_code"], "public native flags cannot manufacture private observed context")
}

func FuzzHTTP3CleanFINLayoutAndCaptureGaps(f *testing.F) {
	f.Add(uint8(0), uint8(5), true, true)
	f.Add(uint8(1), uint8(0), false, true)
	f.Add(uint8(2), uint8(20), true, false)
	f.Fuzz(func(t *testing.T, stage, n uint8, fin, gap bool) {
		q := &binQUIC{native: true, alpn: "h3", byteLimit: 65536}
		partial := []byte{0x40}
		if stage%3 == 1 {
			partial = []byte{0, 0x40}
		} else if stage%3 == 2 {
			partial = h3Data(make([]byte, int(n%96)+1))
			partial = partial[:len(partial)-1]
		}
		wire := append(h3RequestHeaders(), partial...)
		cut := len(wire) / 2
		if gap {
			info, err := h3FINFrameFeed(q, 0, h3FINStream(0, uint64(cut), bytes.Clone(wire[cut:]), fin))
			if err != nil || info["HTTP3 Connection Error"] != nil {
				t.Fatal("a final offset cannot certify missing capture bytes", err)
			}
			wire = wire[:cut]
		}
		other := h3FINStream(4, 0, h3RequestHeaders(), true)
		info, err := h3FINFrameFeed(q, 0, h3FINStream(0, 0, wire, fin && !gap), other)
		if (err != nil) != fin {
			t.Fatalf("clean FIN=%v gap=%v: %v", fin, gap, err)
		}
		if fin && (!isH3ConnectionObservationError(err) || isH3FieldStreamError(err) || info["HTTP3 Connection Error"] == nil) {
			t.Fatal("a fully observed truncated frame lost its connection error scope")
		}
		if other["HTTP3 Frames"] == nil {
			t.Fatal("transport evidence on another stream was discarded")
		}
	})
}

func TestHTTP3FINLayoutWaitsForCapturedGapAndPreservesLaterConnectionWarning(t *testing.T) {
	for _, truncated := range []bool{false, true} {
		q := &binQUIC{native: true, alpn: "h3", byteLimit: 65536}
		wire := append(h3RequestHeaders(), h3Data([]byte("abc"))...)
		if truncated {
			wire = wire[:len(wire)-1]
		}
		cut := len(wire) - 2
		tail := h3FINStream(0, uint64(cut), bytes.Clone(wire[cut:]), true)
		info, err := h3FINFrameFeed(q, 0, tail)
		require.NoError(t, err)
		require.Nil(t, info["HTTP3 Connection Error"], "a FIN beyond an unobserved prefix is not a layout verdict")
		other := h3FINStream(4, 0, h3RequestHeaders(), true)
		info, err = h3FINFrameFeed(q, 0, other)
		require.NoError(t, err)
		require.Nil(t, info["HTTP3 Connection Error"], "traffic on another stream cannot fill the capture gap")
		prefix := h3FINStream(0, 0, bytes.Clone(wire[:cut]), false)
		info, err = h3FINFrameFeed(q, 0, prefix)
		if !truncated {
			require.NoError(t, err)
			require.Nil(t, info["HTTP3 Connection Error"])
			continue
		}
		require.Error(t, err)
		fault, ok := info["HTTP3 Connection Error"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, uint64(3), fault["First PDU ID"])
		fault["Error Name"] = "edited-public-map"
		info, err = h3FINFrameFeed(q, 0, h3FINStream(8, 0, h3RequestHeaders(), true))
		require.NoError(t, err, "passive observation does not infer peer closure")
		require.Equal(t, "H3_FRAME_ERROR", info["HTTP3 Connection Error"].(map[string]any)["Error Name"], "earlier connection violation survives a later individually decodable stream")
		fresh := &binQUIC{native: true, alpn: "h3", byteLimit: 65536}
		info, err = h3FINFrameFeed(fresh, 0, h3FINStream(0, 0, h3RequestHeaders(), true))
		require.NoError(t, err)
		require.Nil(t, info["HTTP3 Connection Error"], "an independent connection generation does not inherit the fault")
	}
}

func TestHTTP3IncompleteOpenFrameAndAbruptResetAreNotCleanFINLayoutErrors(t *testing.T) {
	q := &binQUIC{native: true, alpn: "h3", byteLimit: 65536}
	wire := append(h3RequestHeaders(), 0, 3, 'x')
	info, err := h3FINFrameFeed(q, 0, h3FINStream(0, 0, wire, false))
	require.NoError(t, err)
	require.Nil(t, info["HTTP3 Connection Error"])
	info, err = h3FINFrameFeed(q, 0, map[string]any{"Frame Type": "RESET_STREAM", "Stream ID": uint64(0), "Final Size": uint64(len(wire)), "Application Error Code": uint64(0x10c)})
	require.NoError(t, err)
	require.Nil(t, info["HTTP3 Connection Error"], "RFC9114 permits an abrupt reset inside a frame")
}
