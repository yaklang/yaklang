package pcaputil

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func hdlcSetListRequestSegmentsControls(t *testing.T) []hdlcOpCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-set-list-request-segments", "DLMS_HDLC_SET_LIST_REQUEST_SEGMENTS_TEST_ROOT", "controls.json")
	require.Equal(t, "982e502ddb0e867c8f25282ce5f150615ce6992eed153eb9c92c108b6db92e5d", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var x struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(raw, &x))
	require.Len(t, x.Cases, 19)
	return x.Cases
}
func TestDLMSHDLCSetListRequestSegmentsNative(t *testing.T) {
	hdlcOpNative(t, hdlcSetListRequestSegmentsControls(t), 41000, 4059)
}
func TestDLMSHDLCSetListRequestSegmentsOffPort(t *testing.T) {
	c := hdlcSetListRequestSegmentsControls(t)
	hdlcOpNative(t, c[:3], 45000, 45001)
}
func TestDLMSHDLCSetListRequestSegmentsMatrix(t *testing.T) {
	hdlcListSegmentsMatrixBatch(t, hdlcSetListRequestSegmentsControls(t), "dlms-hdlc-set-list-request-segments", "DLMS_HDLC_SET_LIST_REQUEST_SEGMENTS_TEST_ROOT")
}

func TestDLMSHDLCSetListRequestSegmentsColdProbe(t *testing.T) {
	t.Run("cold-first-list5", func(t *testing.T) {
		s, err := NewProtocolSession(ParserBudget{})
		if err != nil {
			t.Fatal(err)
		}
		w, err := hex.DecodeString("7ea81e0321107ed7e6e600c105c102000100002a0000ff02000001000074627e")
		if err != nil {
			t.Fatal(err)
		}
		p := s.Probe(w)
		if p.Verdict != ProbeAccept || p.Protocol != "dlms" || p.Version != "hdlc-set-list-block-request-segmented" || p.Confidence != 98 {
			t.Fatalf("unexpected admission: %#v", p)
		}
	})
	t.Run("cold-orphan-next3", func(t *testing.T) {
		s, err := NewProtocolSession(ParserBudget{})
		if err != nil {
			t.Fatal(err)
		}
		w, err := hex.DecodeString("7ea81e0321107ed7e6e600c103c101000000021a000102030405060708c8387e")
		if err != nil {
			t.Fatal(err)
		}
		p := s.Probe(w)
		if p.Verdict == ProbeAccept && p.Protocol == "dlms" {
			t.Fatalf("unowned C1next3 admitted as DLMS: %#v", p)
		}
	})
	t.Run("cold-GET-normal-neighbor", func(t *testing.T) {
		s, err := NewProtocolSession(ParserBudget{})
		if err != nil {
			t.Fatal(err)
		}
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
