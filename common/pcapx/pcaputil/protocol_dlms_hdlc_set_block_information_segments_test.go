package pcaputil

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func hdlcSetBlockInformationSegmentsControls(t *testing.T) []hdlcOpCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-set-block-information-segments", "DLMS_HDLC_SET_BLOCK_INFORMATION_SEGMENTS_TEST_ROOT", "controls.json")
	require.Equal(t, "3cb839f9c0cff2ad24c99ac9da349c422ab59be865a2e956d339f860f45f276a", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var x struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(raw, &x))
	require.Len(t, x.Cases, 29)
	return x.Cases
}
func TestDLMSHDLCSetBlockInformationSegmentsNative(t *testing.T) {
	hdlcOpNative(t, hdlcSetBlockInformationSegmentsControls(t), 41000, 4059)
}
func TestDLMSHDLCSetBlockInformationSegmentsOffPort(t *testing.T) {
	c := hdlcSetBlockInformationSegmentsControls(t)
	hdlcOpNative(t, c[:4], 45000, 45001)
}
func TestDLMSHDLCSetBlockInformationSegmentsMatrix(t *testing.T) {
	hdlcListSegmentsMatrixBatch(t, hdlcSetBlockInformationSegmentsControls(t), "dlms-hdlc-set-block-information-segments", "DLMS_HDLC_SET_BLOCK_INFORMATION_SEGMENTS_TEST_ROOT")
}

func TestDLMSHDLCSetBlockInformationSegmentsColdProbe(t *testing.T) {
	t.Run("cold-singleSET-first2", func(t *testing.T) {
		s, err := NewProtocolSession(ParserBudget{})
		if err != nil {
			t.Fatal(err)
		}
		w, err := hex.DecodeString("7ea81e0321107ed7e6e600c102c1000100002a0000ff02000000000001ae997e")
		if err != nil {
			t.Fatal(err)
		}
		p := s.Probe(w)
		if p.Verdict != ProbeAccept || p.Protocol != "dlms" || p.Version != "hdlc-set-block-request-segmented" || p.Confidence != 98 {
			t.Fatalf("unexpected admission: %#v", p)
		}
	})
	t.Run("cold-orphan-knownnext3", func(t *testing.T) {
		s, err := NewProtocolSession(ParserBudget{})
		if err != nil {
			t.Fatal(err)
		}
		w, err := hex.DecodeString("7ea81e0321107ed7e6e600c103c101000000020d0b0c0d0e0f10111213a5b77e")
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
