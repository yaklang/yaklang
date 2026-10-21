package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func hdlcSNMultiResultLoad(t *testing.T, name, pin string, count int) []hdlcSNReadCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-sn-multiresult-source", "DLMS_HDLC_SN_MULTIRESULT_ROOT", name)
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
		// The independent gold stores an actual test-close string; the old
		// checker accepts a reason placeholder and expands it before comparing.
		for j := range c.Close.Events {
			e := &c.Close.Events[j]
			if e.Profile == "dlms-hdlc-sn-information-segmented" {
				require.Equal(t, "test-close: "+e.SummaryUDP, e.SummaryTCP)
				e.SummaryTCP = "<reason>: " + e.SummaryUDP
			}
		}
		for j := range m.Cases[i].Steps {
			if message, ok := m.Cases[i].Steps[j]["error_message"]; ok {
				m.Cases[i].Steps[j]["query_error_message"] = message
			}
		}
		out[i] = hdlcSNReadCase{c, m.Cases[i].Steps, m.Cases[i].UDP, m.Cases[i].TCP}
	}
	return out
}
func TestDLMSHDLCSNMultiResultExistingAPI(t *testing.T) {
	snInformationCountNative(t, hdlcSNMultiResultLoad(t, "baseline-controls.json", "d2cda3869333a73e38c09e4c6b28a67dedb8a11bfcf280d285c3cbea53ec2705", 3), 41000, 4059)
}

func TestDLMSHDLCSNMultiResultNative(t *testing.T) {
	snInformationCountNative(t, hdlcSNMultiResultLoad(t, "controls.json", "9d7c29c96a6eb9afc97b2235457cb77e05b56e45aaa677ea146eb12fe9a83d7b", 8), 41000, 4059)
}
func TestDLMSHDLCSNMultiResultMatrix(t *testing.T) {
	cases := hdlcSNMultiResultLoad(t, "controls.json", "9d7c29c96a6eb9afc97b2235457cb77e05b56e45aaa677ea146eb12fe9a83d7b", 8)
	hdlcSNReadMatrixChecked(t, cases, "dlms-hdlc-sn-multiresult-captures", []string{"tcp", "udp"}, func(t *testing.T, e *ProtocolEvent, st hdlcOpStep, meta map[string]any) {
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

func hdlcSNMultiResultCompletedControls(t *testing.T) []hdlcSNReadCase {
	cs := hdlcSNMultiResultLoad(t, "completed-state-controls.json", "a7e83a0f19494245fe75045952f82df6ff270baae8397a09871f65a6f158e3ae", 1)
	cs = append(cs, hdlcSNMultiResultLoad(t, "completed-gbt-state-controls.json", "cc16624a990b1543d3a44fa54e0a846e353c9f8956e3b34e88c8225c66e2491b", 1)...)
	return hdlcSNMultiResultCaptureHashes(t, cs, "completed-capture-index.json", "2582f072b0c105210c80ea02c341ce73751e1e78d53dc1890bab351497dd5f00")
}
func hdlcSNMultiResultCaptureHashes(t *testing.T, cs []hdlcSNReadCase, file, pin string) []hdlcSNReadCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-sn-multiresult-source", "DLMS_HDLC_SN_MULTIRESULT_ROOT", file)
	require.Equal(t, pin, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var index map[string]map[string]struct {
		SHA   string `json:"sha256"`
		Bytes int    `json:"bytes"`
	}
	require.NoError(t, json.Unmarshal(raw, &index))
	require.Len(t, index, 2)
	for i := range cs {
		row, ok := index[cs[i].Name]
		require.True(t, ok)
		require.Len(t, row, 2)
		require.Empty(t, cs[i].TCPHash)
		require.Empty(t, cs[i].UDPHash)
		require.Len(t, row["tcp"].SHA, 64)
		require.Len(t, row["udp"].SHA, 64)
		cs[i].TCPHash, cs[i].UDPHash = row["tcp"].SHA, row["udp"].SHA
	}
	return cs
}
func TestDLMSHDLCSNMultiResultCompletedBoundary(t *testing.T) {
	snInformationCountNative(t, hdlcSNMultiResultCompletedControls(t), 41000, 4059)
}

func TestDLMSHDLCSNMultiResultCompletedMatrix(t *testing.T) {
	hdlcSNReadMatrixChecked(t, hdlcSNMultiResultCompletedControls(t), "dlms-hdlc-sn-multiresult-captures", []string{"tcp", "udp"}, func(t *testing.T, e *ProtocolEvent, st hdlcOpStep, meta map[string]any) {
		require.Equal(t, meta["public_error"], e.Error)
		actionPoison(e)
		hdlcSNReadQueries(t, e, st)
	})
}

func hdlcSNMultiResultContextControls(t *testing.T) []hdlcSNReadCase {
	cs := hdlcSNMultiResultLoad(t, "context-controls.json", "5592db48911da71fb4f5bf35eda9cee0982e8501d8966907be3a42cd67ad8a70", 2)
	return hdlcSNMultiResultCaptureHashes(t, cs, "context-capture-index.json", "a752ed21302a6c1e452fbb9784faac26936fdcfa27cb39ed513d11575cb32e8f")
}
func TestDLMSHDLCSNMultiResultContextNative(t *testing.T) {
	snInformationCountNative(t, hdlcSNMultiResultContextControls(t), 41000, 4059)
}
func TestDLMSHDLCSNMultiResultContextMatrix(t *testing.T) {
	hdlcSNReadMatrixChecked(t, hdlcSNMultiResultContextControls(t), "dlms-hdlc-sn-multiresult-captures", []string{"tcp", "udp"}, func(t *testing.T, e *ProtocolEvent, st hdlcOpStep, meta map[string]any) {
		require.Equal(t, meta["public_error"], e.Error)
		actionPoison(e)
		hdlcSNReadQueries(t, e, st)
	})
}
