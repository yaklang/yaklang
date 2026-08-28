package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyntaxFlowResultGetValueUnnamedOutOfRange(t *testing.T) {
	r := &SyntaxFlowResult{
		unName: Values{},
	}
	_, err := r.GetValue("_", 0)
	require.Error(t, err, "out-of-range unnamed index must return an error, not panic")
	assert.Contains(t, err.Error(), "index out of range")
}

func TestSyntaxFlowResultGetValueUnnamedNegative(t *testing.T) {
	r := &SyntaxFlowResult{
		unName: Values{},
	}
	_, err := r.GetValue("_", -1)
	require.Error(t, err, "negative unnamed index must return an error, not panic")
}
