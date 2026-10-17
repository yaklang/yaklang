package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type canISOTapeFrame struct {
	id   uint32
	data []byte
	ms   int
}

func canISORecord(id uint32, data []byte, fd bool) []byte {
	n := 16
	if fd {
		n = 72
	}
	w := make([]byte, n)
	binary.BigEndian.PutUint32(w, id)
	w[4] = byte(len(data))
	if fd {
		w[5] = 4
	}
	copy(w[8:], data)
	return w
}
func canISOTape(t *testing.T, frames []canISOTapeFrame, fd bool) []byte {
	t.Helper()
	var b bytes.Buffer
	w := pcapgo.NewWriter(&b)
	require.NoError(t, w.WriteFileHeader(65535, layers.LinkType(227)))
	for _, f := range frames {
		raw := canISORecord(f.id, f.data, fd)
		require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1700001000, 0).Add(time.Duration(f.ms) * time.Millisecond), CaptureLength: len(raw), Length: len(raw)}, raw))
	}
	return b.Bytes()
}
func canISOReplay(t *testing.T, opts CANISOTransport, frames []canISOTapeFrame) ([]*ProtocolEvent, ProtocolStats) {
	t.Helper()
	var es []*ProtocolEvent
	var stats ProtocolStats
	require.NoError(t, ReplayPcap(bytes.NewReader(canISOTape(t, frames, opts.CANFD)), WithCANISOTransport(0, opts), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
	return es, stats
}
func canISOBase() CANISOTransport {
	return CANISOTransport{RequestID: 0x7e0, ResponseID: 0x7e8, DecodeUDS: true}
}
func canISOReadVIN() ([]canISOTapeFrame, []byte) {
	body := append([]byte{0x62, 0xf1, 0x90}, []byte("TESTVIN0123456789")...)
	frames := []canISOTapeFrame{{0x7e0, []byte{3, 0x22, 0xf1, 0x90}, 0}, {0x7e8, append([]byte{0x10, byte(len(body))}, body[:6]...), 10}, {0x7e0, []byte{0x30, 0, 0}, 20}, {0x7e8, append([]byte{0x21}, body[6:13]...), 30}, {0x7e8, append([]byte{0x22}, body[13:]...), 40}}
	return frames, body
}
func TestCANISOSelectedUDSPublicReplay(t *testing.T) {
	frames, body := canISOReadVIN()
	tape := canISOTape(t, frames, false)
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			for _, observe := range []bool{false, true} {
				t.Run(fmt.Sprintf("w%d/d%t/o%t", workers, deferred, observe), func(t *testing.T) {
					var es []*ProtocolEvent
					var stats ProtocolStats
					var seen atomic.Int64
					opts := []CaptureOption{WithCANISOTransport(0, canISOBase()), WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
					if observe {
						opts = append(opts, WithEveryPacket(func(p gopacket.Packet) {
							i := int(seen.Add(1)) - 1
							require.Equal(t, canISORecord(frames[i].id, frames[i].data, false), p.Data())
						}))
					}
					input := bytes.Clone(tape)
					require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
					clear(input)
					require.Len(t, es, 5)
					require.Zero(t, stats.BufferedBytes)
					if observe {
						require.EqualValues(t, 5, seen.Load())
					}
					refs := [][]uint64{{1}, {2}, {2, 3}, {2, 3, 4}, {2, 3, 4, 5}}
					for i, e := range es {
						require.Empty(t, e.Error)
						require.Nil(t, e.sessionError)
						require.EqualValues(t, i+1, e.ID)
						require.Zero(t, e.TransactionID)
						require.Zero(t, e.FlowID)
						require.Equal(t, "can-isotp", e.Protocol)
						require.Equal(t, "can-iso15765-2-selected-uds", e.Profile)
						require.Equal(t, "explicit-interface-id-pair", e.Admission)
						require.Equal(t, CaptureDomain{}, e.Domain)
						require.Equal(t, canISORecord(frames[i].id, frames[i].data, false), e.Raw)
						require.Len(t, e.SourceBytes.PacketRefs, len(refs[i]))
						for j, n := range refs[i] {
							require.Equal(t, n, e.SourceBytes.PacketRefs[j].Number)
							require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[j].Domain)
						}
						f, err := e.GetFields()
						require.NoError(t, err)
						require.Equal(t, frames[i].data, f["Data"])
						require.Equal(t, frames[i].id, f["Identifier"])
						require.Equal(t, byte(0), f["Header Reserved"].([]byte)[0])
						require.Equal(t, byte(0), f["Header Reserved"].([]byte)[1])
						require.Equal(t, byte(0), f["Header Reserved"].([]byte)[2])
						require.Equal(t, i == 0 || i == 2, e.Direction == 0)
						iso := f["ISO-TP"].(map[string]any)
						require.Equal(t, false, iso["Transaction ID Present"])
						require.Equal(t, i == 0 || i == 4, iso["Transfer Complete"])
						if i < 4 {
							require.Zero(t, e.ResponseTo)
						} else {
							require.EqualValues(t, 1, e.ResponseTo)
							require.Equal(t, body, iso["Payload"])
							uds := f["UDS"].(map[string]any)
							require.Equal(t, byte(0x62), uds["Service"])
							require.Equal(t, uint16(0xf190), uds["Data Identifier"])
							require.Equal(t, body[3:], uds["Data Record"])
							require.Equal(t, "unverified raw DID data record", uds["Data Interpretation"])
							require.Equal(t, "observed-request-match", uds["Association"])
						}
						if p, ok := iso["Payload"].([]byte); ok {
							p[0] ^= 255
						}
						clear(iso)
						f2, err := e.GetFields()
						require.NoError(t, err)
						require.Equal(t, frames[i].data, f2["Data"])
						if i == 4 {
							require.Equal(t, body, f2["ISO-TP"].(map[string]any)["Payload"])
						}
					}
					if deferred {
						require.EqualValues(t, 5, stats.Deferred)
					} else {
						require.EqualValues(t, 5, stats.Decoded)
					}
				})
			}
		}
	}
}
func TestCANISOTransportRefusalsAndAdjacent(t *testing.T) {
	frames, _ := canISOReadVIN()
	for _, c := range []struct {
		name  string
		index int
		data  []byte
		kind  string
	}{
		{"NoCTS", 2, []byte{0x21, 1, 2, 3, 4, 5, 6, 7}, "ContextRequired"},
		{"SequenceMismatch", 3, []byte{0x22, 1, 2, 3, 4, 5, 6, 7}, "MalformedMessage"},
		{"ReservedFC", 2, []byte{0x33, 0, 0}, "MalformedMessage"},
		{"ShortFC", 2, []byte{0x30, 0}, "MalformedMessage"},
		{"OverlapFF", 3, frames[1].data, "ContextRequired"},
		{"OversizeCF", 4, []byte{0x22}, "MalformedMessage"},
	} {
		t.Run(c.name, func(t *testing.T) {
			bad := append([]canISOTapeFrame(nil), frames...)
			bad[c.index].data = c.data
			es, stats := canISOReplay(t, canISOBase(), bad)
			require.GreaterOrEqual(t, len(es), 5)
			rocTypedError(t, c.kind, es[c.index].sessionError)
			require.Nil(t, es[c.index].Session)
			require.Zero(t, stats.BufferedBytes)
			if c.index < 4 {
				require.Zero(t, es[4].ResponseTo)
			}
			good, _ := canISOReplay(t, canISOBase(), frames)
			require.EqualValues(t, 1, good[4].ResponseTo)
		})
	}
}
func TestCANISOUDSIdentityAndLifecycle(t *testing.T) {
	req := canISOTapeFrame{0x7e0, []byte{2, 0x10, 3}, 0}
	ack := canISOTapeFrame{0x7e8, []byte{6, 0x50, 3, 0, 50, 1, 244}, 10}
	pending := canISOTapeFrame{0x7e8, []byte{3, 0x7f, 0x10, 0x78}, 5}
	t.Run("PendingThenTerminal", func(t *testing.T) {
		es, stats := canISOReplay(t, canISOBase(), []canISOTapeFrame{req, pending, ack})
		require.Len(t, es, 3)
		for _, e := range es {
			require.Nil(t, e.sessionError)
		}
		require.EqualValues(t, 1, es[1].ResponseTo)
		require.EqualValues(t, 1, es[2].ResponseTo)
		f, err := es[2].GetFields()
		require.NoError(t, err)
		u := f["UDS"].(map[string]any)
		require.Equal(t, uint16(50), u["P2 Server Max Milliseconds"])
		require.Equal(t, uint32(5000), u["P2 Star Server Max Milliseconds"])
		require.Zero(t, stats.BufferedBytes)
	})
	t.Run("ReusedIdentity", func(t *testing.T) {
		req2, ack2 := req, ack
		req2.ms = 20
		ack2.ms = 30
		es, stats := canISOReplay(t, canISOBase(), []canISOTapeFrame{req, ack, req2, ack2})
		require.Len(t, es, 4)
		rocTypedError(t, "ContextRequired", es[2].sessionError)
		rocTypedError(t, "ContextRequired", es[3].sessionError)
		require.Zero(t, es[3].ResponseTo)
		require.Zero(t, stats.BufferedBytes)
	})
	t.Run("SuppressionBitIsNotExactRetry", func(t *testing.T) {
		changed := req
		changed.data = []byte{2, 0x10, 0x83}
		changed.ms = 5
		es, stats := canISOReplay(t, canISOBase(), []canISOTapeFrame{req, changed, ack})
		require.Len(t, es, 3)
		rocTypedError(t, "ContextRequired", es[1].sessionError)
		rocTypedError(t, "ContextRequired", es[2].sessionError)
		require.Zero(t, stats.BufferedBytes)
	})
	t.Run("SuppressedNoPositiveRequired", func(t *testing.T) {
		r := req
		r.data = []byte{2, 0x10, 0x83}
		es, stats := canISOReplay(t, canISOBase(), []canISOTapeFrame{r})
		require.Len(t, es, 1)
		require.Nil(t, es[0].sessionError)
		require.Zero(t, stats.Incomplete)
		require.Zero(t, stats.BufferedBytes)
	})
	t.Run("PendingClose", func(t *testing.T) {
		es, stats := canISOReplay(t, canISOBase(), []canISOTapeFrame{req})
		require.Len(t, es, 2)
		rocTypedError(t, "NeedMore", es[1].sessionError)
		require.Contains(t, es[1].Error, "before terminal response")
		require.Zero(t, stats.BufferedBytes)
		require.Zero(t, es[1].ResponseTo)
	})
	t.Run("DifferentDescriptorNRCAmbiguous", func(t *testing.T) {
		r2 := req
		r2.data = []byte{2, 0x10, 2}
		r2.ms = 20
		n := pending
		n.ms = 25
		a2 := ack
		a2.data = []byte{6, 0x50, 2, 0, 50, 1, 244}
		a2.ms = 30
		es, stats := canISOReplay(t, canISOBase(), []canISOTapeFrame{req, ack, r2, n, a2})
		require.Len(t, es, 5)
		rocTypedError(t, "ContextRequired", es[3].sessionError)
		require.Zero(t, es[3].ResponseTo)
		require.Nil(t, es[4].sessionError)
		require.EqualValues(t, 3, es[4].ResponseTo)
		require.Zero(t, stats.BufferedBytes)
	})
}

