package pcaputil

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// These are post-authentication STREAM algorithm fixtures. Protected-datagram
// integration and the source-derived capture controls are checked separately.
func qpackRecoveryStream(t *testing.T, q *binQUIC, pdu uint64, dir int, sid, off uint64, data []byte) (map[string]any, error) {
	t.Helper()
	q.eventID = pdu
	q.eventRefs = []PacketReference{{Number: pdu, Domain: CaptureDomain{Interface: 7, Encapsulation: "/vlan:91"}}}
	info := map[string]any{}
	frame := map[string]any{"Frame Type": "STREAM", "Stream ID": sid, "Offset": off, "Stream Data": bytes.Clone(data), "FIN": false}
	info["Frames"] = []map[string]any{frame}
	err := q.applyFrames(dir, quicSpaceApplication, []map[string]any{frame}, info, 128)
	return info, err
}

func TestQPACKEncoderZeroMaximumUsesObservedSettings(t *testing.T) {
	for _, known := range []bool{false, true} {
		for _, dir := range []int{0, 1} {
			t.Run(fmt.Sprintf("known%v/dir%d", known, dir), func(t *testing.T) {
				q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
				if known {
					// A complete empty SETTINGS frame establishes both defaults.
					_, err := qpackRecoveryStream(t, q, 1, 1-dir, uint64(3-dir), 0, h3ControlSETTINGS())
					require.NoError(t, err)
				}
				info, err := qpackRecoveryStream(t, q, 2, dir, uint64(6+dir), 0, []byte{2, 0x20})
				var typed *ProtocolError
				require.ErrorAs(t, err, &typed)
				if known {
					// RFC 9204 3.2.3 prohibits every encoder instruction when
					// the observed peer maximum is zero, including SetCapacity(0).
					require.Equal(t, ErrMalformedMessage, typed.Kind)
				} else {
					require.Equal(t, ErrContextRequired, typed.Kind)
				}
				require.NotContains(t, info, "QPACK Instructions")
				require.Equal(t, 1, q.streams[uint64(6+dir)].h3.parsed[dir])
				require.Zero(t, q.qpack.table[dir].insertCount)
			})
		}
	}
}

func TestQPACKLatePeerSettingsResumesOriginalEncoderBytes(t *testing.T) {
	for _, dir := range []int{0, 1} {
		t.Run(fmt.Sprintf("dir%d", dir), func(t *testing.T) {
			q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
			sid := uint64(6 + dir)
			first, err := qpackRecoveryStream(t, q, 11, dir, sid, 0, []byte{2, 0x3f, 0x21}) // capacity 64
			var typed *ProtocolError
			require.ErrorAs(t, err, &typed)
			require.Equal(t, ErrContextRequired, typed.Kind)
			require.Equal(t, 1, q.streams[sid].h3.parsed[dir])
			first["Frames"].([]map[string]any)[0]["Stream Data"].([]byte)[1] ^= 1
			info, err := qpackRecoveryStream(t, q, 12, 1-dir, uint64(3-dir), 0, h3ControlSETTINGS(1, 128, 7, 4))
			require.NoError(t, err)
			require.Equal(t, uint64(64), q.qpack.table[dir].capacity)
			require.Equal(t, 3, q.streams[sid].h3.parsed[dir])
			resumed, ok := info["QPACK Resumed Encoders"].([]map[string]any)
			require.True(t, ok, "new context must publish the retained instruction result")
			require.Len(t, resumed, 1)
			require.Equal(t, []byte{0x3f, 0x21}, resumed[0]["Instruction Bytes"])
			source := resumed[0]["Byte Source"].(map[string]any)
			require.Equal(t, uint64(1), source["Offset"])
			require.Equal(t, uint64(2), source["Length"])
			require.Equal(t, []uint64{11}, source["Parent PDUs"])
			require.Empty(t, q.streams[sid].sources[dir].ranges)
			// Further complete control frames cannot parse the old instructions twice.
			_, err = qpackRecoveryStream(t, q, 13, dir, uint64(2+dir), 0, h3ControlSETTINGS(1, 128))
			require.NoError(t, err)
			require.Equal(t, uint64(64), q.qpack.table[dir].capacity)
		})
	}
}

