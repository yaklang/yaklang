package yakgrpc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestEvaluateExpression(t *testing.T) {
	local, err := NewLocalClient()
	require.NoError(t, err)
	ctx := context.Background()

	check := func(t *testing.T, expression string, expectedResult string, expectedBoolResult bool, vars ...map[string]string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
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
			Expression: expression,
			Variables:  variables,
		})

		require.NoError(t, err)
		require.Equal(t, expectedResult, resp.Result)
		require.Equal(t, expectedBoolResult, resp.BoolResult)
	}

	t.Run("bool", func(t *testing.T) {
		check(t, "false || true", "true", true)
	})

	t.Run("string", func(t *testing.T) {
		check(t, `"a"+"b"`, `"ab"`, true)
	})

	t.Run("vars equal", func(t *testing.T) {
		check(t, "a == 1", "true", true, map[string]string{
			"a": "1",
		})
	})

	t.Run("vars with complex logic", func(t *testing.T) {
		check(t, "a && (b || (c || d))", "true", true, map[string]string{
			"a": "true",
			"b": "false",
			"c": "false",
			"d": "true",
		})
	})
}
