package yakgrpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aiforge"
	"github.com/yaklang/yaklang/common/ai/aiforge/browsercrypto"
	"github.com/yaklang/yaklang/common/browser"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestMultipleOnlineBrowserInstancesForAgentHTTPFlowColor(t *testing.T) {
	a := browser.ExtensionBridgeConnection{DeviceID: "device-a", InstallationID: "install-a",
		ManagedInstance: &browser.ExtensionBridgeManagedInstance{Manager: "ytray", InstanceID: "instance-a", Badge: "A"}}
	b := browser.ExtensionBridgeConnection{DeviceID: "device-b", InstallationID: "install-b",
		ManagedInstance: &browser.ExtensionBridgeManagedInstance{Manager: "ytray", InstanceID: "instance-b", Badge: "B"}}
	duplicate := a
	duplicate.DeviceID = "reconnected-device-a"
	for _, test := range []struct {
		name        string
		connections []browser.ExtensionBridgeConnection
		want        bool
	}{
		{"none", nil, false},
		{"one", []browser.ExtensionBridgeConnection{a}, false},
		{"same-instance-reconnected", []browser.ExtensionBridgeConnection{a, duplicate}, false},
		{"two", []browser.ExtensionBridgeConnection{a, b}, true},
		{"back-to-one", []browser.ExtensionBridgeConnection{b}, false},
		{"invalid-device-does-not-count", []browser.ExtensionBridgeConnection{a, {}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, multipleOnlineBrowserInstances(test.connections))
			for _, connection := range test.connections {
				require.False(t, connection.BrowserInstance().ColorizeHTTPFlow,
					"ordinary Bridge/MITM identities must not opt into Agent colors")
			}
		})
	}
}

func TestServerRuntimeForgeRegistrationIsGenericAndServerScoped(t *testing.T) {
	server := &Server{
		runtimeForges: aiforge.NewRuntimeForgeRegistry(),
	}
	require.NoError(t, server.registerRuntimeForges())

	result, handled, err := server.runtimeForges.Execute(
		browsercrypto.ForgeName,
		context.Background(),
		[]*ypb.ExecParamItem{
			{Key: "device_id", Value: "device-1"},
			{Key: "tab_id", Value: "1"},
		},
	)
	require.ErrorContains(t, err, "bridge is not running")
	require.True(t, handled)
	require.Nil(t, result)
	preparation, handled, err := server.runtimeForges.PrepareReAct(
		browsercrypto.ForgeName,
		context.Background(),
		[]*ypb.ExecParamItem{
			{Key: "device_id", Value: "device-1"},
			{Key: "tab_id", Value: "1"},
		},
	)
	require.ErrorContains(t, err, "bridge is not running")
	require.True(t, handled)
	require.Nil(t, preparation)

	_, handled, err = server.runtimeForges.Execute(
		"not-registered",
		context.Background(),
		nil,
	)
	require.NoError(t, err)
	require.False(t, handled)
}

func TestPrepareRuntimeForgeReActNormalizesAPlainUserQuery(t *testing.T) {
	server := &Server{
		runtimeForges: aiforge.NewRuntimeForgeRegistry(),
	}
	require.NoError(t, server.runtimeForges.RegisterWithReAct(
		"runtime-test",
		func(
			context.Context,
			[]*ypb.ExecParamItem,
			...aicommon.ConfigOption,
		) (*aiforge.ForgeResult, error) {
			return nil, nil
		},
		func(
			_ context.Context,
			params []*ypb.ExecParamItem,
		) (*aiforge.RuntimeForgeReActPreparation, error) {
			require.Equal(t, []*ypb.ExecParamItem{
				{Key: "query", Value: "inspect the selected page"},
			}, params)
			return &aiforge.RuntimeForgeReActPreparation{}, nil
		},
	))

	preparation, handled, err := server.prepareRuntimeForgeReAct(
		"runtime-test",
		context.Background(),
		nil,
		"inspect the selected page",
	)
	require.NoError(t, err)
	require.True(t, handled)
	require.NotNil(t, preparation)
}

func TestExecuteRuntimeForgeNormalizesAPlainUserQuery(t *testing.T) {
	server := &Server{
		runtimeForges: aiforge.NewRuntimeForgeRegistry(),
	}
	require.NoError(t, server.runtimeForges.Register(
		"runtime-test",
		func(
			_ context.Context,
			params []*ypb.ExecParamItem,
			_ ...aicommon.ConfigOption,
		) (*aiforge.ForgeResult, error) {
			require.Equal(t, []*ypb.ExecParamItem{
				{Key: "query", Value: "inspect the selected page"},
			}, params)
			return &aiforge.ForgeResult{Formated: "done"}, nil
		},
	))

	result, handled, err := server.executeRuntimeForge(
		"runtime-test",
		context.Background(),
		nil,
		"inspect the selected page",
	)
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, "done", result.Formated)
}