func TestQPACKRecoverySeparatesCompletedPrefixFromIncompleteSuffix(t *testing.T) {
	for _, dir := range []int{0, 1} {
		t.Run(fmt.Sprintf("dir%d", dir), func(t *testing.T) {
			q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
			sid := uint64(6 + dir)
			_, err := qpackRecoveryStream(t, q, 11, dir, sid, 0, []byte{2, 0x3f, 0x21})
			require.Error(t, err)
			_, err = qpackRecoveryStream(t, q, 12, dir, sid, 3, []byte{0x41, 'x'})
			require.Error(t, err)
			info, err := qpackRecoveryStream(t, q, 13, 1-dir, uint64(3-dir), 0, h3ControlSETTINGS(1, 128))
			require.NoError(t, err)
			rows := info["QPACK Resumed Encoders"].([]map[string]any)
			require.Len(t, rows, 1)
			require.Equal(t, "decoded", rows[0]["Status"])
			require.Equal(t, []byte{0x3f, 0x21}, rows[0]["Instruction Bytes"])
			require.Equal(t, uint64(2), rows[0]["Incomplete Suffix Bytes"])
			source := rows[0]["Byte Source"].(map[string]any)
			require.Equal(t, uint64(1), source["Offset"])
			require.Equal(t, uint64(2), source["Length"])
			require.Equal(t, []uint64{11}, source["Parent PDUs"])
			require.Equal(t, []map[string]any{{"Packet": uint64(11), "Section": uint32(0), "Interface": 7, "Encapsulation": "/vlan:91"}}, source["Packet References"])
			require.Len(t, q.streams[sid].sources[dir].ranges, 1)
			require.Zero(t, q.qpack.table[dir].insertCount)
			info, err = qpackRecoveryStream(t, q, 14, dir, sid, 5, []byte{1, 'y'})
			require.NoError(t, err)
			require.Equal(t, uint64(1), q.qpack.table[dir].insertCount)
			require.Equal(t, []qpackEntry{{name: "x", value: "y", absolute: 0}}, q.qpack.table[dir].entries)
			require.Len(t, info["QPACK Instructions"].([]map[string]any), 1)
			require.Empty(t, q.streams[sid].sources[dir].ranges)
		})
	}
}

func TestQPACKRejectedEncoderBatchDoesNotReapplyAnInsertPrefix(t *testing.T) {
	for _, suffix := range []byte{0x01, 0xff} {
		t.Run(fmt.Sprintf("suffix%x", suffix), func(t *testing.T) {
			q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
			_, err := qpackRecoveryStream(t, q, 1, 1, 3, 0, h3ControlSETTINGS(1, 64))
			require.NoError(t, err)
			_, err = qpackRecoveryStream(t, q, 2, 0, 6, 0, []byte{2, 0x3f, 0x21})
			require.NoError(t, err)
			// A complete literal insertion followed by an invalid duplicate or
			// static-table name index must not mutate the live table on failure.
			bad := []byte{0x41, 'x', 1, 'y', suffix}
			if suffix == 0xff {
				bad = append(bad, 0x24)
			}
			for attempt := uint64(0); attempt < 2; attempt++ {
				info, err := qpackRecoveryStream(t, q, 3+attempt, 0, 6, 3+attempt*uint64(len(bad)), bad)
				require.Error(t, err)
				require.NotContains(t, info, "QPACK Instructions")
				require.Zero(t, q.qpack.table[0].insertCount)
				require.Empty(t, q.qpack.table[0].entries)
				require.Equal(t, uint64(64), q.qpack.table[0].capacity)
				require.Equal(t, 3, q.streams[6].h3.parsed[0])
			}
		})
	}
}

