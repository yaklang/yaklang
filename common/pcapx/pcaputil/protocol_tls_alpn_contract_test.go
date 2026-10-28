package pcaputil

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func tlsALPNContractRead(t *testing.T, file string, count int) []tlsAlertCase {
	var x struct {
		Cases []struct {
			Name   string
			Budget struct {
				Collection int `json:"MaxCollectionElements"`
			}
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
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "tls-alpn-contract", "TLS_ALPN_CONTRACT_TEST_ROOT", file), &x))
	require.Len(t, x.Cases, count)
	out := make([]tlsAlertCase, 0, len(x.Cases))
	for _, c := range x.Cases {
		v := tlsAlertCase{Name: c.Name, Ports: [2]uint16{40000, 443}}
		v.Budget.Collection = c.Budget.Collection
		var n, limited, malformed uint64
		for _, w := range c.Steps {
			v.Steps = append(v.Steps, tlsAlertStep{Wire: w.Wire, SHA: w.SHA, Direction: w.Direction, At: w.At, Offset: w.Offset, Fields: w.Fields, Error: w.Error, Completeness: w.Completeness})
			size := uint64(len(wrapperWire(t, w.Wire)))
			n += size
			if w.Error == "ResourceExceeded" {
				limited += size
			}
			if w.Error == "MalformedMessage" {
				malformed++
			}
		}
		v.Statistics = map[string]uint64{"Messages": uint64(len(c.Steps)), "InputBytes": n, "MessageBytes": n, "UnclassifiedBytes": 0, "LimitedBytes": limited, "Incomplete": 0, "Malformed": malformed}
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
func TestTLSALPNContractBudgetNative(t *testing.T) {
	tlsRecordBudgetNative(t, tlsALPNContractRead(t, "budget-controls.json", 2))
}
func TestTLSALPNContractBudgetMatrix(t *testing.T) {
	tlsRecordBudgetMatrix(t, tlsALPNContractRead(t, "budget-controls.json", 2), "tls-alpn-contract", "TLS_ALPN_CONTRACT")
}
func TestTLSALPNContractSelectionNative(t *testing.T) {
	tlsRecordBudgetNative(t, tlsALPNContractRead(t, "selection-controls.json", 3))
}
func TestTLSALPNContractSelectionMatrix(t *testing.T) {
	tlsRecordBudgetMatrix(t, tlsALPNContractRead(t, "selection-controls.json", 3), "tls-alpn-contract", "TLS_ALPN_CONTRACT")
}
func TestTLSALPNContractFramingNative(t *testing.T) {
	tlsRecordBudgetNative(t, tlsALPNContractRead(t, "framing-controls.json", 4))
}
func TestTLSALPNContractFramingMatrix(t *testing.T) {
	tlsRecordBudgetMatrix(t, tlsALPNContractRead(t, "framing-controls.json", 4), "tls-alpn-contract", "TLS_ALPN_CONTRACT")
}

func tlsALPNDTLSControls(t *testing.T) []tlsHelloDTLSCase {
	var x struct{ Cases []tlsHelloDTLSCase }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "tls-alpn-contract", "TLS_ALPN_CONTRACT_TEST_ROOT", "dtls/controls.json"), &x))
	require.Len(t, x.Cases, 2)
	return x.Cases
}
func TestTLSALPNContractDTLSNative(t *testing.T) { tlsHelloDTLSNative(t, tlsALPNDTLSControls(t)) }
func TestTLSALPNContractDTLSMatrix(t *testing.T) {
	tlsHelloDTLSMatrix(t, tlsALPNDTLSControls(t), "tls-alpn-contract", "TLS_ALPN_CONTRACT")
}
