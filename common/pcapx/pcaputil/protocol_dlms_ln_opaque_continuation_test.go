package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func lnOpaqueLoad(t *testing.T, name, pin string, count int) []hdlcSNReadCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-ln-opaque-continuation-source", "DLMS_LN_OPAQUE_CONTINUATION_ROOT", name)
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
func TestDLMSLNOpaqueContinuationExistingAPI(t *testing.T) {
	snInformationCountNative(t, lnOpaqueLoad(t, "controls.json", "a246b61f2ac45940168769bd4ad44a506d22383c5fd87231c4f94f41149864bf", 2), 41000, 4059)
}

func TestDLMSLNOpaqueContinuationMatrix(t *testing.T) {
	hdlcSNReadMatrixChecked(t, lnOpaqueLoad(t, "controls.json", "a246b61f2ac45940168769bd4ad44a506d22383c5fd87231c4f94f41149864bf", 2), "dlms-ln-opaque-continuation-captures", []string{"tcp", "udp"}, func(t *testing.T, e *ProtocolEvent, st hdlcOpStep, meta map[string]any) {
		require.Equal(t, meta["public_error"], e.Error)
		actionPoison(e)
		hdlcSNReadQueries(t, e, st)
	})
}