func TestQPACKMissingSettingsDoesNotDiscardOtherAuthenticatedStreams(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3", eventID: 11, eventRefs: []PacketReference{{Number: 11}}}
	encoder := map[string]any{"Frame Type": "STREAM", "Stream ID": uint64(6), "Offset": uint64(0), "Stream Data": []byte{2, 0x3f, 0x21}, "FIN": false}
	request := map[string]any{"Frame Type": "STREAM", "Stream ID": uint64(0), "Offset": uint64(0), "Stream Data": h3RequestHeaders(), "FIN": true}
	info := map[string]any{"Frames": []map[string]any{encoder, request}}
	err := q.applyFrames(0, quicSpaceApplication, info["Frames"].([]map[string]any), info, 128)
	require.True(t, isH3EncoderContextError(err))
	require.True(t, isRecoverableH3ObservationError(err))
	require.Equal(t, "peer SETTINGS not observed", encoder["QPACK Decoder Context"])
	require.Equal(t, true, encoder["HTTP3"])
	require.NotEmpty(t, request["HTTP3 Frames"])
	require.Equal(t, 1, q.streams[0].h3.reqHeaders)
	info, err = qpackRecoveryStream(t, q, 12, 1, 3, 0, h3ControlSETTINGS(1, 128))
	require.NoError(t, err)
	require.Len(t, info["QPACK Resumed Encoders"].([]map[string]any), 1)
	require.Equal(t, uint64(64), q.qpack.table[0].capacity)
}

func TestQPACKIncompleteOrRejectedSettingsCannotGrantEncoderContext(t *testing.T) {
	for _, which := range []string{"partial", "duplicate-setting", "request-stream"} {
		t.Run(which, func(t *testing.T) {
			q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
			_, err := qpackRecoveryStream(t, q, 11, 0, 6, 0, []byte{2, 0x3f, 0x21})
			require.Error(t, err)
			control := h3ControlSETTINGS(1, 128)
			sid := uint64(3)
			switch which {
			case "partial":
				control = control[:len(control)-1]
			case "duplicate-setting":
				control = h3ControlSETTINGS(1, 128, 1, 128)
			case "request-stream":
				control, sid = h3Frame(4, []byte{1, 0x40, 0x80}), 0
			}
			info, _ := qpackRecoveryStream(t, q, 12, 1, sid, 0, control)
			require.False(t, q.qpack.maxKnown[0])
			require.Zero(t, q.qpack.maxCap[0])
			require.Zero(t, q.qpack.table[0].capacity)
			require.NotContains(t, info, "QPACK Resumed Encoders")
			require.True(t, q.streams[6].h3.encoderWaiting[0])
		})
	}
}

func TestQPACKLateSettingsDoNotReviveClosedOrRetiredOwner(t *testing.T) {
	for _, closed := range []bool{false, true} {
		t.Run(fmt.Sprintf("closed%v", closed), func(t *testing.T) {
			q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
			_, err := qpackRecoveryStream(t, q, 11, 0, 6, 0, []byte{2, 0x3f, 0x21})
			require.Error(t, err)
			q.qpack.maxCap[0], q.qpack.maxKnown[0] = 128, true
			q.closed, q.retired = closed, !closed
			info := map[string]any{}
			q.resumeSettingsEncoders(0, 128, info)
			require.NotContains(t, info, "QPACK Resumed Encoders")
			require.Zero(t, q.qpack.table[0].capacity)
			require.Equal(t, 1, q.streams[6].h3.parsed[0])
		})
	}
}

