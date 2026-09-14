package base

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLookupItemPreservesPresenceAndLiveUpdates(t *testing.T) {
	config := NewEmptyConfig()
	value, present := config.LookupItem("value")
	require.False(t, present)
	require.Nil(t, value)

	config.SetItem("value", nil)
	value, present = config.LookupItem("value")
	require.True(t, present)
	require.Nil(t, value)

	config.BaseKV.SetItem("value", uint64(17))
	value, present = config.LookupItem("value")
	require.True(t, present)
	require.Equal(t, uint64(17), value)

	copy := CopyConfig(config)
	config.DeleteItem("value")
	value, present = config.LookupItem("value")
	require.False(t, present)
	require.Nil(t, value)
	value, present = copy.LookupItem("value")
	require.True(t, present)
	require.Equal(t, uint64(17), value)
}