func canISOMinLength(n int) int {
	if n <= 8 {
		return n
	}
	for _, v := range []int{12, 16, 20, 24, 32, 48, 64} {
		if n <= v {
			return v
		}
	}
	panic("too large CAN data")
}
func canISOTransferFrames(options CANISOTransport, body []byte, ll, block int) []canISOTapeFrame {
	ae := 0
	if options.ExtendedAddressing {
		ae = 1
	}
	header := func(dir int, p []byte) []byte {
		if ae == 0 {
			return p
		}
		addr := options.RequestAddress
		if dir == 1 {
			addr = options.ResponseAddress
		}
		return append([]byte{addr}, p...)
	}
	capacity := ll - ae - 1
	if ll > 8 {
		capacity--
	}
	if len(body) <= capacity {
		pci := []byte{byte(len(body))}
		if ll > 8 {
			pci = []byte{0, byte(len(body))}
		}
		data := header(0, append(pci, body...))
		data = append(data, bytes.Repeat([]byte{options.PaddingByte}, ll-len(data))...)
		return []canISOTapeFrame{{options.RequestID, data, 0}}
	}
	pci := []byte{0x10 | byte(len(body)>>8), byte(len(body))}
	if len(body) > 4095 {
		pci = []byte{0x10, 0, byte(len(body) >> 24), byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}
	}
	start := ll - ae - len(pci)
	out := []canISOTapeFrame{{options.RequestID, header(0, append(pci, body[:start]...)), 0}}
	offset, sn, inBlock := start, byte(1), 0
	waiting := true
	for offset < len(body) {
		if waiting {
			out = append(out, canISOTapeFrame{options.ResponseID, header(1, []byte{0x30, byte(block), 0}), len(out) * 10})
			waiting = false
			inBlock = 0
		}
		n := min(ll-ae-1, len(body)-offset)
		d := header(0, append([]byte{0x20 | sn}, body[offset:offset+n]...))
		wireN := canISOMinLength(len(d))
		d = append(d, bytes.Repeat([]byte{options.PaddingByte}, wireN-len(d))...)
		out = append(out, canISOTapeFrame{options.RequestID, d, len(out) * 10})
		offset += n
		sn = (sn + 1) & 15
		inBlock++
		if block > 0 && inBlock == block {
			waiting = true
		}
	}
	return out
}
func TestCANISOTransportCarrierAddressingAndWrap(t *testing.T) {
	cases := []struct {
		name                string
		size, ll, block     int
		fd, extended, mixed bool
	}{
		{"ClassicSF1", 1, 8, 0, false, false, false}, {"ClassicSF7", 7, 8, 0, false, false, false}, {"ClassicFF8", 8, 8, 0, false, false, false}, {"ClassicFF20", 20, 8, 0, false, false, false}, {"ClassicBS1", 30, 8, 1, false, false, false}, {"SequenceWrap", 130, 8, 0, false, false, false}, {"WideLength", 4096, 8, 0, false, false, false},
		{"FD8SF7", 7, 8, 0, true, false, false}, {"FDEscape8", 8, 12, 0, true, false, false}, {"FDEscape62", 62, 64, 0, true, false, false}, {"FDFF63", 63, 64, 0, true, false, false}, {"FDFF200", 200, 64, 1, true, false, false},
		{"ExtendedSF6", 6, 8, 0, false, true, false}, {"ExtendedFF7", 7, 8, 0, false, true, false}, {"ExtendedWrap", 130, 8, 0, false, true, false}, {"MixedFF30", 30, 8, 1, false, true, true}, {"ExtendedFDSF61", 61, 64, 0, true, true, false}, {"MixedFDFF200", 200, 64, 1, true, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := CANISOTransport{RequestID: 0x98da10f1, ResponseID: 0x98daf110, CANFD: c.fd, ExtendedAddressing: c.extended, RequestAddress: 0xa1, ResponseAddress: 0xb2, CheckPadding: true, PaddingByte: 0xaa}
			if c.mixed {
				o.ResponseAddress = o.RequestAddress
			}
			body := make([]byte, c.size)
			for i := range body {
				body[i] = byte(i % 251)
			}
			frames := canISOTransferFrames(o, body, c.ll, c.block)
			es, stats := canISOReplay(t, o, frames)
			require.Len(t, es, len(frames))
			require.Zero(t, stats.BufferedBytes)
			require.Empty(t, es[len(es)-1].Error)
			for i, e := range es {
				require.Nil(t, e.sessionError, "frame%d", i)
				require.Equal(t, frames[i].id&0x1fffffff, e.Session["Identifier"])
				iso := e.Session["ISO-TP"].(map[string]any)
				require.Equal(t, i == len(es)-1, iso["Transfer Complete"])
				if c.extended {
					require.Equal(t, 1, iso["Address Bytes"])
					if c.mixed {
						require.Equal(t, "mixed", iso["Addressing"])
					} else {
						require.Equal(t, "extended", iso["Addressing"])
					}
				}
			}
			f, err := es[len(es)-1].GetFields()
			require.NoError(t, err)
			require.Equal(t, body, f["ISO-TP"].(map[string]any)["Payload"])
			require.Zero(t, es[len(es)-1].ResponseTo)
		})
	}
}
func TestCANISOFlowControlAndTiming(t *testing.T) {
	o := CANISOTransport{RequestID: 0x7e0, ResponseID: 0x7e8, MaxWaitFrames: 2}
	body := bytes.Repeat([]byte{0x99}, 20)
	base := canISOTransferFrames(o, body, 8, 0)
	t.Run("WaitThenCTS", func(t *testing.T) {
		fs := []canISOTapeFrame{base[0], {o.ResponseID, []byte{0x31, 0, 0}, 5}, base[1], base[2], base[3]}
		es, stats := canISOReplay(t, o, fs)
		require.Len(t, es, 5)
		for _, e := range es {
			require.Nil(t, e.sessionError)
		}
		require.Equal(t, 1, es[1].Session["ISO-TP"].(map[string]any)["Wait Count"])
		require.Equal(t, false, es[1].Session["ISO-TP"].(map[string]any)["Parameters Applied"])
		require.Equal(t, body, es[4].Session["ISO-TP"].(map[string]any)["Payload"])
		require.Zero(t, stats.BufferedBytes)
	})
	t.Run("WaitLimit", func(t *testing.T) {
		o.MaxWaitFrames = 1
		es, stats := canISOReplay(t, o, []canISOTapeFrame{base[0], {o.ResponseID, []byte{0x31, 0, 0}, 5}, {o.ResponseID, []byte{0x31, 0, 0}, 10}, base[2]})
		rocTypedError(t, "ResourceExceeded", es[2].sessionError)
		rocTypedError(t, "ContextRequired", es[3].sessionError)
		require.Zero(t, stats.BufferedBytes)
	})
	t.Run("Overflow", func(t *testing.T) {
		es, stats := canISOReplay(t, o, []canISOTapeFrame{base[0], {o.ResponseID, []byte{0x32, 0, 0}, 5}, base[2]})
		require.True(t, es[1].Session["ISO-TP"].(map[string]any)["Receiver Overflow"].(bool))
		rocTypedError(t, "ContextRequired", es[2].sessionError)
		require.Zero(t, stats.BufferedBytes)
	})
	for _, gap := range []int{9, 10} {
		t.Run(fmt.Sprintf("STminGap%d", gap), func(t *testing.T) {
			fs := append([]canISOTapeFrame(nil), base...)
			fs[1].data = []byte{0x30, 0, 10}
			fs[3].ms = fs[2].ms + gap
			es, stats := canISOReplay(t, o, fs)
			if gap < 10 {
				rocTypedError(t, "ContextRequired", es[3].sessionError)
			} else {
				require.Nil(t, es[3].sessionError)
				require.Equal(t, body, es[3].Session["ISO-TP"].(map[string]any)["Payload"])
			}
			require.Zero(t, stats.BufferedBytes)
		})
	}
	for _, gap := range []int{1000, 1001} {
		t.Run(fmt.Sprintf("PhaseGap%d", gap), func(t *testing.T) {
			fs := append([]canISOTapeFrame(nil), base...)
			fs[1].ms = gap
			fs[2].ms = gap + 10
			fs[3].ms = gap + 20
			es, stats := canISOReplay(t, o, fs)
			if gap > 1000 {
				rocTypedError(t, "ContextRequired", es[1].sessionError)
			} else {
				require.Nil(t, es[3].sessionError)
			}
			require.Zero(t, stats.BufferedBytes)
		})
	}
}

