package yakgrpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/aiforge/browsercrypto"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

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
