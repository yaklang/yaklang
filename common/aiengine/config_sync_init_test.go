package aiengine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

func TestAllowSyncInitContextEngineOption(t *testing.T) {
	option, ok := Exports["allowSyncInitContext"].(func(bool) AIEngineConfigOption)
	require.True(t, ok)
	for _, tc := range []struct {
		name string
		opts []AIEngineConfigOption
		want bool
	}{
		{name: "default"},
		{name: "opt in", opts: []AIEngineConfigOption{option(true)}, want: true},
		{name: "opt out", opts: []AIEngineConfigOption{option(true), option(false)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := NewAIEngineConfig(tc.opts...)
			require.Equal(t, tc.want, cfg.AllowSyncInitContext)
			opts := buildReActOptions(context.Background(), cfg, make(chan *schema.AiOutputEvent, 1))
			opts = append(opts, aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true))
			reactCfg := aicommon.NewConfig(context.Background(), opts...)
			require.Equal(t, tc.want, reactCfg.GetConfigBool("AllowSyncInitContext"))
		})
	}
}
