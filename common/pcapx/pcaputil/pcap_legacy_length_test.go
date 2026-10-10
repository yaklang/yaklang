package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type legacyLengthCase struct {
	Name, File, SHA256, Order string
	Nano                      bool
	Original, Captured        uint32
	FrameHex                  string `json:"frame_hex"`
	Packets                   int
	Error                     string
	StrictError               string `json:"strict_error"`
}

func legacyLengthCases(t *testing.T) []legacyLengthCase {
	raw, err := trafficfixture.ReadFile("pcap-legacy/controls.json")
	require.NoError(t, err)
	var doc struct{ Cases []legacyLengthCase }
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Len(t, doc.Cases, 11)
	return doc.Cases
}
func legacyLengthBytes(t *testing.T, c legacyLengthCase) []byte {
	raw, err := trafficfixture.ReadFile("pcap-legacy/" + c.File)
	require.NoError(t, err)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
	return raw
}
func TestClassicPcapLegacyLengthSealed(t *testing.T) {
	for _, c := range legacyLengthCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			raw := legacyLengthBytes(t, c)
			strict, err := NewBoundedPcapReader(bytes.NewReader(raw))
			require.NoError(t, err)
			_, _, err = strict.ReadPacketData()
			if c.StrictError == "length" {
				require.ErrorContains(t, err, "invalid pcap record length")
			} else if c.StrictError == "truncated" {
				require.ErrorIs(t, err, io.ErrUnexpectedEOF)
			} else {
				require.NoError(t, err)
			}
			for _, chunk := range []int{1, 7, 64, len(raw)} {
				t.Run(fmt.Sprint(chunk), func(t *testing.T) {
					r, err := NewCaptureReaderWithOptions(&legacyChunkReader{raw: raw, chunk: chunk}, PcapReaderOptions{NormalizeLegacyOriginalLength: true})
					require.NoError(t, err)
					first, ci, err := r.ReadPacketData()
					if c.Error != "" {
						if c.Error == "length" {
							require.ErrorContains(t, err, "invalid pcap record length")
							require.Empty(t, r.classic.large)
						} else {
							require.ErrorIs(t, err, io.ErrUnexpectedEOF)
						}
						return
					}
					frame, err := hex.DecodeString(c.FrameHex)
					require.NoError(t, err)
					require.Equal(t, frame, first)
					require.EqualValues(t, c.Captured, ci.CaptureLength)
					wantLength := c.Original
					if wantLength < c.Captured {
						wantLength = c.Captured
					}
					require.EqualValues(t, wantLength, ci.Length)
					scale := int64(1000)
					if c.Nano {
						scale = 1
					}
					require.True(t, time.Unix(1700000000, 123456*scale).Equal(ci.Timestamp))
					if c.Original < c.Captured {
						require.Contains(t, ci.AncillaryData, PcapOriginalLength(c.Original))
					} else {
						require.Len(t, ci.AncillaryData, 1)
					}
					require.Equal(t, PacketReference{Number: 1}, evidenceFrom(ci).Ref)
					clear(first)
					ci.AncillaryData[0] = PcapOriginalLength(999999)
					second, ci2, err := r.ReadPacketData()
					require.NoError(t, err)
					require.Equal(t, frame, second)
					require.Equal(t, len(frame), ci2.Length)
					require.Len(t, ci2.AncillaryData, 1)
					require.Equal(t, PacketReference{Number: 2}, evidenceFrom(ci2).Ref)
					_, _, err = r.ReadPacketData()
					require.ErrorIs(t, err, io.EOF)
				})
			}
		})
	}
}

type legacyChunkReader struct {
	raw   []byte
	chunk int
}

func (r *legacyChunkReader) Read(b []byte) (int, error) {
	if len(r.raw) == 0 {
		return 0, io.EOF
	}
	n := min(len(b), r.chunk, len(r.raw))
	copy(b, r.raw[:n])
	r.raw = r.raw[n:]
	return n, nil
}

