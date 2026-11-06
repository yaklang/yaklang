package engine

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/suricata/state"
)

func TestEngineCaptureBindingAndStats(t *testing.T) {
	engine, err := NewStatefulEngine(nil, Config{State: state.DefaultConfig()})
	require.NoError(t, err)
	defer engine.Close()

	option := engine.CaptureOption()
	require.NotNil(t, option)
	require.NoError(t, option(pcaputil.NewDefaultConfig()))
	require.Equal(t, AdapterStats{}, engine.CaptureStats())
	require.Equal(t, AdapterStats{}, engine.Stats().Capture)
}

func TestEngineCaptureAfterCloseFails(t *testing.T) {
	engine, err := NewStatefulEngine(nil, Config{State: state.DefaultConfig()})
	require.NoError(t, err)
	require.NoError(t, engine.Close())
	require.Error(t, engine.OpenPcapFile("missing.pcap"))
	require.Error(t, engine.StartSniff("missing0"))
	require.Error(t, engine.CaptureOption()(pcaputil.NewDefaultConfig()))
}
