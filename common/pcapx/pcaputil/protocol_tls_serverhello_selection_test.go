package pcaputil

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func tlsServerHelloSelectionRead(t *testing.T, file string, count int) []tlsAlertCase {
	var x struct {
		Cases []struct {
			Name  string
			Steps []struct {
				Wire         string `json:"input_hex"`
				SHA          string `json:"input_sha256"`
				Direction    int
				At           int64 `json:"timestamp_unix"`
				Offset       uint64
				Error        string
				Completeness string
				Fields       map[string]any `json:"correct_fields"`
			}
			After *struct {
				Wire      string `json:"input_hex"`
				Direction int
				At        int64 `json:"timestamp_unix"`
				Error     string
			} `json:"after_refusal"`
		}
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "tls-serverhello-selection", "TLS_SERVERHELLO_SELECTION_TEST_ROOT", file), &x))
	require.Len(t, x.Cases, count)
	out := make([]tlsAlertCase, 0, len(x.Cases))
	for _, c := range x.Cases {
		v := tlsAlertCase{Name: c.Name, Ports: [2]uint16{40000, 443}}
		var n uint64
		malformed := uint64(0)
		for _, w := range c.Steps {
			v.Steps = append(v.Steps, tlsAlertStep{Wire: w.Wire, SHA: w.SHA, Direction: w.Direction, At: w.At, Offset: w.Offset, Fields: w.Fields, Error: w.Error, Completeness: w.Completeness})
			n += uint64(len(wrapperWire(t, w.Wire)))
			if w.Error != "" {
				malformed++
			}
		}
		v.Statistics = map[string]uint64{"Messages": uint64(len(c.Steps)), "InputBytes": n, "MessageBytes": n, "UnclassifiedBytes": 0, "LimitedBytes": 0, "Incomplete": 0, "Malformed": malformed}
		if c.After != nil {
			v.Probes = []tlsAlertProbe{{Wire: c.After.Wire, Direction: c.After.Direction, At: c.After.At, Error: c.After.Error}}
			n = uint64(len(wrapperWire(t, c.After.Wire)))
			v.Statistics["InputBytes"] += n
			v.Statistics["UnclassifiedBytes"] = n
		}
		out = append(out, v)
	}
	return out
}
func TestTLSServerHelloSelectionNative(t *testing.T) {
	tlsRecordBudgetNative(t, tlsServerHelloSelectionControls(t))
}
func TestTLSServerHelloSelectionMatrix(t *testing.T) {
	tlsRecordBudgetMatrix(t, tlsServerHelloSelectionControls(t), "tls-serverhello-selection", "TLS_SERVERHELLO_SELECTION")
}

func tlsServerHelloSelectionControls(t *testing.T) []tlsAlertCase {
	return tlsServerHelloSelectionRead(t, "controls.json", 3)
}
func TestTLSServerHelloSelectionPartialNative(t *testing.T) {
	tlsRecordBudgetNative(t, tlsServerHelloSelectionRead(t, "partial-controls.json", 1))
}
func TestTLSServerHelloSelectionPartialMatrix(t *testing.T) {
	tlsRecordBudgetMatrix(t, tlsServerHelloSelectionRead(t, "partial-controls.json", 1), "tls-serverhello-selection", "TLS_SERVERHELLO_SELECTION")
}

func TestTLSServerHelloSelectionNegotiationNative(t *testing.T) {
	tlsRecordBudgetNative(t, tlsServerHelloSelectionRead(t, "negotiation-controls.json", 4))
}
func TestTLSServerHelloSelectionNegotiationMatrix(t *testing.T) {
	tlsRecordBudgetMatrix(t, tlsServerHelloSelectionRead(t, "negotiation-controls.json", 4), "tls-serverhello-selection", "TLS_SERVERHELLO_SELECTION")
}