func canISOTestParser(es *[]*ProtocolEvent, b ParserBudget, o CANISOTransport) *binParser {
	return &binParser{budget: b, canDecodeAs: map[int]string{0: "can-isotp", 1: "can-isotp"}, canISOTransports: map[int][]CANISOTransport{0: {o}, 1: {o}}, config: BinParserConfig{MaxBufferedBytes: b.MaxBufferedBytes, OnEvent: func(e *ProtocolEvent) { *es = append(*es, e) }}}
}
func TestCANISODomainsBudgetsAndClose(t *testing.T) {
	o := CANISOTransport{RequestID: 0x7e0, ResponseID: 0x7e8}
	body := bytes.Repeat([]byte{0x35}, 20)
	frames := canISOTransferFrames(o, body, 8, 0)
	now := time.Unix(1700001000, 0)
	t.Run("DomainsAndOwnedOutput", func(t *testing.T) {
		var es []*ProtocolEvent
		a := canISOTestParser(&es, DefaultParserBudget(), o)
		ds := []CaptureDomain{{}, {Interface: 1}, {Section: 1}, {Encapsulation: "vpn/1"}}
		for i, f := range frames {
			for j, d := range ds {
				w := canISORecord(f.id, f.data, false)
				droneCANStatsDeliver(a, w, uint64(i*4+j+1), d, now.Add(time.Duration(f.ms)*time.Millisecond))
				clear(w)
			}
		}
		require.Len(t, es, 16)
		for j, d := range ds {
			e := es[12+j]
			require.Equal(t, d, e.Domain)
			require.Equal(t, body, e.Session["ISO-TP"].(map[string]any)["Payload"])
			require.Len(t, e.SourceBytes.PacketRefs, 4)
			for i, ref := range e.SourceBytes.PacketRefs {
				require.Equal(t, d, ref.Domain)
				require.EqualValues(t, i*4+j+1, ref.Number)
			}
			e.Session["ISO-TP"].(map[string]any)["Payload"].([]byte)[0] ^= 255
			f, err := e.GetFields()
			require.NoError(t, err)
			require.Equal(t, body, f["ISO-TP"].(map[string]any)["Payload"])
		}
		a.closeCANISO()
		a.closeCANISO()
		require.Zero(t, a.buffered.Load())
		require.Len(t, es, 16)
	})
	t.Run("RefusedDomainDoesNotPoisonOtherInterface", func(t *testing.T) {
		var es []*ProtocolEvent
		a := canISOTestParser(&es, DefaultParserBudget(), o)
		for j, d := range []CaptureDomain{{}, {Interface: 1}} {
			droneCANStatsDeliver(a, canISORecord(frames[0].id, frames[0].data, false), uint64(j+1), d, now)
		}
		a.refuseCANISORecord(CaptureDomain{})
		for i, f := range frames[1:] {
			droneCANStatsDeliver(a, canISORecord(f.id, f.data, false), uint64(i+3), CaptureDomain{Interface: 1}, now.Add(time.Duration(f.ms)*time.Millisecond))
		}
		require.Equal(t, body, es[len(es)-1].Session["ISO-TP"].(map[string]any)["Payload"])
		a.closeCANISO()
		require.Zero(t, a.buffered.Load())
	})
	for _, name := range []string{"DeclaredMessage", "Buffered", "Contributors"} {
		t.Run(name, func(t *testing.T) {
			b := DefaultParserBudget()
			switch name {
			case "DeclaredMessage":
				b.MaxMessageBytes = 16
			case "Buffered":
				b.MaxBufferedBytes = 256
			case "Contributors":
				b.MaxCollectionElements = 1
			}
			var es []*ProtocolEvent
			a := canISOTestParser(&es, b, o)
			for i, f := range frames {
				droneCANStatsDeliver(a, canISORecord(f.id, f.data, false), uint64(i+1), CaptureDomain{}, now.Add(time.Duration(f.ms)*time.Millisecond))
			}
			rocTypedError(t, "ResourceExceeded", es[0].sessionError)
			for _, e := range es {
				require.Nil(t, e.Session)
			}
			require.LessOrEqual(t, a.peak.Load(), int64(b.MaxBufferedBytes))
			a.closeCANISO()
			a.closeCANISO()
			require.Zero(t, a.buffered.Load())
		})
	}
	t.Run("ForeignReservationSurvivesClose", func(t *testing.T) {
		var es []*ProtocolEvent
		a := canISOTestParser(&es, DefaultParserBudget(), o)
		require.True(t, a.reserveEvidence(57))
		droneCANStatsDeliver(a, canISORecord(frames[0].id, frames[0].data, false), 1, CaptureDomain{}, now)
		a.closeCANISO()
		a.closeCANISO()
		require.Len(t, es, 2)
		rocTypedError(t, "NeedMore", es[1].sessionError)
		require.EqualValues(t, 57, a.buffered.Load())
		a.buffered.Add(-57)
		require.Zero(t, a.buffered.Load())
	})
	t.Run("AbsentTimeCannotClaimTimedContext", func(t *testing.T) {
		var es []*ProtocolEvent
		a := canISOTestParser(&es, DefaultParserBudget(), o)
		droneCANStatsDeliver(a, canISORecord(frames[0].id, frames[0].data, false), 1, CaptureDomain{}, time.Time{})
		rocTypedError(t, "ContextRequired", es[0].sessionError)
		a.closeCANISO()
		require.Zero(t, a.buffered.Load())
	})
}
func TestCANISOConfigurationAndUnsupportedIdentity(t *testing.T) {
	for _, o := range []CANISOTransport{{RequestID: 1, ResponseID: 1}, {RequestID: 0x1801, ResponseID: 2}, {RequestID: 0x60000001, ResponseID: 2}, {RequestID: 1, ResponseID: 2, UDSResponseTimeout: -time.Second}} {
		var c CaptureConfig
		require.Error(t, WithCANISOTransport(0, o)(&c))
	}
	var c CaptureConfig
	o := canISOBase()
	option := WithCANISOTransport(0, o)
	require.NoError(t, option(&c))
	require.NoError(t, option(&c))
	require.Len(t, c.canISOTransports[0], 1)
	require.Error(t, WithCANDecodeAs(0, "j1939")(&c))
	require.Error(t, WithCANISOTransport(0, CANISOTransport{RequestID: o.RequestID, ResponseID: 0x7e9})(&c))
	o.RequestID = 0
	require.Equal(t, uint32(0x7e0), c.canISOTransports[0][0].RequestID)
	t.Run("UnsupportedDIDListKeepsIdentityUnknown", func(t *testing.T) {
		req := canISOTapeFrame{0x7e0, []byte{5, 0x22, 0xf1, 0x90, 0xf1, 0x91}, 0}
		next := canISOTapeFrame{0x7e0, []byte{3, 0x22, 0xf1, 0x90}, 10}
		reply := canISOTapeFrame{0x7e8, []byte{4, 0x62, 0xf1, 0x90, 0x42}, 20}
		es, stats := canISOReplay(t, canISOBase(), []canISOTapeFrame{req, next, reply})
		require.Len(t, es, 3)
		rocTypedError(t, "UnsupportedFeature", es[0].sessionError)
		rocTypedError(t, "ContextRequired", es[1].sessionError)
		rocTypedError(t, "ContextRequired", es[2].sessionError)
		require.Zero(t, es[2].ResponseTo)
		require.Zero(t, stats.BufferedBytes)
	})
}

