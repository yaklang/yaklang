package ssaapi

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadDataflowTimeout(t *testing.T) {
	t.Setenv(dataflowTimeoutEnv, "")
	require.Equal(t, 5*time.Minute, loadDataflowTimeout())

	t.Setenv(dataflowTimeoutEnv, "30")
	require.Equal(t, 30*time.Second, loadDataflowTimeout())

	t.Setenv(dataflowTimeoutEnv, "0")
	require.Equal(t, time.Duration(0), loadDataflowTimeout())

	t.Setenv(dataflowTimeoutEnv, "garbage")
	require.Equal(t, 5*time.Minute, loadDataflowTimeout())

	t.Setenv(dataflowTimeoutEnv, "-5")
	require.Equal(t, 5*time.Minute, loadDataflowTimeout())
}
