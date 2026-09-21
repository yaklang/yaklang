package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

func TestFirstBatchT00(t *testing.T) {
	raw, err := os.ReadFile("testdata/protocol-sessions/m1-manifest.json")
	require.NoError(t, err)
	var doc struct {
		Schema  int `json:"schema_version"`
		Samples []struct {
			File, Protocol, SHA256, Representation, Transport, Completeness string
			Native                                                          bool `json:"native_carrier"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Equal(t, 2, doc.Schema)
	require.Len(t, doc.Samples, 39) // M1 manifest subset; additional real captures have separate provenance.
	for _, sample := range doc.Samples {
		t.Run(sample.File, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata/protocol-sessions", sample.File))
			require.NoError(t, err)
			hash := sha256.Sum256(raw)
			require.Equal(t, sample.SHA256, hex.EncodeToString(hash[:]))
			require.Equal(t, "synthetic_frame", sample.Representation)
			require.NotEmpty(t, sample.Completeness)
			r, err := pcapgo.NewReader(bytes.NewReader(raw))
			require.NoError(t, err)
			records := 0
			for {
				data, _, err := r.ReadPacketData()
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
				records++
				p := gopacket.NewPacket(data, r.LinkType(), gopacket.Default)
				switch sample.Transport {
				case "tcp":
					require.NotNil(t, p.Layer(layers.LayerTypeTCP))
				case "udp":
					require.NotNil(t, p.Layer(layers.LayerTypeUDP))
				default:
					t.Fatalf("unregistered carrier %q", sample.Transport)
				}
			}
			require.Positive(t, records)
			switch sample.Protocol {
			case "quic", "http3", "doq", "goose", "rtp":
				require.False(t, sample.Native, "TCP wrapper cannot establish native wire coverage")
			}
		})
	}
}

func TestFirstBatchT00CapabilityEvidence(t *testing.T) {
	raw, err := os.ReadFile("testdata/protocol-sessions/first-batch-capabilities.json")
	require.NoError(t, err)
	var doc struct {
		Profiles []json.RawMessage         `json:"profiles"`
		Tasks    []struct{ Complete bool } `json:"task_progress"`
		Evidence []struct {
			Sample, SHA256 string
			OracleFile     string `json:"oracle_file"`
			OracleSHA256   string `json:"oracle_sha256"`
		} `json:"added_evidence"`
	}
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Len(t, doc.Profiles, 39)
	require.Len(t, doc.Tasks, 22)
	for _, task := range doc.Tasks {
		require.False(t, task.Complete, "M0 is not full first-batch acceptance")
	}
	require.Len(t, doc.Evidence, 1)
	for _, e := range doc.Evidence {
		for path, want := range map[string]string{e.Sample: e.SHA256, e.OracleFile: e.OracleSHA256} {
			b, err := os.ReadFile(filepath.Join("testdata/protocol-sessions", path))
			require.NoError(t, err)
			sum := sha256.Sum256(b)
			require.Equal(t, want, hex.EncodeToString(sum[:]))
		}
	}
}

func firstBatchNG(order binary.ByteOrder, kind uint32, body []byte) []byte {
	var b bytes.Buffer
	for _, x := range []uint32{kind, uint32(len(body) + 12)} {
		requireWrite := binary.Write(&b, order, x)
		if requireWrite != nil {
			panic(requireWrite)
		}
	}
	b.Write(body)
	_ = binary.Write(&b, order, uint32(len(body)+12))
	return b.Bytes()
}
func firstBatchNGPrefix(order binary.ByteOrder) []byte {
	b := make([]byte, 16)
	order.PutUint32(b, 0x1a2b3c4d)
	order.PutUint16(b[4:], 1)
	order.PutUint64(b[8:], ^uint64(0))
	out := firstBatchNG(order, 0x0a0d0d0a, b)
	b = make([]byte, 8)
	order.PutUint16(b, 1)
	order.PutUint32(b[4:], 65535)
	return append(out, firstBatchNG(order, 1, b)...)
}
func TestFirstBatchT01(t *testing.T) {
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		t.Run(fmt.Sprint(order), func(t *testing.T) {
			for _, kind := range []uint32{2, 3, 6} {
				t.Run(fmt.Sprint(kind), func(t *testing.T) {
					fixed := 20
					if kind == 3 {
						fixed = 4
					}
					body := make([]byte, fixed+4)
					if kind == 3 {
						order.PutUint32(body, 3)
					} else {
						order.PutUint32(body[12:], 3)
						order.PutUint32(body[16:], 3)
					}
					copy(body[fixed:], []byte{1, 2, 3})
					raw := append(firstBatchNGPrefix(order), firstBatchNG(order, kind, body)...)
					r, err := NewBoundedNgReader(iotest.OneByteReader(bytes.NewReader(raw)), pcapgo.NgReaderOptions{})
					require.NoError(t, err)
					data, ci, err := r.ReadPacketData()
					require.NoError(t, err)
					require.Equal(t, []byte{1, 2, 3}, data)
					require.Equal(t, 3, ci.CaptureLength)
					_, _, err = r.ReadPacketData()
					require.ErrorIs(t, err, io.EOF)
				})
			}
			for _, tc := range []struct {
				name         string
				caplen, orig uint32
				extra        []byte
			}{
				{"32MiB-in-empty-block", 32 << 20, 32 << 20, nil}, {"caplen-over-orig", 4, 3, make([]byte, 4)},
				{"truncated-options", 0, 0, []byte{1, 0, 255, 255}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					b := make([]byte, 20)
					order.PutUint32(b[12:], tc.caplen)
					order.PutUint32(b[16:], tc.orig)
					b = append(b, tc.extra...)
					raw := append(firstBatchNGPrefix(order), firstBatchNG(order, 6, b)...)
					guard := &boundedNgInput{input: bytes.NewReader(raw)}
					_, err := io.ReadAll(guard)
					require.Error(t, err)
					require.LessOrEqual(t, cap(guard.block), 64, "invalid caplen must be rejected before growing the block buffer")
					require.Error(t, ReplayPcap(bytes.NewReader(raw)))
				})
			}
			t.Run("trailer", func(t *testing.T) {
				raw := firstBatchNGPrefix(order)
				raw[len(raw)-1] ^= 1
				_, err := io.ReadAll(&boundedNgInput{input: bytes.NewReader(raw)})
				require.Error(t, err)
			})
			for _, code := range []uint16{9, 11, 14} {
				t.Run(fmt.Sprintf("short-option-%d", code), func(t *testing.T) {
					b := make([]byte, 12)
					order.PutUint16(b, 1)
					order.PutUint32(b[4:], 65535)
					order.PutUint16(b[8:], code)
					raw := append(firstBatchNGPrefix(order)[:28], firstBatchNG(order, 1, b)...)
					_, err := NewBoundedNgReader(bytes.NewReader(raw), pcapgo.NgReaderOptions{})
					require.Error(t, err)
				})
			}
		})
	}
}

func TestFirstBatchT07(t *testing.T) {
	for _, n := range []int{16, 63, 127} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			wire := bytes.Repeat([]byte{1, 'a'}, n)
			wire = append(wire, 0)
			name, next, err := dnsParseName(wire, 0)
			require.NoError(t, err)
			require.Equal(t, len(wire), next)
			require.Equal(t, strings.TrimSuffix(strings.Repeat("a.", n), "."), name)
		})
	}
	for _, wire := range [][]byte{{0xc0, 0}, {0xc0, 2, 0xc0, 0}, {64, 1, 0}, {0xc0, 255}, append(bytes.Repeat([]byte{1, 'a'}, 128), 0)} {
		_, _, err := dnsParseName(wire, 0)
		require.Error(t, err)
	}
	name, next, err := dnsParseName([]byte{1, 'a', 0, 0xc0, 0}, 3)
	require.NoError(t, err)
	require.Equal(t, "a", name)
	require.Equal(t, 5, next)
}
func TestFirstBatchT14(t *testing.T) {
	for _, wire := range []string{":+1\r\n", ":-1\r\n", ":9223372036854775807\r\n", "(+123456789012345678901234567890\r\n", "(-999999999999999999999\r\n", "$-1\r\n", "*2\r\n:+1\r\n:-1\r\n"} {
		t.Run(wire, func(t *testing.T) {
			for i := 0; i < len(wire); i++ {
				n, err := redisFrameLength([]byte(wire[:i]), 0)
				require.NoError(t, err, "prefix %d", i)
				require.Zero(t, n)
			}
			n, err := redisFrameLength([]byte(wire), 0)
			require.NoError(t, err)
			require.Equal(t, len(wire), n)
		})
	}
	for _, wire := range []string{"$+1\r\nx\r\n", ":+\r\n", ":9223372036854775808\r\n", ":--1\r\n", ":1\rx", "(1x\r\n"} {
		_, err := redisFrameLength([]byte(wire), 0)
		require.Error(t, err)
	}
}
func TestFirstBatchT12(t *testing.T) {
	q := &binQUIC{}
	missing := quicLongPacket(2, 1, quicTestDCID(), quicTestSCID(), nil, 0, []byte{1})
	info, err := q.consume(0, missing, 64)
	require.Error(t, err)
	require.Equal(t, true, info["Missing Keys"])
	require.Nil(t, info["Frames"])
	require.Nil(t, q.keys)
	require.Empty(t, q.state)
	bad := append([]byte(nil), rfc9001ClientInitial()...)
	bad[len(bad)-1] ^= 1
	info, err = q.consume(0, bad, 64)
	require.Error(t, err)
	require.Equal(t, ErrAuthenticationFailed, err.(*ProtocolError).Kind)
	require.Nil(t, q.keys)
	require.Empty(t, q.initialDCID)
	require.False(t, q.spaces[0][0].init)
	require.Empty(t, q.streams)
	info, err = q.consume(0, rfc9001ClientInitial(), 64)
	require.NoError(t, err)
	require.Equal(t, true, info["Authentication Verified"])
	require.Equal(t, uint64(2), info["Packet Number"])
	before := q.spaces[0][0].largest
	_, err = q.consume(0, bad, 64)
	require.Error(t, err)
	require.Equal(t, before, q.spaces[0][0].largest)
	_, err = NewDecryptedQUICSession(DefaultParserBudget(), "")
	require.Error(t, err)
}
func TestFirstBatchT03(t *testing.T) {
	// RFC9001 A.2/A.3 protected bytes, unmodified, in a synthetic native UDP
	// carrier. This is a standard-vector test, not a live-server capture claim.
	steps := []sessionStep{{0, append(append([]byte(nil), rfc9001ClientInitial()...), rfc9001ClientInitial()...)}, {1, rfc9001ServerInitial()}}
	raw := sessionDatagramPCAP(t, steps, layers.UDPPort(14443))
	path := "testdata/protocol-sessions/quic-rfc9001-native-udp.pcap"
	if os.Getenv("YAK_UPDATE_SESSION_PCAP") == "1" {
		require.NoError(t, os.WriteFile(path, raw, 0644))
	}
	stored, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, raw, stored)
	for _, workers := range []int{1, 2, 4} {
		t.Run(fmt.Sprint(workers), func(t *testing.T) {
			events, stats, err := binReplay(t, raw, workers)
			require.NoError(t, err)
			require.Len(t, events, 3)
			for i, e := range events {
				require.Equal(t, "udp", e.Transport)
				require.Equal(t, "quic", e.Protocol)
				require.Equal(t, true, e.Session["Decrypted"])
				require.Equal(t, true, e.Session["Authentication Verified"])
				require.Equal(t, false, e.Session["Peer Identity Verified"])
				require.Equal(t, "decoded", e.Status)
				require.NotEmpty(t, e.Session["Frames"])
				if i < 2 {
					require.Equal(t, 0, e.Direction)
				} else {
					require.Equal(t, 1, e.Direction)
				}
			}
			require.Equal(t, true, events[1].Session["Duplicate"])
			require.Equal(t, len(rfc9001ClientInitial()), events[1].Session["Datagram Offset"])
			require.Zero(t, stats.BufferedBytes)
			deferred, ds, err := binReplay(t, raw, workers, WithBinParserDeferred(true))
			require.NoError(t, err)
			require.Len(t, deferred, len(events))
			for i := range events {
				require.Equal(t, events[i].Session, deferred[i].Session)
				require.Equal(t, events[i].Raw, deferred[i].Raw)
				require.Equal(t, "deferred", deferred[i].Status)
				decoded, err := deferred[i].Decode()
				require.NoError(t, err)
				full, err := events[i].Decode()
				require.NoError(t, err)
				require.Equal(t, full, decoded)
			}
			require.Equal(t, stats.Messages, ds.Messages)
			require.Zero(t, ds.BufferedBytes)
		})
	}
}
func FuzzFirstBatchM0(f *testing.F) {
	for _, w := range [][]byte{[]byte(":+1\r\n"), {1, 'a', 0}, {0xc0, 0}, rfc9001ClientInitial()} {
		f.Add(w)
	}
	f.Fuzz(func(t *testing.T, w []byte) {
		if len(w) > 4096 {
			return
		}
		_, _ = redisFrameLength(w, 0)
		_, _, _ = dnsParseName(w, 0)
		_, _ = (&binQUIC{}).consume(0, w, 64)
	})
}

func TestFirstBatchT12DirectionalState(t *testing.T) {
	q := &binQUIC{decryptedSource: "synthetic direction regression"}
	for _, dir := range []int{0, 1} {
		pkt := quicLongPacket(0, 1, quicTestDCID(), quicTestSCID(), nil, 0, []byte{6, 0, 1, 'x'})
		info, err := q.consume(dir, pkt, 64)
		require.NoError(t, err)
		require.Nil(t, info["Duplicate"])
		require.Nil(t, info["Retransmission"])
	}
	for _, name := range []string{"STREAM", "HANDSHAKE_DONE", "CONNECTION_CLOSE_APP", "RESET_STREAM"} {
		require.Error(t, quicValidateProtectedFrames(quicSpaceInitial, []map[string]any{{"Frame Type": name}}))
		require.Error(t, quicValidateProtectedFrames(quicSpaceHandshake, []map[string]any{{"Frame Type": name}}))
	}
	require.NoError(t, quicValidateProtectedFrames(quicSpaceInitial, []map[string]any{{"Frame Type": "CRYPTO"}, {"Frame Type": "ACK"}}))
	q1, q2 := &binQUIC{}, &binQUIC{}
	_, err := q1.consume(0, rfc9001ClientInitial(), 64)
	require.NoError(t, err)
	require.Nil(t, q2.keys)
	require.Empty(t, q2.initialDCID)
}

func TestFirstBatchT03BadTagAndBudget(t *testing.T) {
	bad := append([]byte(nil), rfc9001ClientInitial()...)
	bad[len(bad)-1] ^= 1
	raw := sessionDatagramPCAP(t, []sessionStep{{0, bad}, {0, rfc9001ClientInitial()}}, layers.UDPPort(14443))
	events, stats, err := binReplay(t, raw, 1)
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, "malformed", events[0].Status)
	require.Contains(t, events[0].Error, string(ErrAuthenticationFailed))
	require.Zero(t, events[0].FlowID)
	require.Nil(t, events[0].Session["Frames"])
	require.Equal(t, "decoded", events[1].Status)
	require.Nil(t, events[1].Session["Duplicate"])
	require.Zero(t, stats.BufferedBytes)
	// A deliberately tiny shared budget must reject rather than retain QUIC state.
	events, stats, err = binReplay(t, raw, 1, func(c *CaptureConfig) error {
		c.binParserConfig.MaxBufferedBytes = 2048
		c.binParserConfig.MaxMessageBytes = 2048
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, events)
	require.Zero(t, stats.BufferedBytes)
	for _, e := range events {
		require.NotEqual(t, "decoded", e.Status)
	}
}
