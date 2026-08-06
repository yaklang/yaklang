package aiforge

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestRuntimeForgeRegistryIsScopedAndReportsWhetherItHandledAForge(t *testing.T) {
	registry := NewRuntimeForgeRegistry()
	calls := 0
	require.NoError(t, registry.Register(
		"runtime-test",
		func(
			_ context.Context,
			params []*ypb.ExecParamItem,
			_ ...aicommon.ConfigOption,
		) (*ForgeResult, error) {
			calls++
			require.Len(t, params, 1)
			require.Equal(t, "query", params[0].Key)
			return &ForgeResult{Formated: "done"}, nil
		},
	))

	result, handled, err := registry.Execute(
		"runtime-test",
		context.Background(),
		[]*ypb.ExecParamItem{{Key: "query", Value: "hello"}},
	)
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, "done", result.Formated)
	require.Equal(t, 1, calls)

	result, handled, err = registry.Execute("missing", context.Background(), nil)
	require.NoError(t, err)
	require.False(t, handled)
	require.Nil(t, result)

	require.Error(t, registry.Register("runtime-test", func(
		context.Context,
		[]*ypb.ExecParamItem,
		...aicommon.ConfigOption,
	) (*ForgeResult, error) {
		return nil, nil
	}))

	preparation, handled, err := registry.PrepareReAct(
		"runtime-test",
		context.Background(),
		nil,
	)
	require.ErrorContains(t, err, "does not support the ReAct transport")
	require.True(t, handled)
	require.Nil(t, preparation)
}

func TestRuntimeForgeRegistryPreparesARegisteredReActTransport(t *testing.T) {
	registry := NewRuntimeForgeRegistry()
	expected := &RuntimeForgeReActPreparation{
		Options: []aicommon.ConfigOption{aicommon.WithForgeName("runtime-test")},
	}
	var received []*ypb.ExecParamItem
	require.NoError(t, registry.RegisterWithReAct(
		"runtime-test",
		func(
			context.Context,
			[]*ypb.ExecParamItem,
			...aicommon.ConfigOption,
		) (*ForgeResult, error) {
			return nil, nil
		},
		func(
			_ context.Context,
			params []*ypb.ExecParamItem,
		) (*RuntimeForgeReActPreparation, error) {
			received = params
			return expected, nil
		},
	))

	params := []*ypb.ExecParamItem{{Key: "query", Value: "inspect evidence"}}
	preparation, handled, err := registry.PrepareReAct(
		"runtime-test",
		context.Background(),
		params,
	)
	require.NoError(t, err)
	require.True(t, handled)
	require.Same(t, expected, preparation)
	require.Equal(t, params, received)

	preparation, handled, err = registry.PrepareReAct(
		"missing",
		context.Background(),
		nil,
	)
	require.NoError(t, err)
	require.False(t, handled)
	require.Nil(t, preparation)

	require.ErrorContains(t, registry.RegisterWithReAct(
		"missing-preparer",
		func(
			context.Context,
			[]*ypb.ExecParamItem,
			...aicommon.ConfigOption,
		) (*ForgeResult, error) {
			return nil, nil
		},
		nil,
	), "no ReAct preparer")
}
