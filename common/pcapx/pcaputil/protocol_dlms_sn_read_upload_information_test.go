package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func snReadUploadInformationLoad(t *testing.T, name, pin string, count int) []hdlcSNReadCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-sn-read-upload-information-source", "DLMS_SN_READ_UPLOAD_INFORMATION_ROOT", name)
	require.Equal(t, pin, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var g struct{ Cases []hdlcOpCase }
	var m struct {
		Cases []struct {
			Steps []map[string]any
			UDP   string `json:"sha256"`
			TCP   string `json:"tcp_sha256"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &g))
	require.NoError(t, json.Unmarshal(raw, &m))
	require.Len(t, g.Cases, count)
	require.Len(t, m.Cases, count)
	out := make([]hdlcSNReadCase, count)
	for i, c := range g.Cases {
		for j := range c.Close.Events {
			e := &c.Close.Events[j]
			if e.Profile == "dlms-hdlc-sn-information-segmented" {
				require.Equal(t, "test-close: "+e.SummaryUDP, e.SummaryTCP)
				e.SummaryTCP = "<reason>: " + e.SummaryUDP
			}
		}
		out[i] = hdlcSNReadCase{c, m.Cases[i].Steps, m.Cases[i].UDP, m.Cases[i].TCP}
	}
	return out
}
func TestDLMSSNReadUploadInformationNative(t *testing.T) {
	snInformationCountNativeWithIDSeed(t, snReadUploadInformationLoad(t, "full-controls.json", "43a9b856802bcd998bd4ae027bfb5f5d2c50a6ed78967b67f21dee9cdbebde57", 6), 41000, 4059, 10000)
}

func TestDLMSSNReadUploadInformationColdNative(t *testing.T) {
	snInformationCountNativeWithIDSeed(t, snReadUploadInformationLoad(t, "cold-read-controls.json", "4c4541a157ce4b5d43f60bcf3c746cd93794593b3b474634a156dac5af71a4d5", 1), 41000, 4059, 10000)
}

func TestDLMSSNReadUploadInformationFullExistingAPI(t *testing.T) {
	snInformationCountNative(t, snReadUploadInformationLoad(t, "baseline-controls.json", "c96e3c482c6137a67f80ff6f83be0155176ce8c072d76cd94d7df13cf3daef3c", 3), 41000, 4059)
}
func TestDLMSSNReadUploadInformationMatrix(t *testing.T) {
	cases := snReadUploadInformationLoad(t, "captured-controls.json", "8b18b243a624eb78e91ef1711f0749a888336132ec0ca454ad38ff4f739685ee", 6)
	neighbor := snReadUploadInformationLoad(t, "captured-baseline-controls.json", "6709f5fa3bbf743ed5424e039e3444599a509524f4a2494c01044356089f0a14", 3)
	cases = append(cases, neighbor[2])
	cases = append(cases, snReadUploadInformationLoad(t, "cold-read-captured-controls.json", "3da544501eb96064976c3e25f5824bd6a93fbf11397e9a65a78ad3942e95d22a", 1)...)
	hdlcSNReadMatrixChecked(t, cases, "dlms-sn-read-upload-information-source/captures", []string{"tcp", "udp"}, func(t *testing.T, e *ProtocolEvent, st hdlcOpStep, meta map[string]any) {
		require.Equal(t, meta["public_error"], e.Error)
		if st.Error != "" {
			snMultiOwnedDiagnosis(t, e, st.Error, meta["error_message"].(string))
		}
		actionPoison(e)
		hdlcSNReadQueries(t, e, st)
		if st.Error != "" {
			snMultiOwnedDiagnosis(t, e, st.Error, meta["error_message"].(string))
		}
	})
}

// A capability sidecar upgrades exactly one previously unsupported Read trace.
// The original immutable answer and capture pins still guard every input.
// The previously upgraded Write case and ordinary Read neighbour are preserved.
func snReadUploadInformationUpgrade(t *testing.T, raw []byte, cases []hdlcSNReadCase) []hdlcSNReadCase {
	t.Helper()
	require.Len(t, cases, 3)
	side := finsReadBatch(t, "dlms-sn-read-upload-information-source", "DLMS_SN_READ_UPLOAD_INFORMATION_ROOT", "historical-read-information-upgrade.json")
	require.Equal(t, "c1f0bb38cb4e8bb4636dd9b63b10b78b9fe61640d3755b3d80a0eeebbbf741ce", fmt.Sprintf("%x", sha256.Sum256(side)))
	var original struct{ Cases []map[string]any }
	var doc struct {
		Records []struct {
			Index    int             `json:"case_index0"`
			Original map[string]any  `json:"original_case"`
			New      json.RawMessage `json:"new_case"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &original))
	require.NoError(t, json.Unmarshal(side, &doc))
	require.Len(t, original.Cases, 3)
	require.Len(t, doc.Records, 1)
	r := doc.Records[0]
	require.Equal(t, 0, r.Index)
	require.Equal(t, original.Cases[0], r.Original)
	var updated hdlcOpCase
	var meta struct {
		Steps []map[string]any
		UDP   string `json:"sha256"`
		TCP   string `json:"tcp_sha256"`
	}
	require.NoError(t, json.Unmarshal(r.New, &updated))
	require.NoError(t, json.Unmarshal(r.New, &meta))
	old := cases[0]
	require.Equal(t, old.Name, updated.Name)
	require.Len(t, updated.Steps, len(old.Steps))
	require.Len(t, meta.Steps, len(old.Metadata))
	require.Equal(t, old.UDPHash, meta.UDP)
	require.Equal(t, old.TCPHash, meta.TCP)
	for i, st := range updated.Steps {
		require.Equal(t, old.Steps[i].Wire, st.Wire)
		require.Equal(t, old.Steps[i].SHA, st.SHA)
		require.Equal(t, old.Steps[i].Direction, st.Direction)
		require.Equal(t, old.Steps[i].At, st.At)
		require.Equal(t, old.Steps[i].OffsetTCP, st.OffsetTCP)
		require.Equal(t, old.Steps[i].OffsetUDP, st.OffsetUDP)
		if i < 3 {
			require.Equal(t, old.Metadata[i], meta.Steps[i])
		}
	}
	require.Len(t, updated.Close.Events, 1)
	cases[0] = hdlcSNReadCase{updated, meta.Steps, meta.UDP, meta.TCP}
	return cases
}