func TestClassicPcapLegacyLengthReplayMatrix(t *testing.T) {
	for _, c := range legacyLengthCases(t) {
		if c.Error != "" {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			raw := legacyLengthBytes(t, c)
			normalized := bytes.Clone(raw)
			// Canonical carrier retains the exact payload and timestamps. Only record
			// metadata changes; semantic parity is not an independent DNS oracle.
			order := binaryOrder(c.Order)
			offset := 24
			for offset < len(normalized) {
				n := int(order.Uint32(normalized[offset+8:]))
				if order.Uint32(normalized[offset+12:]) < uint32(n) {
					order.PutUint32(normalized[offset+12:], uint32(n))
				}
				offset += 16 + n
			}
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				opts := []CaptureOption{WithProtocolDeferred(deferred)}
				var seen []gopacket.CaptureInfo
				if observe {
					opts = append(opts, WithEveryPacket(func(p gopacket.Packet) { seen = append(seen, p.Metadata().CaptureInfo) }))
				}
				want, ws, err := binReplay(t, normalized, workers, opts...)
				if c.Original > c.Captured {
					require.ErrorContains(t, err, "capture contains truncated packets")
				} else {
					require.NoError(t, err)
				}
				canonicalErr := err
				seen = nil
				got, gs, err := binReplay(t, raw, workers, append(opts, WithLegacyPcapLengthNormalization(true))...)
				if canonicalErr != nil {
					require.EqualError(t, err, canonicalErr.Error())
				} else {
					require.NoError(t, err)
				}
				require.Len(t, got, 2)
				require.Equal(t, ws, gs)
				require.Zero(t, gs.BufferedBytes)
				for i, e := range got {
					wf, we := want[i].GetFields()
					gf, ge := e.GetFields()
					if c.Original > c.Captured && i == 0 {
						require.EqualError(t, we, "protocol parser: event is incomplete, not an exact message")
						require.EqualError(t, ge, we.Error())
						require.Nil(t, wf)
						require.Nil(t, gf)
						require.Equal(t, "incomplete", e.Status)
						require.Equal(t, "truncated or invalid UDP datagram", e.Summary)
						require.Empty(t, e.Protocol)
						require.Nil(t, e.Structured)
						require.Nil(t, e.Session)
						require.Zero(t, e.TransactionID)
						require.Zero(t, e.ResponseTo)
					} else {
						require.NoError(t, we)
						require.NoError(t, ge)
						wantJSON, err := json.Marshal(wf)
						require.NoError(t, err)
						var whole map[string]any
						require.NoError(t, json.Unmarshal(wantJSON, &whole))
						rocEqualFields(t, whole, gf)
						require.Equal(t, "dns", e.Protocol)
					}
					require.Equal(t, want[i].Raw, e.Raw)
					require.Equal(t, want[i].Session, e.Session)
					require.Equal(t, want[i].TransactionID, e.TransactionID)
					require.Equal(t, want[i].ResponseTo, e.ResponseTo)
					require.Equal(t, want[i].Direction, e.Direction)
					require.Equal(t, want[i].SourceBytes, e.SourceBytes)
					require.Equal(t, want[i].Error, e.Error)
					require.Equal(t, want[i].ID, e.ID)
					require.Equal(t, want[i].FlowID, e.FlowID)
					require.Equal(t, want[i].Status, e.Status)
					require.Equal(t, want[i].Summary, e.Summary)
				}
				if observe {
					require.Len(t, seen, 2)
					if c.Original < c.Captured {
						require.Contains(t, seen[0].AncillaryData, PcapOriginalLength(c.Original))
					}
					require.NotContains(t, seen[1].AncillaryData, PcapOriginalLength(c.Original))
				}
			})
		})
	}
}
func TestClassicPcapLegacyLengthSourceBoundary(t *testing.T) {
	ng := binTestPcap(t, []tcpStep{{seq: 99, syn: true}, {seq: 100, data: string(binMQTTConnect)}}, 1883, false, true)
	want, ws, err := binReplay(t, ng, 1)
	require.NoError(t, err)
	got, gs, err := binReplay(t, ng, 1, WithLegacyPcapLengthNormalization(true))
	require.NoError(t, err)
	require.Equal(t, ws, gs)
	require.Len(t, got, len(want))
	require.NotEmpty(t, got)
	for i, e := range got {
		require.Equal(t, want[i].Raw, e.Raw)
		require.Equal(t, want[i].SourceBytes, e.SourceBytes)
		require.Equal(t, want[i].Domain, e.Domain)
	}

	require.ErrorContains(t, Start(WithLegacyPcapLengthNormalization(true)), "requires offline replay")
	_, err = NewPacketAnalyzer(WithLegacyPcapLengthNormalization(true))
	require.Error(t, err)
	for _, snap := range []uint32{0, 0xffffffff} {
		_, err := NewBoundedPcapReaderWithOptions(bytes.NewReader(classicFixture(binary.LittleEndian, false, snap)), PcapReaderOptions{NormalizeLegacyOriginalLength: true})
		require.Error(t, err)
	}
}

func binaryOrder(s string) binary.ByteOrder {
	if s == ">" {
		return binary.BigEndian
	}
	return binary.LittleEndian
}
