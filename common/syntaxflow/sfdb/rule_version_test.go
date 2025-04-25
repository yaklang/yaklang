package sfdb

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestEmbedRuleVersion(t *testing.T) {
	err := EmbedRuleVersion()
	require.NoError(t, err)
}
