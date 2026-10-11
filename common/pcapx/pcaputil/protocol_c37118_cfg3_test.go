package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"testing"
	"time"
)

// Independent zero-channel CFG-3 grammar pinned to GPA/GSF
// 22a9aa1028c6e53d115c4ea81026055a57b803c2 ConfigurationCell3/Frame3.
func c37CFG3Control() []byte {
	b := []byte{0, 0, 0, 15, 66, 64, 0, 1, 3, 'P', 'M', 'U', 0, 7}
	b = append(b, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}...)
	b = append(b, make([]byte, 8)...)
	b = append(b, []byte{0x41, 0x20, 0, 0, 0xc1, 0x20, 0, 0, 0x7f, 0x80, 0, 0, 'M', 0, 0, 0, 10, 255, 255, 255, 254, 0, 1, 0, 9, 0, 50}...)
	return c37frame(5, 2, 7, b)
}
func TestC37118CFG3CompleteConfigurationAndData(t *testing.T) {
	cfg := c37CFG3Control()
	m, err := stream_parser.DecodeC37118Message(cfg, nil, 4096)
	require.NoError(t, err)
	require.True(t, m.BodyDecoded, "valid native CFG-3 is not an opaque configuration")
	require.NotNil(t, m.Configuration)
	for _, transport := range []string{"tcp", "udp"} {
		t.Run(transport, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(39300, 28400))
			require.NoError(t, err)
			defer s.Close("test")
			r := s.Feed(0, time.Unix(1, 0), cfg)
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			f, err := r.Events[0].GetFields()
			require.NoError(t, err)
			require.Equal(t, "CFG-3", f["Packet Name"])
			require.Contains(t, f, "Configuration")
			r = s.Feed(0, time.Unix(2, 0), c37frame(0, 2, 7, []byte{0, 0, 0, 5, 255, 254}))
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			require.Contains(t, r.Events[0].Session, "Data")
			// A reverse publisher must not borrow CFG-3; a same-direction CFG-2
			// subsequently replaces it without any extended fields leaking into CFG-2.
			r = s.Feed(1, time.Unix(3, 0), c37frame(0, 2, 7, []byte{0, 0, 0, 5, 255, 254}))
			require.NotNil(t, r.Err)
			require.Equal(t, ErrContextRequired, r.Err.Kind)
			r = s.Feed(0, time.Unix(4, 0), c37ControlCFG(2, 7, 0))
			require.Nil(t, r.Err)
			r = s.Feed(0, time.Unix(5, 0), c37frame(0, 2, 7, []byte{0, 0, 0, 5, 255, 254}))
			require.Nil(t, r.Err)
			s.Close("test")
			s.Close("again")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
func TestC37118CFG3RefusalClearsContext(t *testing.T) {
	for _, transport := range []string{"tcp", "udp"} {
		t.Run(transport, func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxCollectionElements = 1
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport(transport), WithSessionPorts(39300, 28400))
			require.NoError(t, err)
			defer s.Close("test")
			cfg := c37CFG3Control()
			r := s.Feed(0, time.Unix(1, 0), cfg)
			require.Nil(t, r.Err)
			body := append([]byte(nil), cfg[14:len(cfg)-4]...)
			binary.BigEndian.PutUint16(body[6:], 2)
			body = append(body, cfg[22:len(cfg)-4]...)
			body = append(body, 0, 50)
			r = s.Feed(0, time.Unix(2, 0), c37frame(5, 2, 7, body))
			require.NotNil(t, r.Err)
			require.Equal(t, ErrResourceExceeded, r.Err.Kind)
			r = s.Feed(0, time.Unix(3, 0), c37frame(0, 2, 7, []byte{0, 0, 0, 5, 255, 254}))
			require.NotNil(t, r.Err)
			require.Equal(t, ErrContextRequired, r.Err.Kind)
		})
	}
}

func TestC37118CFG3TCPChunkBoundaries(t *testing.T) {
	b, err := trafficfixture.ReadFile("c37118-cfg3/controls.json")
	require.NoError(t, err)
	var controls struct {
		Cases []struct {
			ID, Carrier, Answer string
			Limit               int `json:"collection_limit"`
		}
	}
	require.NoError(t, json.Unmarshal(b, &controls))
	for _, c := range controls.Cases {
		if c.Carrier != "tcp" {
			continue
		}
		ab, err := trafficfixture.ReadFile("c37118-cfg3/" + c.Answer)
		require.NoError(t, err)
		var answer struct {
			Events []struct {
				Raw, Error, Profile string
				Direction           int
				Fields              map[string]any
			}
		}
		require.NoError(t, json.Unmarshal(ab, &answer))
		for _, chunk := range []int{1, 3, 14, 16, 255} {
			t.Run(fmt.Sprintf("%s/chunk%d", c.ID, chunk), func(t *testing.T) {
				b := DefaultParserBudget()
				b.MaxCollectionElements = c.Limit
				s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("tcp"), WithSessionPorts(39300, 28400))
				require.NoError(t, err)
				defer s.Close("test")
				for i, w := range answer.Events {
					wire, err := hex.DecodeString(w.Raw)
					require.NoError(t, err)
					var events []*ProtocolEvent
					var feedError *ProtocolError
					for at := 0; at < len(wire); {
						end := min(at+chunk, len(wire))
						part := append([]byte(nil), wire[at:end]...)
						r := s.Feed(w.Direction, time.Unix(int64(i+1), 0), part)
						clear(part)
						events = append(events, r.Events...)
						if end == len(wire) {
							feedError = r.Err
						}
						if end < len(wire) {
							require.Empty(t, r.Events)
							require.NotNil(t, r.Err)
							require.Equal(t, ErrNeedMore, r.Err.Kind)
						}
						at = end
					}
					require.Len(t, events, 1)
					e := events[0]
					require.Equal(t, w.Profile, e.Profile)
					require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
					require.Zero(t, e.ResponseTo)
					require.Zero(t, e.TransactionID)
					if w.Error != "" {
						rocTypedError(t, w.Error, feedError)
						rocEqualFields(t, w.Fields, e.Session)
						continue
					}
					require.Nil(t, feedError)
					f, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, w.Fields, f)
					if cfg, ok := e.Session["Configuration"].(map[string]any); ok {
						cfg["PMUs"] = nil
					}
					d, err := e.Decode()
					require.NoError(t, err)
					rocEqualFields(t, w.Fields, d["session"].(map[string]any))
				}
				s.Close("test")
				s.Close("again")
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}