func TestQPACKLateSettingsAcrossProtectedNativePackets(t *testing.T) {
	// Key/ALPN state is an explicit algorithm fixture here. STREAM envelopes,
	// authentication, packet-number handling and native dispatch are exercised;
	// source-derived captures separately cover actual TLS/HTTP3 negotiation.
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, cipher := range []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305} {
			for _, dir := range []int{0, 1} {
				t.Run(fmt.Sprintf("v%x/%s/dir%d", version, cipher, dir), func(t *testing.T) {
					a := earlyRecoveryParser(t)
					q, key := quicConformanceContext(t, version, cipher)
					q.alpn, q.byteLimit = "h3", 65536
					domain := CaptureDomain{Section: 2, Interface: 7, Encapsulation: "/vlan:91"}
					f := earlyRecoveryOwner(a, q, domain)
					base := func(number uint64, sender int) *ProtocolEvent {
						e := earlyRecoveryBase(f, domain, number)
						if sender == 1 {
							e.Source, e.Destination = e.Destination, e.Source
						}
						return e
					}
					sid := uint64(6 + dir)
					firstWire := quicConformancePacket(t, key, "1-RTT", 1, 0, quicStreamFrame(sid, 0, false, []byte{2, 0x3f, 0x21}))
					first, ok := a.decodeQUICDatagram(base(11, dir), firstWire)
					require.True(t, ok)
					require.Len(t, first, 1)
					require.Equal(t, "context-required", first[0].Status)
					require.Equal(t, "http3", first[0].Protocol)
					require.Equal(t, true, first[0].Session["Authentication Verified"])
					require.NotContains(t, q.spaces[dir][quicSpaceApplication].rejected, uint64(1))
					firstPDU := first[0].ID
					first[0].Raw[0] ^= 1
					first[0].SourceBytes.PacketRefs[0].Number = 999
					settings := quicConformancePacket(t, key, "1-RTT", 2, 0, quicStreamFrame(uint64(3-dir), 0, false, h3ControlSETTINGS(1, 128)))
					second, ok := a.decodeQUICDatagram(base(12, 1-dir), settings)
					require.True(t, ok)
					require.Len(t, second, 1)
					a.emit(second[0])
					require.Equal(t, "decoded", second[0].Status)
					require.Equal(t, f.id, second[0].FlowID)
					rows := second[0].Session["QPACK Resumed Encoders"].([]map[string]any)
					require.Len(t, rows, 1)
					require.Equal(t, firstPDU, rows[0]["Reprocessed From PDU"])
					require.Equal(t, []byte{0x3f, 0x21}, rows[0]["Instruction Bytes"])
					source := rows[0]["Byte Source"].(map[string]any)
					require.Equal(t, []uint64{firstPDU}, source["Parent PDUs"])
					require.Equal(t, []map[string]any{{"Packet": uint64(11), "Section": uint32(2), "Interface": 7, "Encapsulation": "/vlan:91"}}, source["Packet References"])
					require.NotNil(t, second[0].SourceBytes.CompletedIn)
					require.Equal(t, uint64(12), second[0].SourceBytes.CompletedIn.Number)
					require.Equal(t, uint64(64), q.qpack.table[dir].capacity)
					require.Empty(t, q.streams[sid].sources[dir].ranges)
					again, ok := a.decodeQUICDatagram(base(13, 1-dir), settings)
					require.True(t, ok)
					require.Len(t, again, 1)
					require.NotContains(t, again[0].Session, "QPACK Resumed Encoders")
				})
			}
		}
	}
}

