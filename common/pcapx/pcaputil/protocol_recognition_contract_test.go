package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func TestCurrentRecognitionEvidenceContract(t *testing.T) {
	root := "../../bin-parser/testdata"
	data, err := trafficfixture.ReadFile(filepath.Join(root, "quality-gates/recognition-evidence.json"))
	require.NoError(t, err)
	var doc struct {
		Schema   int `json:"schema_version"`
		Scope    string
		Profiles []struct {
			Protocol, Transport, Scope, Fixture, SHA256 string
			Boundaries                                  []string
			Messages                                    int
		}
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	require.Equal(t, 1, doc.Schema)
	require.NotEmpty(t, doc.Scope)
	require.Len(t, doc.Profiles, 7)
	profiles := map[string]ProtocolProfile{}
	for _, p := range NativeProtocolProfiles() {
		require.NotContains(t, profiles, p.Protocol)
		profiles[p.Protocol] = p
	}
	seen := map[string]bool{}
	for _, p := range doc.Profiles {
		t.Run(p.Protocol, func(t *testing.T) {
			require.False(t, seen[p.Protocol])
			seen[p.Protocol] = true
			require.Contains(t, profiles, p.Protocol)
			require.NotEmpty(t, p.Scope)
			require.NotEmpty(t, p.Boundaries)
			require.NotEmpty(t, p.Transport)
			wire, err := trafficfixture.ReadFile(filepath.Join(root, p.Fixture))
			require.NoError(t, err)
			require.Equal(t, p.SHA256, fmt.Sprintf("%x", sha256.Sum256(wire)))
			for _, deferred := range []bool{false, true} {
				options := []CaptureOption{WithProtocolDeferred(deferred)}
				if p.Protocol == "dns" {
					options = append(options, WithProtocolDecodeAs("udp", 19555, "dns"))
				}
				events, stats, err := binReplay(t, wire, 2, options...)
				require.NoError(t, err)
				require.Zero(t, stats.BufferedBytes)
				require.Zero(t, stats.Malformed)
				require.Zero(t, stats.LimitedBytes)
				require.NotEmpty(t, events)
				require.Positive(t, p.Messages)
				require.Len(t, events, p.Messages)
				for _, e := range events {
					require.Equal(t, p.Protocol, e.Protocol)
					require.Empty(t, e.Error)
					require.Contains(t, []string{"decoded", "deferred"}, e.Status)
					_, err = e.Decode()
					require.NoError(t, err)
				}
				t.Logf("%s deferred=%v messages=%d", p.Protocol, deferred, len(events))
			}
		})
	}
}

func TestRecognitionDTLSTransportAndPortIsolation(t *testing.T) {
	hello := dtlsTestHello()
	valid := dtlsTestRecord(22, 0, 0, dtlsTestFragment(1, 0, len(hello), 0, hello))
	for _, port := range []uint16{53, 161, 443, 4444, 5353, 1883} {
		c, a := dtlsTestParser(t, 0)
		e := &ProtocolEvent{Source: fmt.Sprintf("192.0.2.1:%d", port), Destination: "192.0.2.2:40000", Transport: "udp", Timestamp: time.Unix(1, 0)}
		require.True(t, a.decodeNativeDatagram(e, valid, port, 40000))
		require.Equal(t, "dtls", e.Protocol)
		require.Empty(t, e.Error)
		for _, wire := range [][]byte{[]byte("GET / HTTP/1.1\r\n\r\n"), []byte("CONNECT\naccept-version:1.2\n\n\x00"), []byte("INFO {\"proto\":1}\r\n"), []byte{22, 3, 3, 0, 0}, []byte{22, 0xfe, 0xfa, 0, 0}} {
			other := &ProtocolEvent{Source: e.Source, Destination: e.Destination, Transport: "udp", Timestamp: time.Unix(2, 0)}
			a.decodeNativeDatagram(other, wire, port, 40000)
			require.NotEqual(t, "dtls", other.Protocol, "UDP port %d bytes %x", port, wire)
		}
		require.NoError(t, c.finishBinParser())
		require.Zero(t, a.stats().BufferedBytes)
	}
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.NotEqual(t, "dtls", s.Probe(valid).Protocol)
	for _, e := range s.Feed(0, time.Unix(1, 0), valid).Events {
		require.NotEqual(t, "dtls", e.Protocol)
	}
	s.Close("test")
}
