package pcaputil

import (
	"bytes"
	"encoding/binary"
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

func TestQUICAuthenticationFieldSurvivesApplicationLayerAndPublicEdits(t *testing.T) {
	for _, protocol := range []string{"quic", "http3", "doq"} {
		e := &ProtocolEvent{Protocol: protocol, nativeQUICObservation: true, semanticSession: map[string]any{"Authentication Verified": true}, Session: map[string]any{"Authentication Verified": false}}
		require.Equal(t, []any{true}, e.DisplayFields()["quic.packet.authenticated"], protocol)
		e.semanticSession = map[string]any{"Authentication Verified": false}
		e.Session["Authentication Verified"] = true
		require.Equal(t, []any{false}, e.DisplayFields()["quic.packet.authenticated"], protocol)
		e.semanticSession = nil
		require.Nil(t, e.DisplayFields()["quic.packet.authenticated"], "public flags cannot manufacture native packet authentication")
	}
	e := &ProtocolEvent{Protocol: "tls", Session: map[string]any{"Authentication Verified": true}}
	require.Nil(t, e.DisplayFields()["quic.packet.authenticated"], "TLS record authentication is a different carrier")
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

func TestHTTP3HugeFinalFrameLengthIsLayoutErrorWithoutPayloadAllocation(t *testing.T) {
	q := &binQUIC{native: true, alpn: "h3", byteLimit: 65536}
	// DATA length = (1<<60)+7, encoded in eight bytes; no payload was sent.
	wire := append(h3RequestHeaders(), 0, 0xd0, 0, 0, 0, 0, 0, 0, 7)
	bad := h3FINStream(0, 0, wire, true)
	_, err := h3FINFrameFeed(q, 0, bad)
	var pe *ProtocolError
	require.ErrorAs(t, err, &pe)
	require.Equal(t, ErrMalformedMessage, pe.Kind, "a known clean final extent is not a request to allocate the declared body")
	fault := bad["HTTP3 Frame Error"].(map[string]any)
	require.Equal(t, uint64(1<<60)+7, fault["Declared Payload Length"])
	require.Equal(t, uint64(0), fault["Available Payload Bytes"])
	require.Less(t, q.initialMemory(), int64(20000))
}

func TestHTTP3KnownStreamRetransmissionKeepsIdentityWithoutReplayingFrames(t *testing.T) {
	q := &binQUIC{native: true, alpn: "h3", byteLimit: 65536}
	wire := append(h3BodyRequest("POST", "3"), h3Data([]byte("abc"))...)
	_, err := h3FINFrameFeed(q, 0, h3FINStream(0, 0, bytes.Clone(wire), true))
	require.NoError(t, err)
	fr := h3FINStream(0, 0, bytes.Clone(wire), true)
	info, err := h3FINFrameFeed(q, 0, fr)
	require.NoError(t, err)
	require.Equal(t, true, info["HTTP3"], "known authenticated stream identity must survive a no-new-bytes delivery")
	require.Equal(t, true, info["HTTP3 Wire Only"])
	require.Equal(t, "retransmission", fr["HTTP3 Wire Observation"])
	require.Empty(t, fr["HTTP3 Frames"], "do not fabricate another logical frame")
	require.Equal(t, uint64(3), q.streams[0].h3.body[0].observed)
	require.Equal(t, 1, q.streams[0].h3.reqHeaders)
	info, err = h3FINFrameFeed(q, 0, h3FINStream(0, 0, bytes.Clone(wire), true), h3FINStream(8, 0, h3RequestHeaders(), true))
	require.NoError(t, err)
	require.Equal(t, false, info["HTTP3 Wire Only"], "another stream in the packet can supply a genuinely new application frame")
	unknown := h3FINStream(4, 10, []byte("unseen-prefix"), false)
	info, err = h3FINFrameFeed(q, 0, unknown)
	require.NoError(t, err)
	require.Equal(t, true, info["HTTP3 Wire Only"], "current h3 ALPN identifies a client bidi wire observation, not another stream's message")
	require.Equal(t, "client-bidirectional", info["HTTP3 Stream Kind"])
	require.Nil(t, q.streams[4].h3, "connection identity must not fabricate framing state")
	fresh := &binQUIC{native: true, byteLimit: 65536}
	info, err = h3FINFrameFeed(fresh, 0, h3FINStream(0, 10, []byte("new-generation"), false))
	require.NoError(t, err)
	require.Nil(t, info["HTTP3"], "a different connection generation does not inherit stream identity")
}

func TestHTTP3MissingPrefixKeepsOnlyAuthenticatedConnectionAndStreamRole(t *testing.T) {
	for _, dir := range []int{0, 1} {
		q := &binQUIC{native: true, alpn: "h3", byteLimit: 65536}
		fr := h3FINStream(12, 100, []byte("suffix"), true)
		info, err := h3FINFrameFeed(q, dir, fr)
		require.NoError(t, err)
		require.Equal(t, true, info["HTTP3 Wire Only"])
		require.Equal(t, "prefix-missing", info["HTTP3 Wire Observation"])
		require.Equal(t, "client-bidirectional", fr["HTTP3 Stream Kind"])
		require.Equal(t, 0, fr["Delivered Bytes"])
		require.Equal(t, false, fr["Stream Complete"], "FIN above a hole does not deliver an application ending")
		require.Nil(t, q.streams[12].h3)
		require.Zero(t, q.streams[12].requestPDU)
		for _, key := range []string{"HTTP3 Frames", "Headers", "Request PDU ID", "Transaction ID"} {
			require.Nil(t, fr[key], key)
			require.Nil(t, info[key], key)
		}
	}
	for _, tt := range []struct {
		name string
		alpn string
		sid  uint64
		dir  int
		data []byte
	}{
		{"new-generation-without-ALPN", "", 12, 0, []byte("suffix")},
		{"other-application", "doq", 12, 0, []byte("suffix")},
		{"client-unidirectional-type-unknown", "h3", 2, 0, []byte("suffix")},
		{"server-unidirectional-type-unknown", "h3", 3, 1, []byte("suffix")},
		{"server-bidirectional-extension-unknown", "h3", 1, 1, []byte("suffix")},
		{"empty-STREAM", "h3", 12, 0, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			q := &binQUIC{native: true, alpn: tt.alpn, byteLimit: 65536}
			info, err := h3FINFrameFeed(q, tt.dir, h3FINStream(tt.sid, 100, tt.data, false))
			require.NoError(t, err)
			require.Nil(t, info["HTTP3"])
			require.Nil(t, q.streams[tt.sid].h3)
		})
	}
}

func TestHTTP3MissingPrefixRecoveryDeliversFramesAndRequestAnchorOnce(t *testing.T) {
	q := &binQUIC{native: true, alpn: "h3", byteLimit: 65536}
	wire := append(h3BodyRequest("POST", "3"), h3Data([]byte("abc"))...)
	split := len(wire) - 3
	_, err := h3FINFrameFeed(q, 0, h3FINStream(12, uint64(split), bytes.Clone(wire[split:]), true))
	require.NoError(t, err)
	fr := h3FINStream(12, 0, bytes.Clone(wire[:split]), false)
	info, err := h3FINFrameFeed(q, 0, fr)
	require.NoError(t, err)
	require.Equal(t, uint64(2), fr["Request PDU ID"], "the request anchor starts on delivery, not its earlier buffered suffix")
	require.Len(t, info["HTTP3 Frames"], 2)
	require.Equal(t, uint64(3), q.streams[12].h3.body[0].observed)
	require.Equal(t, 1, q.streams[12].h3.reqHeaders)
	require.Equal(t, true, fr["Stream Complete"])
	require.Nil(t, info["HTTP3 Wire Only"])
	info, err = h3FINFrameFeed(q, 0, h3FINStream(12, uint64(split), bytes.Clone(wire[split:]), true))
	require.NoError(t, err)
	require.Equal(t, "retransmission", info["HTTP3 Wire Observation"])
	require.Equal(t, true, info["HTTP3 Wire Only"])
	require.Empty(t, info["HTTP3 Frames"])
	require.Equal(t, uint64(3), q.streams[12].h3.body[0].observed)
	require.Equal(t, 1, q.streams[12].h3.reqHeaders)
}

func TestHTTP3MissingPrefixCannotReplaceAnotherStreamsDecodedMetadata(t *testing.T) {
	for _, missingFirst := range []bool{false, true} {
		q := &binQUIC{native: true, alpn: "h3", byteLimit: 65536}
		missing := h3FINStream(12, 100, []byte("suffix"), false)
		decoded := h3FINStream(0, 0, h3RequestHeaders(), true)
		frames := []map[string]any{decoded, missing}
		if missingFirst {
			frames[0], frames[1] = frames[1], frames[0]
		}
		info, err := h3FINFrameFeed(q, 0, frames...)
		require.NoError(t, err)
		require.Equal(t, false, info["HTTP3 Wire Only"])
		require.Equal(t, uint64(0), info["HTTP3 Stream ID"])
		require.Equal(t, "request", info["HTTP3 Stream Kind"])
		require.Equal(t, "prefix-missing", missing["HTTP3 Wire Observation"])
		require.Nil(t, q.streams[12].h3)
	}
}

func TestHTTP3MixedPacketWireObservationCannotReplaceApplicationMetadata(t *testing.T) {
	for _, control := range []bool{false, true} {
		for _, replayFirst := range []bool{false, true} {
			q := &binQUIC{native: true, alpn: "h3", byteLimit: 65536}
			wire, sid := h3RequestHeaders(), uint64(0)
			if control {
				wire, sid = []byte{0, 4, 0}, 2 // Control stream and empty SETTINGS.
			}
			_, err := h3FINFrameFeed(q, 0, h3FINStream(sid, 0, bytes.Clone(wire), false))
			require.NoError(t, err)
			replay := h3FINStream(sid, 0, bytes.Clone(wire), false)
			fresh := h3FINStream(8, 0, h3RequestHeaders(), true)
			frames := []map[string]any{fresh, replay}
			if replayFirst {
				frames[0], frames[1] = frames[1], frames[0]
			}
			info, err := h3FINFrameFeed(q, 0, frames...)
			require.NoError(t, err)
			require.Equal(t, uint64(8), info["HTTP3 Stream ID"], "replay must not replace the newly decoded application's stream")
			require.Equal(t, "request", info["HTTP3 Stream Kind"])
			require.Equal(t, uint64(8), info["HTTP3 Message Conformance"].(map[string]any)["Stream ID"])
			require.Equal(t, false, info["HTTP3 Wire Only"])
			require.Empty(t, replay["HTTP3 Frames"])
		}
	}
}

func TestHTTP3BufferedRangeIdentityRequiresItsOwnKnownPrefixAndRejectsConflict(t *testing.T) {
	q := &binQUIC{native: true, alpn: "h3", byteLimit: 65536}
	wire := h3RequestHeaders()
	_, err := h3FINFrameFeed(q, 0, h3FINStream(0, 0, bytes.Clone(wire), false))
	require.NoError(t, err)
	future := h3FINStream(0, uint64(len(wire)+100), []byte("x"), false)
	info, err := h3FINFrameFeed(q, 0, future)
	require.NoError(t, err)
	require.Equal(t, true, info["HTTP3 Wire Only"])
	require.Equal(t, "buffered-range", future["HTTP3 Wire Observation"])
	require.Nil(t, future["HTTP3 Frames"])
	conflict := bytes.Clone(wire)
	conflict[0] ^= 1
	info, err = h3FINFrameFeed(q, 0, h3FINStream(0, 0, conflict, false))
	require.Error(t, err)
	require.Nil(t, info["HTTP3 Wire Observation"], "conflicting bytes are rejected before any read-only application classification")
}

func FuzzHTTP3FinalDeclaredLengthIsBoundedAndExact(f *testing.F) {
	f.Add(uint64(1<<60) + 7)
	f.Add(uint64(0))
	f.Add(^uint64(0))
	f.Fuzz(func(t *testing.T, input uint64) {
		length := input & ((uint64(1) << 62) - 1)
		header := make([]byte, 9)
		binary.BigEndian.PutUint64(header[1:], length|3<<62)
		q := &binQUIC{native: true, alpn: "h3", byteLimit: 65536}
		fr := h3FINStream(0, 0, append(h3RequestHeaders(), header...), true)
		_, err := h3FINFrameFeed(q, 0, fr)
		if length == 0 {
			if err != nil {
				t.Fatal(err)
			}
		} else {
			if !isH3ConnectionObservationError(err) {
				t.Fatal("clean final extent lost frame-layout classification", err)
			}
			if fr["HTTP3 Frame Error"].(map[string]any)["Declared Payload Length"] != length {
				t.Fatal("62-bit declared length was rounded or narrowed")
			}
		}
		if q.initialMemory() > 20000 {
			t.Fatal("declared length caused payload allocation")
		}
	})
}

func FuzzHTTP3RetransmissionDoesNotReplayApplication(f *testing.F) {
	f.Add(uint8(5), uint8(2))
	f.Add(uint8(0), uint8(0))
	f.Fuzz(func(t *testing.T, n, seed uint8) {
		q := &binQUIC{native: true, alpn: "h3", byteLimit: 65536}
		wire := append(h3BodyRequest("POST", "-"), h3Data(make([]byte, int(n%96)))...)
		if _, err := h3FINFrameFeed(q, 0, h3FINStream(0, 0, bytes.Clone(wire), true)); err != nil {
			t.Fatal(err)
		}
		off := 1 + int(seed)%(len(wire)-1)
		fr := h3FINStream(0, uint64(off), bytes.Clone(wire[off:]), false)
		info, err := h3FINFrameFeed(q, 0, fr)
		if err != nil || info["HTTP3 Wire Only"] != true || fr["HTTP3 Frames"] != nil || q.streams[0].h3.body[0].observed != uint64(n%96) {
			t.Fatal("retransmission changed semantic delivery", err)
		}
		fresh := &binQUIC{native: true, byteLimit: 65536}
		info, err = h3FINFrameFeed(fresh, 0, h3FINStream(0, uint64(off), bytes.Clone(wire[off:]), false))
		if err != nil || info["HTTP3"] != nil {
			t.Fatal("new generation borrowed a missing application prefix", err)
		}
		confirmed := &binQUIC{native: true, alpn: "h3", byteLimit: 65536}
		suffix := h3FINStream(0, uint64(off), bytes.Clone(wire[off:]), false)
		info, err = h3FINFrameFeed(confirmed, 0, suffix)
		if err != nil || info["HTTP3 Wire Only"] != true || info["HTTP3 Stream Kind"] != "client-bidirectional" || confirmed.streams[0].h3 != nil || confirmed.streams[0].requestPDU != 0 || suffix["HTTP3 Frames"] != nil {
			t.Fatal("h3 connection identity fabricated stream content", err)
		}
	})
}
