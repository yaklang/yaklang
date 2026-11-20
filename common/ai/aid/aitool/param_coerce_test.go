package aitool

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateParamsCoercesScalarTypes(t *testing.T) {
	tool, err := New("coerce_test",
		WithIntegerParam("timeout"),
		WithStringParam("accepted-exit-codes"),
		WithStringParam("command"),
		WithSimpleCallback(func(params InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			return nil, nil
		}),
	)
	require.NoError(t, err)

	params := map[string]any{
		"command":             "curl http://169.254.169.254/latest/meta-data/",
		"timeout":             "30",
		"accepted-exit-codes": 0,
	}
	valid, errs := tool.ValidateParams(params)
	require.True(t, valid, "errors: %v", errs)
	require.Equal(t, 30, params["timeout"])
	require.Equal(t, "0", params["accepted-exit-codes"])
}