func canISOSealedFile(t *testing.T, name string) []byte {
	t.Helper()
	if root := os.Getenv("CAN_ISOTP_TEST_ROOT"); root != "" {
		b, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		return b
	}
	b, err := trafficfixture.ReadFile("can-isotp-uds/" + name)
	require.NoError(t, err)
	return b
}
func TestCANISOSealedFullFieldReplay(t *testing.T) {
	var m struct {
		Cases []struct {
			ID, Capture, Answer, SHA256 string
			AnswerSHA                   string `json:"answer_sha256"`
			Packets                     int
		}
	}
	require.NoError(t, json.Unmarshal(canISOSealedFile(t, "manifest.json"), &m))
	require.Len(t, m.Cases, 51)
	var frozen map[string]trafficfixture.FrozenCase
	if os.Getenv("CAN_ISOTP_TEST_ROOT") == "" {
		frozen = make(map[string]trafficfixture.FrozenCase)
		all, err := trafficfixture.AllExpectations()
		require.NoError(t, err)
		for _, batch := range all {
			for _, c := range batch.Cases {
				frozen[c.ID] = c
			}
		}
	}
	for _, c := range m.Cases {
		t.Run(c.ID, func(t *testing.T) {
			if frozen != nil {
				v, ok := frozen["can-isotp-uds/"+c.ID]
				require.True(t, ok)
				require.Equal(t, c.SHA256, v.Input.SHA256)
				require.Equal(t, "can-isotp-uds/"+c.Capture, v.Input.File)
				require.Equal(t, c.Packets, v.Facts.PacketCount)
				require.Len(t, v.Expectations, 1)
				var binding struct {
					Answer string `json:"answer_file"`
					SHA    string `json:"answer_sha256"`
				}
				require.NoError(t, json.Unmarshal(v.Expectations[0].PayloadConstraints, &binding))
				require.Equal(t, c.Answer, binding.Answer)
				require.Equal(t, c.AnswerSHA, binding.SHA)
			}
			capture, answer := canISOSealedFile(t, c.Capture), canISOSealedFile(t, c.Answer)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(capture)))
			require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(answer)))
			var a struct {
				Options CANISOTransport
				Events  []struct {
					Wire, Error string
					Fields      map[string]any
					Refs        []uint64
					Direction   int
					ResponseTo  uint64 `json:"response_to"`
					Why         string
					Close       bool
				}
			}
			require.NoError(t, decodeExactProtocolAnswer(answer, &a))
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/d%t/o%t", workers, deferred, observe), func(t *testing.T) {
							var es []*ProtocolEvent
							var stats ProtocolStats
							seen := 0
							opts := []CaptureOption{WithCANISOTransport(0, a.Options), WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(p gopacket.Packet) {
									w, err := hex.DecodeString(a.Events[seen].Wire)
									require.NoError(t, err)
									require.Equal(t, w, p.Data())
									seen++
								}))
							}
							input := bytes.Clone(capture)
							require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
							clear(input)
							require.Len(t, es, len(a.Events))
							require.Zero(t, stats.BufferedBytes)
							if observe {
								require.Equal(t, c.Packets, seen)
							}
							profile := canISOProfile
							if a.Options.DecodeUDS {
								profile = "can-iso15765-2-selected-uds"
							}
							success := 0
							for i, want := range a.Events {
								e := es[i]
								require.EqualValues(t, i+1, e.ID)
								require.Equal(t, want.Direction, e.Direction)
								require.Equal(t, want.ResponseTo, e.ResponseTo)
								require.Zero(t, e.TransactionID)
								require.Zero(t, e.FlowID)
								require.Equal(t, profile, e.Profile)
								require.Equal(t, "can-isotp", e.Protocol)
								require.Equal(t, "can", e.Transport)
								require.Equal(t, "explicit-interface-id-pair", e.Admission)
								require.Equal(t, CaptureDomain{}, e.Domain)
								if want.Error != "" {
									rocTypedError(t, want.Error, e.sessionError)
									require.Contains(t, e.Error, want.Why)
									require.Nil(t, e.Session)
									require.Nil(t, e.Structured)
									if want.Close {
										require.Empty(t, e.Raw)
									} else {
										raw, err := hex.DecodeString(want.Wire)
										require.NoError(t, err)
										require.Equal(t, raw, e.Raw)
									}
									require.Len(t, e.SourceBytes.PacketRefs, len(want.Refs))
									for j, n := range want.Refs {
										require.Equal(t, n, e.SourceBytes.PacketRefs[j].Number)
									}
									e.Session = map[string]any{"forged": true}
									e.Protocol = "http"
									e.Status = "decoded"
									e.Error = ""
									for _, fn := range []func() (map[string]any, error){e.GetFields, e.Decode} {
										f, err := fn()
										require.Nil(t, f)
										rocTypedError(t, want.Error, err)
									}
									continue
								}
								success++
								require.Empty(t, e.Error)
								raw, err := hex.DecodeString(want.Wire)
								require.NoError(t, err)
								require.Equal(t, raw, e.Raw)
								require.Len(t, e.SourceBytes.PacketRefs, len(want.Refs))
								for j, n := range want.Refs {
									require.Equal(t, n, e.SourceBytes.PacketRefs[j].Number)
									require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[j].Domain)
								}
								if len(want.Refs) > 1 {
									require.Equal(t, "reassembled", e.SourceBytes.Kind)
								} else {
									require.Equal(t, "captured", e.SourceBytes.Kind)
								}
								status := "decoded"
								if deferred {
									status = "deferred"
								}
								require.Equal(t, status, e.Status)
								require.Equal(t, "message", e.Completeness)
								rocEqualFields(t, want.Fields, e.Session)
								for _, fn := range []func() (map[string]any, error){e.GetFields, e.Decode} {
									f, err := fn()
									require.NoError(t, err)
									rocEqualFields(t, want.Fields, protocolFields(f))
									clear(f)
								}
								f, err := e.GetFields()
								require.NoError(t, err)
								f["Data"].([]byte)[0] ^= 255
								clear(f["ISO-TP"].(map[string]any))
								rocEqualFields(t, want.Fields, e.Session)
								rocEqualFields(t, want.Fields, e.semanticSession)
							}
							if deferred {
								require.EqualValues(t, success, stats.Deferred)
							} else {
								require.EqualValues(t, success, stats.Decoded)
							}
						})
					}
				}
			}
		})
	}
}
