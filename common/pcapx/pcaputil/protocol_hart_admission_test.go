package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"net"
	"testing"
	"time"
)

type hartAdmissionControl struct {
	Schema string `json:"schema"`
	TLS    struct {
		SourceSHA string `json:"source_sha256"`
		Selected  struct {
			SHA   string `json:"sha256"`
			Bytes int    `json:"record_bytes"`
		} `json:"selected_clienthello"`
	} `json:"tls_reused"`
	DTLS       string         `json:"dtls_record_hex"`
	DTLSSHA    string         `json:"dtls_record_sha256"`
	DTLSFields map[string]any `json:"dtls_fields"`
}

func hartAdmissionWire(t *testing.T, carrier string) ([]byte, string, hartAdmissionControl) {
	t.Helper()
	var c hartAdmissionControl
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "hart-admission", "HART_ADMISSION_TEST_ROOT", "controls.json"), &c))
	require.Equal(t, "hart-admission-invariants/v1", c.Schema)
	if carrier == "udp" {
		w := wrapperWire(t, c.DTLS)
		require.Equal(t, c.DTLSSHA, fmt.Sprintf("%x", sha256.Sum256(w)))
		return w, "dtls", c
	}
	original := binCorpusBytes(t, "ndpi/ndpi-http-connect.pcap")
	require.Equal(t, c.TLS.SourceSHA, fmt.Sprintf("%x", sha256.Sum256(original)))
	w := t21CapturedTLSClientHello(t)
	require.Len(t, w, c.TLS.Selected.Bytes)
	require.Equal(t, c.TLS.Selected.SHA, fmt.Sprintf("%x", sha256.Sum256(w)))
	return w, "tls", c
}
func hartAdmissionCanonical(t *testing.T, carrier string, w []byte) (map[string]any, map[string]any, ProbeResult) {
	t.Helper()
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(carrier), WithSessionPorts(40000, 443))
	require.NoError(t, err)
	probe := s.Probe(w)
	out := s.Feed(0, time.Unix(1, 0), w)
	require.Nil(t, out.Err)
	require.Len(t, out.Events, 1)
	fields, err := out.Events[0].GetFields()
	require.NoError(t, err)
	session := cloneSession(out.Events[0].Session)
	require.Empty(t, s.Close("end"))
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
	return fields, session, probe
}
func hartAdmissionAssert(t *testing.T, e *ProtocolEvent, w []byte, protocol string, fields, session map[string]any) {
	t.Helper()
	require.Equal(t, protocol, e.Protocol)
	require.Equal(t, w, e.Raw)
	require.Len(t, e.Raw, e.Length)
	require.Empty(t, e.Error)
	require.Zero(t, e.ResponseTo)
	require.Zero(t, e.TransactionID)
	got, err := e.GetFields()
	require.NoError(t, err)
	rocEqualFields(t, fields, got)
	rocEqualFields(t, session, e.Session)
	decoded, err := e.Decode()
	require.NoError(t, err)
	rocEqualFields(t, fields, decoded["fields"].(map[string]any))
	rocEqualFields(t, session, decoded["session"].(map[string]any))
	require.Equal(t, false, e.Session["Authentication Verified"])
}
func TestHARTAdmissionPreservesTLSRecords(t *testing.T) {
	for _, carrier := range []string{"tcp", "udp"} {
		w, protocol, c := hartAdmissionWire(t, carrier)
		fields, session, probe := hartAdmissionCanonical(t, carrier, w)
		if carrier == "udp" {
			rocEqualFields(t, c.DTLSFields, fields)
			rocEqualFields(t, c.DTLSFields, session)
		}
		for _, deferred := range []bool{false, true} {
			for _, chunk := range []int{1, 7, 64, 4096} {
				if carrier == "udp" && chunk != 4096 {
					continue
				}
				t.Run(fmt.Sprintf("%s/df%t/chunk%d", carrier, deferred, chunk), func(t *testing.T) {
					s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(carrier), WithSessionPorts(40000, 5094))
					require.NoError(t, err)
					s.(*captureSession).f.a.config.Deferred = deferred
					require.Equal(t, probe, s.Probe(w))
					var events []*ProtocolEvent
					for at := 0; at < len(w); at += chunk {
						in := bytes.Clone(w[at:min(len(w), at+chunk)])
						out := s.Feed(0, time.Unix(1, 0), in)
						clear(in)
						if out.Err != nil {
							require.Equal(t, ErrNeedMore, out.Err.Kind)
						}
						events = append(events, out.Events...)
					}
					require.Len(t, events, 1)
					hartAdmissionAssert(t, events[0], w, protocol, fields, session)
					require.Empty(t, s.Close("end"))
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
}
func TestHARTAdmissionCaptureMatrix(t *testing.T) {
	for _, carrier := range []string{"tcp", "udp"} {
		t.Run(carrier, func(t *testing.T) {
			w, protocol, c := hartAdmissionWire(t, carrier)
			fields, session, _ := hartAdmissionCanonical(t, carrier, w)
			if carrier == "udp" {
				rocEqualFields(t, c.DTLSFields, fields)
			}
			var raw []byte
			if carrier == "tcp" {
				raw = sessionTestPCAP(t, []sessionStep{{dir: 0, wire: w}}, 5094, 7, false, true)
			} else {
				raw = t20UDPCapture(t, net.ParseIP("192.0.2.1"), net.ParseIP("192.0.2.2"), 40000, 5094, w)
			}
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				events, stats := discoveryReplay(t, raw, workers, deferred, observe)
				require.Len(t, events, 1)
				hartAdmissionAssert(t, events[0], w, protocol, fields, session)
				require.NotEmpty(t, events[0].SourceBytes.PacketRefs)
				require.Zero(t, stats.BufferedBytes)
			})
		})
	}
}
func TestHARTExplicitAdmissionRemainsStrict(t *testing.T) {
	w, _, _ := hartAdmissionWire(t, "udp")
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionHARTIP(5094), WithSessionTransport("udp"), WithSessionPorts(40000, 5094))
	require.NoError(t, err)
	out := s.Feed(0, time.Unix(1, 0), w)
	require.Len(t, out.Events, 1)
	e := out.Events[0]
	require.Equal(t, "hart-ip", e.Protocol)
	_, err = e.GetFields()
	rocTypedError(t, "UnsupportedVersion", err)
	require.Zero(t, e.ResponseTo)
	require.Zero(t, e.TransactionID)
	require.Nil(t, e.Session)
	assertOwnedProtocolRefusal(t, e, "UnsupportedVersion", err)
	require.Empty(t, s.Close("end"))
	require.Zero(t, s.Stats().BufferedBytes)
}
