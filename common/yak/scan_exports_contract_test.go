package yak

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/synscanx"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklib/tools"
)

func TestYakScanExportsKeepCallShapeAndContextPrecedence(t *testing.T) {
	runtimeCtx, cancelRuntime := context.WithCancel(context.Background())
	defer cancelRuntime()
	scriptCtx, cancelScript := context.WithCancel(context.Background())
	defer cancelScript()
	scanError := errors.New("fixture scan failed")

	var serviceConfigs []*fp.Config
	serviceScan := func(targets, ports string, opts ...fp.ConfigOption) (chan *fp.MatchResult, error) {
		if targets == "fixture-error" {
			return nil, scanError
		}
		assertScanContractInput(t, targets, ports)
		serviceConfigs = append(serviceConfigs, fp.NewConfig(opts...))
		results := make(chan *fp.MatchResult)
		close(results)
		return results, nil
	}
	var synConfigs []*synscanx.SynxConfig
	synScan := func(targets, ports string, opts ...synscanx.SynxConfigOption) (chan *synscan.SynScanResult, error) {
		if targets == "fixture-error" {
			return nil, scanError
		}
		assertScanContractInput(t, targets, ports)
		config := synscanx.NewDefaultConfig()
		for _, opt := range opts {
			opt(config)
		}
		synConfigs = append(synConfigs, config)
		results := make(chan *synscan.SynScanResult)
		close(results)
		return results, nil
	}
	var synOptionCalls int
	markSynOption := synscanx.SynxConfigOption(func(*synscanx.SynxConfig) { synOptionCalls++ })

	require.IsType(t, serviceScan, tools.FingerprintScanExports["Scan"])
	require.IsType(t, synScan, tools.SynxPortScanExports["Scan"])

	// Run the actual Yak VM and option-binding hooks with in-memory streams.
	engine := antlr4yak.New()
	BindYakitPluginContextToEngine(engine, CreateYakitPluginContext("scan-contract").
		WithContext(runtimeCtx).WithContextCancel(cancelRuntime).WithCliApp(GetHookCliApp(nil)))
	engine.SetVars(map[string]any{
		"scriptCtx":     scriptCtx,
		"markSynOption": markSynOption,
		"servicescan": map[string]any{
			"Scan": serviceScan, "context": tools.FingerprintScanExports["context"],
			"concurrent": tools.FingerprintScanExports["concurrent"],
		},
		"synscan": map[string]any{"Scan": synScan, "context": tools.SynxPortScanExports["context"]},
	})
	require.NoError(t, engine.SafeEvalWithoutCache(runtimeCtx, `
count = 0
results, err = servicescan.Scan("fixture", "1-65535")
assert err == nil
for result = range results { count++ }
serviceOpts = [servicescan.concurrent(17), servicescan.context(scriptCtx)]
for result in servicescan.Scan("fixture", "1-65535", serviceOpts...)~ { count++ }

results, err = synscan.Scan("fixture", "1-65535")
assert err == nil
for result = range results { count++ }
for result in synscan.Scan("fixture", "1-65535", markSynOption, synscan.context(scriptCtx))~ { count++ }
assert count == 0

_, err = servicescan.Scan("fixture-error", "")
assert err.Error() == "fixture scan failed"
_, err = synscan.Scan("fixture-error", "")
assert err.Error() == "fixture scan failed"
`))

	require.Len(t, serviceConfigs, 2)
	require.Same(t, runtimeCtx, serviceConfigs[0].Ctx)
	require.Equal(t, 20, serviceConfigs[0].PoolSize)
	require.Equal(t, 5*time.Second, serviceConfigs[0].ProbeTimeout)
	require.Same(t, scriptCtx, serviceConfigs[1].Ctx)
	require.Equal(t, 17, serviceConfigs[1].PoolSize)
	require.Len(t, synConfigs, 2)
	require.Same(t, runtimeCtx, synConfigs[0].Ctx)
	require.Same(t, scriptCtx, synConfigs[1].Ctx)
	require.Equal(t, 1, synOptionCalls)
}

func assertScanContractInput(t *testing.T, targets, ports string) {
	t.Helper()
	if targets != "fixture" || ports != "1-65535" {
		t.Errorf("scan arguments changed: targets=%q ports=%q", targets, ports)
	}
}