func TestQPACKSettingsRecoveryKeepsInterfaceAndVLANDomainsSeparate(t *testing.T) {
	for _, separate := range []CaptureDomain{{Interface: 8, Encapsulation: "/vlan:91"}, {Interface: 7, Encapsulation: "/vlan:92"}} {
		t.Run(fmt.Sprintf("if%d%s", separate.Interface, separate.Encapsulation), func(t *testing.T) {
			a := earlyRecoveryParser(t)
			domain := CaptureDomain{Interface: 7, Encapsulation: "/vlan:91"}
			q, key := quicConformanceContext(t, quicVersion1, quicAEADAES128GCM)
			q.alpn, q.byteLimit = "h3", 65536
			f := earlyRecoveryOwner(a, q, domain)
			other, _ := quicConformanceContext(t, quicVersion1, quicAEADAES128GCM)
			other.alpn, other.byteLimit = "h3", 65536
			g := earlyRecoveryOwner(a, other, separate)
			first, ok := a.decodeQUICDatagram(earlyRecoveryBase(f, domain, 11), quicConformancePacket(t, key, "1-RTT", 1, 0, quicStreamFrame(6, 0, false, []byte{2, 0x3f, 0x21})))
			require.True(t, ok)
			require.Len(t, first, 1)
			require.Equal(t, "context-required", first[0].Status)
			base := earlyRecoveryBase(g, separate, 12)
			base.Source, base.Destination = base.Destination, base.Source
			settings := quicConformancePacket(t, key, "1-RTT", 2, 0, quicStreamFrame(3, 0, false, h3ControlSETTINGS(1, 128)))
			otherEvents, ok := a.decodeQUICDatagram(base, settings)
			require.True(t, ok)
			require.Len(t, otherEvents, 1)
			require.Equal(t, g.id, otherEvents[0].FlowID)
			require.NotContains(t, otherEvents[0].Session, "QPACK Resumed Encoders")
			require.False(t, q.qpack.maxKnown[0])
			require.Zero(t, q.qpack.table[0].capacity)
			require.True(t, q.streams[6].h3.encoderWaiting[0])
			base = earlyRecoveryBase(f, domain, 13)
			base.Source, base.Destination = base.Destination, base.Source
			events, ok := a.decodeQUICDatagram(base, settings)
			require.True(t, ok)
			require.Len(t, events, 1)
			require.Equal(t, f.id, events[0].FlowID)
			require.Len(t, events[0].Session["QPACK Resumed Encoders"].([]map[string]any), 1)
			require.Equal(t, uint64(64), q.qpack.table[0].capacity)
		})
	}
}

func FuzzQPACKLateSettingsFragmentation(f *testing.F) {
	f.Add([]byte("value"), uint8(1), true)
	f.Add([]byte{0, 0xff, '\r'}, uint8(5), false)
	f.Add([]byte{}, uint8(0), true)
	f.Fuzz(func(t *testing.T, value []byte, split uint8, settingsBefore bool) {
		if len(value) > 32 {
			return
		}
		q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
		wire := append([]byte{2, 0x3f, 0x21, 0x41, 'x', byte(len(value))}, value...)
		at := int(split) % (len(wire) + 1)
		settings := func() {
			_, err := qpackRecoveryStream(t, q, 12, 1, 3, 0, h3ControlSETTINGS(1, 64))
			require.NoError(t, err)
		}
		if settingsBefore {
			settings()
		}
		_, err := qpackRecoveryStream(t, q, 11, 0, 6, 0, wire[:at])
		if err != nil {
			require.True(t, isH3EncoderContextError(err))
		}
		if !settingsBefore {
			settings()
		}
		_, err = qpackRecoveryStream(t, q, 13, 0, 6, uint64(at), wire[at:])
		require.NoError(t, err)
		require.Equal(t, uint64(1), q.qpack.table[0].insertCount)
		require.Equal(t, uint64(64), q.qpack.table[0].capacity)
		require.Equal(t, []qpackEntry{{name: "x", value: string(value), absolute: 0}}, q.qpack.table[0].entries)
		require.Equal(t, len(wire), q.streams[6].h3.parsed[0])
		require.Empty(t, q.streams[6].sources[0].ranges)
		info, err := qpackRecoveryStream(t, q, 14, 0, 6, 0, wire)
		require.NoError(t, err)
		require.NotContains(t, info, "QPACK Instructions")
		require.Equal(t, uint64(1), q.qpack.table[0].insertCount)
	})
}

