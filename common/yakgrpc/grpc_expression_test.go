package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestEvaluateExpression(t *testing.T) {
	local, err := NewLocalClient()
	require.NoError(t, err)
	ctx := context.Background()

	check := func(t *testing.T, expression string, importYaklangLibs bool, expectedResult string, expectedBoolResult bool, vars ...map[string]string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		var variables []*ypb.KVPair
		for _, v := range vars {
			for k, v := range v {
				variables = append(variables, &ypb.KVPair{
					Key:   k,
					Value: v,
				})
			}
		}

		resp, err := local.EvaluateExpression(ctx, &ypb.EvaluateExpressionRequest{
			Expression:        expression,
			ImportYaklangLibs: importYaklangLibs,
			Variables:         variables,
		})

		spew.Dump(resp)

		require.NoError(t, err)
		require.Equal(t, expectedResult, resp.Result)
		require.Equal(t, expectedBoolResult, resp.BoolResult)

		multiResp, err := local.EvaluateMultiExpression(ctx, &ypb.EvaluateMultiExpressionRequest{
			Expressions:       []string{expression},
			ImportYaklangLibs: importYaklangLibs,
			Variables:         variables,
		})

		require.NoError(t, err)
		require.Len(t, multiResp.Results, 1)
		require.Equal(t, expectedResult, multiResp.Results[0].Result)
		require.Equal(t, expectedBoolResult, multiResp.Results[0].BoolResult)
	}

	t.Run("bool", func(t *testing.T) {
		check(t, "false || true", false, "true", true)
	})

	t.Run("string", func(t *testing.T) {
		check(t, `"a"+"b"`, false, `"ab"`, true)
	})

	t.Run("vars equal", func(t *testing.T) {
		check(t, "a == 1", false, "true", true, map[string]string{
			"a": "1",
		})
	})

	t.Run("vars with complex logic", func(t *testing.T) {
		check(t, "a && (b || (c || d))", false, "true", true, map[string]string{
			"a": "true",
			"b": "false",
			"c": "false",
			"d": "true",
		})
	})

	t.Run("use yaklang libs", func(t *testing.T) {
		check(t, "max(1,2)", true, "2", true)
	})

}
