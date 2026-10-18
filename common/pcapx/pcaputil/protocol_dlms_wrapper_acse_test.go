package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const acseRequestHex = "601da109060760857405080101be10040e01000000065f1f04000000100400"
const acseResponseHex = "6129a109060760857405080101a203020100a305a103020100be10040e0800065f1f040000001004000007"

func acseTestWire(t *testing.T, src, dst uint16, s string) []byte {
	t.Helper()
	p, e := hex.DecodeString(s)
	require.NoError(t, e)
	w := make([]byte, 8)
	binary.BigEndian.PutUint16(w, 1)
	binary.BigEndian.PutUint16(w[2:], src)
	binary.BigEndian.PutUint16(w[4:], dst)
	binary.BigEndian.PutUint16(w[6:], uint16(len(p)))
	return append(w, p...)
}
func TestDLMSACSEExistingAPIReferencePair(t *testing.T) {
	s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
	require.NoError(t, e)
	defer s.Close("test")
	q := s.Feed(0, time.Unix(1, 0), acseTestWire(t, 1, 16, acseRequestHex))
	require.Nil(t, q.Err)
	require.Len(t, q.Events, 1)
	r := s.Feed(1, time.Unix(1, 0), acseTestWire(t, 16, 1, acseResponseHex))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	require.Equal(t, q.Events[0].ID, r.Events[0].ResponseTo)
	require.Equal(t, q.Events[0].ID, r.Events[0].TransactionID)
	f, e := r.Events[0].GetFields()
	require.NoError(t, e)
	require.Equal(t, "AARE", f["kind"])
	require.Equal(t, uint32(524288), f["conformance_bits"])
	require.Equal(t, uint16(1024), f["max_pdu_size"])
	require.Equal(t, byte(6), f["dlms_version"])
	require.Equal(t, uint16(7), f["vaa_name"])
	require.Equal(t, false, f["authentication_verified"])
	require.Equal(t, "observed-association-response", r.Events[0].Session["Association"])
	require.Empty(t, s.Close("test"))
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestDLMSACSEGenerationDirectionAndSelection(t *testing.T) {
	for _, c := range []struct {
		name     string
		dup      bool
		src, dst uint16
		response string
		want     string
		linked   bool
	}{
		{"accepted", false, 16, 1, acseResponseHex, "observed-association-response", true},
		{"request-reuse", true, 16, 1, acseResponseHex, "ambiguous-association-generation", false},
		{"wrong-address", false, 17, 1, acseResponseHex, "unmatched-association-response", false},
		{"unproposed-conformance", false, 16, 1, strings.Replace(acseResponseHex, "0000100400", "0000300400", 1), "unverified-conformance-selection", false},
		{"rejected-no-information", false, 16, 1, "6117a109060760857405080101a203020101a305a103020100", "observed-association-response", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, e)
			defer s.Close("test")
			q := s.Feed(0, time.Unix(1, 0), acseTestWire(t, 1, 16, acseRequestHex))
			require.Nil(t, q.Err)
			require.Len(t, q.Events, 1)
			if c.dup {
				d := s.Feed(0, time.Unix(1, 0), acseTestWire(t, 1, 16, acseRequestHex))
				require.Nil(t, d.Err)
				require.Zero(t, d.Events[0].TransactionID)
			}
			r := s.Feed(1, time.Unix(1, 0), acseTestWire(t, c.src, c.dst, c.response))
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			require.Equal(t, c.want, r.Events[0].Session["Association"])
			if c.linked {
				require.Equal(t, q.Events[0].ID, r.Events[0].ResponseTo)
			} else {
				require.Zero(t, r.Events[0].ResponseTo)
				require.Zero(t, r.Events[0].TransactionID)
			}
			f, e := r.Events[0].GetFields()
			require.NoError(t, e)
			require.Equal(t, false, f["authentication_verified"])
			s.Close("test")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
func TestDLMSACSEWireNegativeBoundaries(t *testing.T) {
	q, _ := hex.DecodeString(acseRequestHex)
	badContext := bytes.Clone(q)
	badContext[12] = 3
	duplicate := append(bytes.Clone(q), q[2:13]...)
	duplicate[1] += 11
	trailing := append(bytes.Clone(q), 0)
	short := bytes.Clone(q)
	short[1]++
	acceptedBadDiag, _ := hex.DecodeString(strings.Replace(acseResponseHex, "a103020100", "a103020101", 1))
	for _, c := range []struct {
		name string
		p    []byte
		want ProtocolErrorKind
	}{
		{"context-ciphered", badContext, ErrUnsupportedFeature}, {"duplicate", duplicate, ErrMalformedMessage}, {"trailing", trailing, ErrMalformedMessage}, {"outer-short", short, ErrMalformedMessage}, {"accepted-nonzero-diagnostic", acceptedBadDiag, ErrMalformedMessage}, {"indefinite", []byte{0x60, 0x80, 0, 0}, ErrUnsupportedFeature},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, e)
			defer s.Close("test")
			w := acseTestWire(t, 1, 16, hex.EncodeToString(c.p))
			r := s.Feed(0, time.Unix(1, 0), w)
			require.NotNil(t, r.Err)
			require.Equal(t, c.want, r.Err.Kind)
			require.Len(t, r.Events, 1)
			require.Nil(t, r.Events[0].Session)
			require.Zero(t, r.Events[0].ResponseTo)
			f, e := r.Events[0].GetFields()
			require.Nil(t, f)
			rocTypedError(t, string(c.want), e)
			s.Close("test")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDLMSACSESplitOwnershipAndIdle(t *testing.T) {
	for _, transport := range []string{"tcp", "udp"} {
		for _, chunk := range []int{1, 7, 65535} {
			if transport == "udp" && chunk != 65535 {
				continue
			}
			t.Run(fmt.Sprintf("%s/%d", transport, chunk), func(t *testing.T) {
				s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(40000, 4059))
				require.NoError(t, e)
				defer s.Close("test")
				var events []*ProtocolEvent
				for d, wire := range [][]byte{acseTestWire(t, 1, 16, acseRequestHex), acseTestWire(t, 16, 1, acseResponseHex)} {
					for at := 0; at < len(wire); at += chunk {
						part := bytes.Clone(wire[at:min(at+chunk, len(wire))])
						r := s.Feed(d, time.Unix(1, 0), part)
						if r.Err != nil {
							require.Equal(t, ErrNeedMore, r.Err.Kind)
							require.Empty(t, r.Events)
						}
						events = append(events, r.Events...)
						clear(part)
					}
				}
				require.Len(t, events, 2)
				require.Equal(t, events[0].ID, events[1].ResponseTo)
				f, e := events[1].GetFields()
				require.NoError(t, e)
				clear(f)
				clear(events[1].Session)
				clear(events[1].Raw)
				f, e = events[1].GetFields()
				require.NoError(t, e)
				require.Equal(t, uint32(524288), f["conformance_bits"])
				require.Equal(t, events[0].ID, f["association_request_event"])
				require.Empty(t, s.Close("test"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
	t.Run("idle-late-response", func(t *testing.T) {
		s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
		require.NoError(t, e)
		defer s.Close("test")
		q := s.Feed(0, time.Unix(1, 0), acseTestWire(t, 1, 16, acseRequestHex))
		require.Nil(t, q.Err)
		r := s.Feed(1, time.Unix(32, 0), acseTestWire(t, 16, 1, acseResponseHex))
		require.Nil(t, r.Err)
		require.Len(t, r.Events, 1)
		require.Zero(t, r.Events[0].ResponseTo)
		require.Equal(t, "ambiguous-conversation", r.Events[0].Session["Association"])
		s.Close("test")
		require.Zero(t, s.Stats().BufferedBytes)
	})
}

func TestDLMSACSEUntrackedResourceRefusalCannotOwnLateReply(t *testing.T) {
	b := DefaultParserBudget()
	b.MaxMessageBytes = 1024
	b.MaxBufferedBytes = 6000
	s, e := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
	require.NoError(t, e)
	defer s.Close("test")
	refused := s.Feed(0, time.Unix(1, 0), acseTestWire(t, 1, 16, acseRequestHex))
	require.NotNil(t, refused.Err)
	require.Equal(t, ErrResourceExceeded, refused.Err.Kind)
	require.Len(t, refused.Events, 1)
	require.Nil(t, refused.Events[0].Session)
	require.Zero(t, s.Stats().BufferedBytes)
	// Restore only this test instance's ingress credits, not a public contract or shared cache.
	s.(*captureSession).f.a.config.MaxBufferedBytes = 1 << 20
	q := s.Feed(0, time.Unix(1, 0), acseTestWire(t, 1, 16, acseRequestHex))
	require.Nil(t, q.Err)
	require.Len(t, q.Events, 1)
	require.Zero(t, q.Events[0].TransactionID)
	require.Equal(t, "ambiguous-conversation", q.Events[0].Session["Association"])
	late := s.Feed(1, time.Unix(1, 0), acseTestWire(t, 16, 1, acseResponseHex))
	require.Nil(t, late.Err)
	require.Len(t, late.Events, 1)
	require.Zero(t, late.Events[0].ResponseTo)
	require.Zero(t, late.Events[0].TransactionID)
	require.Equal(t, "ambiguous-conversation", late.Events[0].Session["Association"])
	s.Close("test")
	require.Zero(t, s.Stats().BufferedBytes)
}
func TestDLMSACSERefusalHintRequiresCompleteContext(t *testing.T) {
	q := acseTestWire(t, 1, 16, acseRequestHex)
	require.True(t, wrapperUDPRefusalHint(q))
	for _, at := range []int{1, 7, 9, 12, 17} {
		t.Run(fmt.Sprint(at), func(t *testing.T) { w := bytes.Clone(q); w[at] ^= 0x20; require.False(t, wrapperUDPRefusalHint(w)) })
	}
}

func TestDLMSACSEMissingOptionalInformationIsUnverified(t *testing.T) {
	for _, requestMissing := range []bool{false, true} {
		t.Run(fmt.Sprint(requestMissing), func(t *testing.T) {
			s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, e)
			defer s.Close("test")
			qhex := acseRequestHex
			rhex := "6117a109060760857405080101a203020100a305a103020100"
			if requestMissing {
				qhex = "600ba109060760857405080101"
				rhex = acseResponseHex
			}
			q := s.Feed(0, time.Unix(1, 0), acseTestWire(t, 1, 16, qhex))
			require.Nil(t, q.Err)
			require.Len(t, q.Events, 1)
			r := s.Feed(1, time.Unix(1, 0), acseTestWire(t, 16, 1, rhex))
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			require.Zero(t, r.Events[0].ResponseTo)
			require.Zero(t, r.Events[0].TransactionID)
			f, e := r.Events[0].GetFields()
			require.NoError(t, e)
			require.Equal(t, "AARE", f["kind"])
			require.Equal(t, false, f["authentication_verified"])
			s.Close("test")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func acseControls(t *testing.T) []wrapperBlockControl {
	t.Helper()
	var raw []byte
	var e error
	if root := os.Getenv("DLMS_ACSE_TEST_ROOT"); root != "" {
		raw, e = os.ReadFile(filepath.Join(root, "controls.json"))
	} else {
		raw, e = trafficfixture.ReadFile("dlms-acse/controls.json")
	}
	require.NoError(t, e)
	var m struct{ Cases []wrapperBlockControl }
	require.NoError(t, json.Unmarshal(raw, &m))
	require.Len(t, m.Cases, 32)
	for _, c := range m.Cases {
		require.Equal(t, c.SHA, fmt.Sprintf("%x", sha256.Sum256(wrapperStructuredInput(t, c.Alias))))
	}
	return m.Cases
}
func TestDLMSACSESealedMatrix(t *testing.T) {
	wrapperBlocksReplayConfigured(t, acseControls(t), func(c wrapperBlockControl) []CaptureOption {
		if strings.HasPrefix(c.Name, "resource-low-buffer-") {
			return []CaptureOption{WithProtocolBudget(1024, 6000)}
		}
		return nil
	})
}
func TestDLMSACSESealedNativeOwnership(t *testing.T) {
	wrapperBlocksOwnershipConfigured(t, acseControls(t), func(c wrapperBlockControl, b *ParserBudget) {
		if strings.HasPrefix(c.Name, "resource-low-buffer-") {
			b.MaxMessageBytes = 1024
			b.MaxBufferedBytes = 6000
		}
	})
}

func TestDLMSACSEMissingInitiateTombstoneSurvivesUDP(t *testing.T) {
	s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
	require.NoError(t, e)
	defer s.Close("test")
	missing := s.Feed(0, time.Unix(1, 0), acseTestWire(t, 1, 16, "600ba109060760857405080101"))
	require.Nil(t, missing.Err)
	require.Len(t, missing.Events, 1)
	require.Zero(t, missing.Events[0].TransactionID)
	next := s.Feed(0, time.Unix(2, 0), acseTestWire(t, 1, 16, acseRequestHex))
	require.Nil(t, next.Err)
	require.Len(t, next.Events, 1)
	require.Zero(t, next.Events[0].TransactionID)
	require.Equal(t, "ambiguous-association-generation", next.Events[0].Session["Association"])
	late := s.Feed(1, time.Unix(3, 0), acseTestWire(t, 16, 1, acseResponseHex))
	require.Nil(t, late.Err)
	require.Len(t, late.Events, 1)
	require.Zero(t, late.Events[0].ResponseTo)
	require.Equal(t, "ambiguous-association-generation", late.Events[0].Session["Association"])
	s.Close("test")
	require.Zero(t, s.Stats().BufferedBytes)
}