func TestQPACKObservedPeerLimitIsProtocolConformance(t *testing.T) {
	for _, settings := range [][]byte{h3ControlSETTINGS(), h3ControlSETTINGS(1, 0), h3ControlSETTINGS(1, 32)} {
		t.Run(fmt.Sprintf("%x", settings), func(t *testing.T) {
			q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
			_, err := qpackRecoveryStream(t, q, 11, 1, 3, 0, settings)
			require.NoError(t, err)
			require.True(t, q.qpack.maxKnown[0])
			_, err = qpackRecoveryStream(t, q, 12, 0, 6, 0, []byte{2, 0x3f, 0x21})
			var typed *ProtocolError
			require.ErrorAs(t, err, &typed)
			require.Equal(t, ErrMalformedMessage, typed.Kind)
			require.Zero(t, q.qpack.table[0].capacity)
			require.False(t, q.streams[6].h3.encoderWaiting[0])
		})
	}
}

func TestQPACKCompleteSettingsIsUsableBeforeLaterControlFrameRejection(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	_, err := qpackRecoveryStream(t, q, 11, 0, 6, 0, []byte{2, 0x3f, 0x21})
	require.Error(t, err)
	control := append(h3ControlSETTINGS(1, 128), h3Data([]byte("forbidden on control"))...)
	info, err := qpackRecoveryStream(t, q, 12, 1, 3, 0, control)
	require.Error(t, err, "later invalid control frame remains rejected")
	require.Len(t, info["QPACK Resumed Encoders"].([]map[string]any), 1)
	require.Equal(t, uint64(64), q.qpack.table[0].capacity)
	require.Equal(t, 3, q.streams[6].h3.parsed[0])
}

func TestQPACKLateSettingsRecoveryPropagatesResourceRefusal(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	_, err := qpackRecoveryStream(t, q, 11, 0, 6, 0, []byte{2, 0x3f, 0x21, 0x41, 'x', 1, 'y'})
	require.Error(t, err)
	q.qpack.maxKnown[0], q.qpack.maxCap[0] = true, 64
	info := map[string]any{}
	err = q.resumeSettingsEncoders(0, 1, info)
	var typed *ProtocolError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, ErrResourceExceeded, typed.Kind)
	require.Zero(t, q.qpack.table[0].capacity, "failed batch never committed its prefix")
	require.Zero(t, q.qpack.table[0].insertCount)
	require.Equal(t, 1, q.streams[6].h3.parsed[0])
	require.Equal(t, "limited", info["QPACK Resumed Encoders"].([]map[string]any)[0]["Status"])
}

func TestQPACKOneRejectedBlockedHeaderDoesNotHideAnotherRecoveredStream(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	_, err := qpackRecoveryStream(t, q, 10, 1, 3, 0, h3ControlSETTINGS(1, 220, 7, 2))
	require.NoError(t, err)
	_, err = qpackRecoveryStream(t, q, 11, 0, 0, 0, h3Frame(1, rfcHex("03811011")))
	require.NoError(t, err)
	_, err = qpackRecoveryStream(t, q, 12, 0, 4, 0, h3Frame(1, rfcHex("03811011d1d7")))
	require.NoError(t, err)
	require.True(t, q.streams[0].h3.blocked[0])
	require.True(t, q.streams[4].h3.blocked[0])
	info, err := qpackRecoveryStream(t, q, 13, 0, 6, 0, h3EncoderStream(rfcHex("3fbd01c00f7777772e6578616d706c652e636f6dc10c2f73616d706c652f70617468")))
	require.True(t, isH3FieldStreamError(err), "the first stream remains a rejected HTTP message")
	require.Equal(t, uint64(2), q.qpack.table[0].insertCount)
	require.Equal(t, 1, q.streams[4].h3.reqHeaders)
	require.False(t, q.streams[4].h3.blocked[0])
	resumed := info["QPACK Resumed Streams"].([]map[string]any)
	require.Len(t, resumed, 2)
	require.Contains(t, resumed[0], "Decode Error")
	require.NotContains(t, resumed[1], "Decode Error")
	frame := resumed[1]["HTTP3 Frames"].([]map[string]any)[0]
	require.Equal(t, uint64(4), frame["Stream ID"])
	require.Equal(t, []uint64{12}, frame["Byte Source"].(map[string]any)["Parent PDUs"])
	require.Empty(t, q.streams[4].sources[0].ranges)
}
