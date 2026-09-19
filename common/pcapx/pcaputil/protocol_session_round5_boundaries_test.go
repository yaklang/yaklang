package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRound5CaptureProvenance(t *testing.T) {
	for _, tc := range []struct {
		root, manifest string
		new            bool
	}{{"testdata/protocol-sessions/upstream", "manifest.json", true}, {"../../bin-parser/testdata/protocol-corpus", "manifest.json", false}} {
		raw, err := os.ReadFile(filepath.Join(tc.root, tc.manifest))
		require.NoError(t, err)
		var m struct {
			Captures []struct {
				File    string `json:"file"`
				Capture string `json:"capture_file"`
				ID      string `json:"id"`
				SHA     string `json:"sha256"`
				Size    int    `json:"size_bytes"`
				Source  string `json:"source_url"`
				License string `json:"license_file"`
			}
		}
		require.NoError(t, json.Unmarshal(raw, &m))
		found := 0
		wanted := map[string]bool{"ndpi-stun": true, "ndpi-tftp": true, "ndpi-rtsp-http": true, "ndpi-vnc": true, "ndpi-ipp": true, "ndpi-diameter": true, "ndpi-s7comm": true, "ndpi-opcua": true, "ndpi-iec104": true, "ws-opcua-signed": true}
		for _, c := range m.Captures {
			if !tc.new && !wanted[c.ID] {
				continue
			}
			file := c.File
			if file == "" {
				file = c.Capture
			}
			b, err := os.ReadFile(filepath.Join(tc.root, file))
			require.NoError(t, err)
			sum := sha256.Sum256(b)
			require.Equal(t, c.SHA, hex.EncodeToString(sum[:]), file)
			require.Equal(t, c.Size, len(b))
			require.Contains(t, c.Source, "https://")
			if c.License != "" {
				_, err = os.Stat(filepath.Join(tc.root, c.License))
				require.NoError(t, err)
			}
			found++
		}
		if tc.new {
			require.Equal(t, 4, found)
		} else {
			require.Equal(t, len(wanted), found)
		}
	}
}
func TestRound5SnapshotOwnsNestedAttributes(t *testing.T) {
	source := map[string]any{"Attributes": []any{map[string]any{"Data": []byte{1, 2}}}, "Encodings": []int32{0, 16}}
	copy := cloneSession(source)
	copy["Attributes"].([]any)[0].(map[string]any)["Data"].([]byte)[0] = 9
	copy["Encodings"].([]int32)[0] = 7
	require.Equal(t, byte(1), source["Attributes"].([]any)[0].(map[string]any)["Data"].([]byte)[0])
	require.Equal(t, int32(0), source["Encodings"].([]int32)[0])
	require.Greater(t, sessionSnapshotBytes(source), sessionSnapshotBytes(map[string]any{"Attributes": []any{}}))
}
func TestTURNMultiplePermissionsAndTransactionalBudget(t *testing.T) {
	s := &binSTUN{}
	ts := time.Unix(1, 0)
	peer := func(last byte) []byte {
		return stunTestAttr(0x12, []byte{0, 1, 0x21, 0x13, 0xe1, 0x12, 0xa6, last ^ 0x42})
	}
	request := stunTestMessage(8, 0, 1, peer(1), peer(2))
	_, err := s.consume(0, ts, request, 4, false)
	require.NoError(t, err)
	_, err = s.consume(1, ts, stunTestMessage(8, 2, 1), 1, false)
	require.Error(t, err)
	require.Empty(t, s.permissions)
	require.Len(t, s.pending, 1)
	_, err = s.consume(1, ts, stunTestMessage(8, 2, 1), 4, false)
	require.NoError(t, err)
	require.Len(t, s.permissions, 2)
	s.expire(ts.Add(301 * time.Second))
	require.Empty(t, s.permissions)
	_, err = s.consume(0, ts, stunTestMessage(3, 0, 2), 4, false)
	require.NoError(t, err)
	_, err = s.consume(1, ts, stunTestMessage(3, 2, 2), 4, false)
	require.Error(t, err)
}
func TestTFTPRolloverAndEmptyFinalBlock(t *testing.T) {
	s := &binTFTP{}
	_, err := s.consume(0, []byte("\x00\x01f\x00octet\x00"), 8)
	require.NoError(t, err)
	s.expected = 65535
	data := append([]byte{0, 3, 255, 255}, make([]byte, 512)...)
	_, err = s.consume(1, data, 8)
	require.NoError(t, err)
	_, err = s.consume(0, []byte{0, 4, 255, 255}, 8)
	require.NoError(t, err)
	_, err = s.consume(1, []byte{0, 3, 0, 0}, 8)
	require.NoError(t, err)
	out, err := s.consume(1, []byte{0, 3, 0, 0}, 8)
	require.NoError(t, err)
	require.Equal(t, true, out["Retransmission"])
	out, err = s.consume(0, []byte{0, 4, 0, 0}, 8)
	require.NoError(t, err)
	require.Equal(t, true, out["Transfer Complete"])
}
func TestRound5UDPExpiryAndBudget(t *testing.T) {
	c := NewDefaultConfig()
	require.NoError(t, WithBinParserConfig(BinParserConfig{OnEvent: func(*ProtocolEvent) {}})(c))
	require.NoError(t, c.prepareBinParser())
	a := c.binParser
	a.budget.MaxCollectionElements = 1
	feed := func(src string, ts int64) *ProtocolEvent {
		e := &ProtocolEvent{Source: src, Destination: "192.0.2.2:3478", Timestamp: time.Unix(ts, 0)}
		require.True(t, a.decodeSTUNDatagram(e, stunTestMessage(1, 0, 1)))
		return e
	}
	first := feed("192.0.2.1:1111", 1)
	require.Empty(t, first.Error)
	limited := feed("192.0.2.1:2222", 2)
	require.Equal(t, "limited", limited.Status)
	next := feed("192.0.2.1:2222", 602)
	require.Empty(t, next.Error)
	require.NotEqual(t, first.FlowID, next.FlowID)
	require.Len(t, a.udpSessions.entries, 1)
	require.NoError(t, c.finishBinParser())
	require.Zero(t, a.stats().BufferedBytes)
}
func TestS7ReadErrorItems(t *testing.T) {
	w, err := hex.DecodeString("0300002902f080320300000a0000020014000004050a0000040a0000040a0000040a0000040a000004")
	require.NoError(t, err)
	out, err := (&binS7{}).consume(1, w, 8, 512)
	require.NoError(t, err)
	items := out["Data Items"].([]map[string]any)
	require.Len(t, items, 5)
	for _, item := range items {
		require.Equal(t, byte(10), item["Return Code"])
		require.Empty(t, item["Value"])
	}
}
func uaTestChunk(kind string, chunk byte, channel, token, seq, req uint32, body []byte) []byte {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint32(b, channel)
	binary.LittleEndian.PutUint32(b[4:], token)
	binary.LittleEndian.PutUint32(b[8:], seq)
	binary.LittleEndian.PutUint32(b[12:], req)
	w := uaTestEnvelope(kind, append(b, body...))
	w[3] = chunk
	return w
}
func TestOPCUAChunkAbortAndLimits(t *testing.T) {
	s := &binOPCUA{}
	_, err := s.consume(0, uaTestEnvelope("MSG", make([]byte, 16)), 8, 128)
	require.NoError(t, err)
	s.plain[1] = true
	first := uaTestChunk("MSG", 'C', 1, 1, 1, 1, []byte{1, 0})
	_, err = s.consume(0, first, 8, 128)
	require.NoError(t, err)
	out, err := s.consume(0, uaTestChunk("MSG", 'F', 1, 1, 2, 1, []byte{0x77, 2}), 8, 128)
	require.NoError(t, err)
	require.Equal(t, true, out["Reassembled"])
	require.Equal(t, uint32(631), out["Service Type ID"])
	require.Empty(t, s.chunks)
	_, err = s.consume(0, uaTestChunk("MSG", 'C', 1, 1, 3, 2, []byte{1, 0}), 8, 128)
	require.NoError(t, err)
	_, err = s.consume(0, uaTestChunk("MSG", 'A', 1, 1, 4, 2, make([]byte, 8)), 8, 128)
	require.NoError(t, err)
	require.Empty(t, s.chunks)
	_, err = s.consume(0, uaTestChunk("MSG", 'C', 1, 1, 5, 3, bytes.Repeat([]byte{1}, 129)), 8, 128)
	require.Error(t, err)
	require.Empty(t, s.chunks)
}
func TestRound5NewProfilesReleaseOnLimit(t *testing.T) {
	for _, tc := range []struct {
		name string
		wire []byte
	}{{"diameter", diameterTestMessage(true, 1, nil)}, {"iec104", []byte{0x68, 4, 7, 0, 0, 0}}, {"opcua", uaTestEnvelope("ACK", make([]byte, 20))}, {"vnc", []byte("RFB 003.008\n")}, {"rtsp", []byte("OPTIONS * RTSP/1.0\r\nCSeq: 1\r\n\r\n")}} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := NewProtocolSession(ParserBudget{MaxBufferedBytes: 64})
			require.NoError(t, err)
			r := s.Feed(0, time.Time{}, tc.wire)
			require.NotNil(t, r.Err)
			s.Close("limit")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
func FuzzRound5ProtocolBoundaries(f *testing.F) {
	for _, seed := range [][]byte{stunTestMessage(1, 0, 1), diameterTestMessage(true, 1, nil), {0x68, 4, 7, 0, 0, 0}, []byte("\x00\x01f\x00octet\x00"), []byte("RFB 003.008\n"), ippTestWire(1), uaTestEnvelope("MSG", make([]byte, 16))} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, w []byte) {
		if len(w) > 4096 {
			t.Skip()
		}
		for dir := 0; dir < 2; dir++ {
			_, _ = (&binSTUN{}).consume(dir, time.Time{}, w, 32, false)
			_, _ = (&binTFTP{}).consume(dir, w, 32)
			_, _ = (&binDiameter{}).consume(dir, w, 32, 8)
			_, _ = (&binIEC104{}).consume(dir, w, 32)
			_, _ = (&binS7{}).consume(dir, w, 32, 4096)
			_, _ = (&binOPCUA{}).consume(dir, w, 32, 4096)
			_, _ = (&binRFB{server: 0, phase: "server-version"}).consume(dir, w, 32, 4096)
			_, _ = (&binRFB{server: 0, phase: "messages", bpp: 4}).consume(dir, w, 32, 4096)
			_, _ = ippFields(w, 32, 8)
			_, _ = (&binRTSP{}).consume(dir, w, time.Time{}, 32)
		}
	})
}
