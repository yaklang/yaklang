package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func snWriteUploadInformationLoad(t *testing.T, name, pin string, count int) []hdlcSNReadCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-sn-write-upload-information-source", "DLMS_SN_WRITE_UPLOAD_INFORMATION_ROOT", name)
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
func TestDLMSSNWriteUploadInformationNative(t *testing.T) {
	snInformationCountNativeWithIDSeed(t, snWriteUploadInformationLoad(t, "full-controls.json", "897541445d60fcbb1c5c67b06b76b7c050fd10491a5c30edc96a546699fc39c4", 6), 41000, 4059, 10000)
}

func TestDLMSSNWriteUploadInformationFullExistingAPI(t *testing.T) {
	snInformationCountNative(t, snWriteUploadInformationLoad(t, "baseline-controls.json", "83e11ab0e9a63286fec0ad1700415c6662125055fb8635ce259ccbe91874a3d5", 3), 41000, 4059)
}
func TestDLMSSNWriteUploadInformationMatrix(t *testing.T) {
	cases := snWriteUploadInformationLoad(t, "captured-controls.json", "ae1f428ef0a9ecbd3a556f65d3d440666fd7270b988c5fff928607306ab5ee9b", 6)
	neighbor := snWriteUploadInformationLoad(t, "captured-baseline-controls.json", "d7fa206919c34d7625873f4bd3ed12d064b7a17c2ebfeee06646bd9c9d6a1a2f", 3)
	cases = append(cases, neighbor[2])
	hdlcSNReadMatrixChecked(t, cases, "dlms-sn-write-upload-information-source/captures", []string{"tcp", "udp"}, func(t *testing.T, e *ProtocolEvent, st hdlcOpStep, meta map[string]any) {
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

// A capability sidecar upgrades exactly one previously unsupported Write trace.
// The original immutable answer and capture pins still guard every input and
// the other two boundary cases keep their complete original expectations.
func snWriteUploadInformationUpgrade(t *testing.T, raw []byte, cases []hdlcSNReadCase) []hdlcSNReadCase {
	t.Helper()
	require.Len(t, cases, 3)
	side := finsReadBatch(t, "dlms-sn-write-upload-information-source", "DLMS_SN_WRITE_UPLOAD_INFORMATION_ROOT", "historical-write-information-upgrade.json")
	require.Equal(t, "8d54efb83a74c1a29c9e57793da9c5e7e71f6fd8f180f755f3fd6682eeada6a6", fmt.Sprintf("%x", sha256.Sum256(side)))
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
	require.Equal(t, 1, r.Index)
	require.Equal(t, original.Cases[1], r.Original)
	var updated hdlcOpCase
	var meta struct {
		Steps []map[string]any
		UDP   string `json:"sha256"`
		TCP   string `json:"tcp_sha256"`
	}
	require.NoError(t, json.Unmarshal(r.New, &updated))
	require.NoError(t, json.Unmarshal(r.New, &meta))
	old := cases[1]
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
	cases[1] = hdlcSNReadCase{updated, meta.Steps, meta.UDP, meta.TCP}
	return cases
}
