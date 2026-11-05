package engine

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/suricata/state"
)

func TestIncompleteProtocolEventProducesDiagnostic(t *testing.T) {
	engine, err := NewStatefulEngine(nil, Config{State: state.DefaultConfig()})
	require.NoError(t, err)
	defer engine.Close()
	result := engine.Handle(InspectionEvent{View: ViewTransaction, Protocol: &pcaputil.ProtocolEvent{Protocol: "http", Status: "incomplete", Completeness: "incomplete", Error: "capture ended"}})
	require.Len(t, result.Diagnostics, 1)
	require.Equal(t, "protocol-event", result.Diagnostics[0].Kind)
	require.Equal(t, "capture ended", result.Diagnostics[0].Message)
}
