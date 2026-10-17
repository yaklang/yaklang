package pcaputil

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

// Source-derived discovery boundary only; no device or complete method claim.
func TestDLMSHDLCActionUploadRequestSegmentsColdProbe(t *testing.T) {
	t.Run("cold-first4-partial-only", func(t *testing.T) {
		s, err := NewProtocolSession(ParserBudget{})
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close("coldprobe")
		w, err := hex.DecodeString("7ea81e0321326ed5e6e600c304c1000f0000280000ff0100000000010e3e037e")
		if err != nil {
			t.Fatal(err)
		}
		p := s.Probe(w)
		if p.Verdict != ProbeAccept || p.Protocol != "dlms" || p.Version != "hdlc-action-upload-request-segmented" || p.Confidence != 98 {
			t.Fatalf("unexpected admission: %#v", p)
		}
	})
	t.Run("cold-orphan-next6-not-GET", func(t *testing.T) {
		s, err := NewProtocolSession(ParserBudget{})
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close("coldprobe")
		w, err := hex.DecodeString("7ea81e0321545ed3e6e600c306c101000000020c0c0d0e0f101112131472b67e")
		if err != nil {
			t.Fatal(err)
		}
		p := s.Probe(w)
		if p.Verdict == ProbeAccept && p.Protocol == "dlms" {
			t.Fatalf("orphan next6 admitted without upload owner: %#v", p)
		}
	})
	t.Run("cold-GET-normal-neighbor", func(t *testing.T) {
		s, err := NewProtocolSession(ParserBudget{})
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close("coldprobe")
		w, err := hex.DecodeString("7ea0190321107fdae6e600c001cf000100002a0000ff0200cfc97e")
		if err != nil {
			t.Fatal(err)
		}
		p := s.Probe(w)
		if p.Verdict != ProbeAccept || p.Protocol != "dlms" || p.Version != "hdlc-get-normal" || p.Confidence != 98 {
			t.Fatalf("unexpected admission: %#v", p)
		}
	})
}

func hdlcActionRequestColdCase(t *testing.T) hdlcOpCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-action-upload-request-segments", "DLMS_HDLC_ACTION_UPLOAD_REQUEST_SEGMENTS_TEST_ROOT", "cold-admission.json")
	require.Equal(t, "992ca4b3275614f5efcdb099f79d15e030150655efcb0fc5236768a92e498123", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct {
		Inputs []struct {
			Name string
			Case hdlcOpCase
		}
	}
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Inputs, 3)
	require.Equal(t, "cold-first4-partial-only", d.Inputs[0].Name)
	c := d.Inputs[0].Case
	require.Len(t, c.Steps, 1)
	require.Len(t, c.Close.Events, 1)
	return c
}
func TestDLMSHDLCActionUploadRequestSegmentsColdNative(t *testing.T) {
	hdlcOpNative(t, []hdlcOpCase{hdlcActionRequestColdCase(t)}, 41000, 4059)
}
func TestDLMSHDLCActionUploadRequestSegmentsColdMatrix(t *testing.T) {
	hdlcListSegmentsMatrixBatch(t, []hdlcOpCase{hdlcActionRequestColdCase(t)}, "dlms-hdlc-action-upload-request-segments", "DLMS_HDLC_ACTION_UPLOAD_REQUEST_SEGMENTS_TEST_ROOT")
}
