package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestForgeQueryOptionsAcceptNilPaging(t *testing.T) {
	config := NewForgeQueryConfig(
		WithForgeQueryPaging(nil),
		WithForgeFilter_Limit(5),
	)

	require.NotNil(t, config.Paging)
	require.Equal(t, int64(1), config.Paging.GetPage())
	require.Equal(t, int64(5), config.Paging.GetLimit())
}

func TestForgeFilterLimitRepairsNilPaging(t *testing.T) {
	clearPaging := func(config *ForgeQueryConfig) {
		config.Paging = nil
	}
	config := NewForgeQueryConfig(clearPaging, WithForgeFilter_Limit(5))

	require.NotNil(t, config.Paging)
	require.Equal(t, int64(1), config.Paging.GetPage())
	require.Equal(t, int64(5), config.Paging.GetLimit())
}
