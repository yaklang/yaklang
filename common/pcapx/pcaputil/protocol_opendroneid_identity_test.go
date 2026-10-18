package pcaputil

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type odidIdentityControl struct {
	bsapControl
	Port            uint16
	CaptureAlias    string `json:"capture_alias"`
	ReferenceCaseID string `json:"reference_case_id"`
}

func odidIdentityControls(t *testing.T) []odidIdentityControl {
	t.Helper()
	b, err := trafficfixture.ReadFile("opendroneid-identity/controls.json")
	require.NoError(t, err)
	var m struct {
		Schema string
		Cases  []odidIdentityControl
	}
	require.NoError(t, json.Unmarshal(b, &m))
	require.Equal(t, "opendroneid-identity-selected-v2/v1", m.Schema)
	require.Len(t, m.Cases, 45)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	bound := map[string]trafficfixture.FrozenCase{}
	for _, batch := range all {
		for _, c := range batch.Cases {
			bound[c.ID] = c
		}
	}
	for _, c := range m.Cases {
		id := "opendroneid-identity/" + c.ID
		if c.ReferenceCaseID != "" {
			id = c.ReferenceCaseID
		}
		v, ok := bound[id]
		require.True(t, ok)
		if c.ReferenceCaseID == "" {
			require.Equal(t, c.Capture, v.Input.File)
		} else {
			require.Equal(t, c.CaptureAlias, v.Input.OriginalPath)
		}
		require.Equal(t, c.SHA256, v.Input.SHA256)
		require.Equal(t, c.Packets, v.Facts.PacketCount)
		if c.ReferenceCaseID == "" {
			require.Len(t, v.Expectations, 1)
			var a struct {
				File string `json:"answer_file"`
				SHA  string `json:"answer_sha256"`
			}
			require.NoError(t, json.Unmarshal(v.Expectations[0].PayloadConstraints, &a))
			require.Equal(t, c.Answer, a.File)
			require.Equal(t, c.AnswerSHA, a.SHA)
		}
	}
	return m.Cases
}

// Called by the existing mandatory OpenDroneID replay root. The extension does
// not replace legacy inputs or turn their unsupported System answers positive.
func testOpenDroneIDIdentityReplay(t *testing.T) {
	for _, c := range odidIdentityControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, want := odidAliasedInput(t, "opendroneid-identity", c.bsapControl, c.CaptureAlias)
			odidReplayCase(t, c.bsapControl, raw, want, c.Port, "opendroneid-identity", openDroneIDIdentityProfile)
		})
	}
}

func testOpenDroneIDIdentityBounds(t *testing.T) {
	var wire, pack []byte
	for _, c := range odidIdentityControls(t) {
		raw, a := odidAliasedInput(t, "opendroneid-identity", c.bsapControl, c.CaptureAlias)
		if c.ID == "self-description" {
			wire, _ = hex.DecodeString(a.Events[0].Raw)
		}
		if c.ID == "pack-six-ordered" {
			pack, _ = hex.DecodeString(a.Events[0].Raw)
		}
		if a.Events[0].Fields == nil {
			continue
		}
		// Neither carrier ports nor these wire bytes grant automatic admission.
		require.NoError(t, ReplayPcap(bytes.NewReader(raw), WithOnProtocolMessage(func(e *ProtocolEvent) { require.NotEqual(t, "opendroneid", e.Protocol) })))
	}
	require.Len(t, wire, 25)
	require.Len(t, pack, 153)
	for _, tc := range []struct {
		raw             []byte
		bytes, elements int
	}{{wire, 24, 9}, {wire, 25, 0}, {pack, 153, 5}} {
		f, err := decodeOpenDroneIDProfile(tc.raw, tc.bytes, tc.elements, odidIdentity)
		rocTypedError(t, "ResourceExceeded", err)
		require.Nil(t, f)
	}
	f, err := decodeOpenDroneIDProfile(pack, len(pack), 6, odidIdentity)
	require.NoError(t, err)
	require.Equal(t, 6, f["Message Count"])
	owned := bytes.Clone(wire)
	f, err = decodeOpenDroneIDProfile(owned, 25, 1, odidIdentity)
	require.NoError(t, err)
	clear(owned)
	require.Equal(t, wire, f["Messages"].([]map[string]any)[0]["Raw"])
	bad := bytes.Clone(pack)
	bad[78] = 0x22
	f, err = decodeOpenDroneIDProfile(bad, len(bad), 9, odidIdentity)
	rocTypedError(t, "UnsupportedFeature", err)
	require.Nil(t, f)
	// Reserve failure is atomic. Releasing the competing holder permits a
	// later reverse-direction observation, without acquiring an aircraft role.
	s, err := NewProtocolSessionWithOptions(ParserBudget{MaxFrameBytes: 128, MaxMessageBytes: 128, MaxBufferedBytes: 700, ProbeBytes: 16}, WithSessionTransport("udp"), WithSessionPorts(39200, 28400))
	require.NoError(t, err)
	cs := s.(*captureSession)
	cs.f.a.datagramDecodeAs[28400] = "opendroneid-identity"
	hold := &binFlow{a: cs.f.a}
	require.NoError(t, hold.reserveSession(64))
	g := s.Feed(0, time.Unix(1, 0), wire)
	rocTypedError(t, "ResourceExceeded", g.Err)
	require.Len(t, g.Events, 1)
	require.Nil(t, g.Events[0].Session)
	require.Zero(t, g.Events[0].ResponseTo)
	require.Zero(t, g.Events[0].TransactionID)
	require.EqualValues(t, 64, s.Stats().BufferedBytes)
	hold.closeSession()
	require.Zero(t, s.Stats().BufferedBytes)
	g = s.Feed(1, time.Unix(2, 0), wire)
	require.Nil(t, g.Err)
	require.Len(t, g.Events, 1)
	require.Equal(t, openDroneIDIdentityProfile, g.Events[0].Profile)
	require.Zero(t, g.Events[0].FlowID)
	require.Zero(t, g.Events[0].ResponseTo)
	require.Zero(t, g.Events[0].TransactionID)
	require.Zero(t, s.Stats().BufferedBytes)
	require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(700))
	s.Close("complete")
	s.Close("idempotent")
	require.Zero(t, s.Stats().BufferedBytes)
}
