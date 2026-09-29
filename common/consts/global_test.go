package consts

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetGlobalMaxContentLength(t *testing.T) {
	previous := GetGlobalMaxContentLength()
	t.Cleanup(func() {
		SetGlobalMaxContentLength(previous)
	})

	t.Run("accepts frontend maximum", func(t *testing.T) {
		SetGlobalMaxContentLength(50 * 1024 * 1024)
		require.Equal(t, uint64(50*1024*1024), GetGlobalMaxContentLength())
	})

	t.Run("clamps values above frontend maximum", func(t *testing.T) {
		SetGlobalMaxContentLength(51 * 1024 * 1024)
		require.Equal(t, uint64(50*1024*1024), GetGlobalMaxContentLength())
	})
}

func TestSetHTTPFlowListInlineMaxContentLength(t *testing.T) {
	previous := GetHTTPFlowListInlineMaxContentLength()
	t.Cleanup(func() {
		SetHTTPFlowListInlineMaxContentLength(previous)
	})

	t.Run("allows zero to drop all list packets", func(t *testing.T) {
		SetHTTPFlowListInlineMaxContentLength(0)
		require.Equal(t, uint64(0), GetHTTPFlowListInlineMaxContentLength())
	})

	t.Run("accepts default 300K", func(t *testing.T) {
		SetHTTPFlowListInlineMaxContentLength(DefaultHTTPFlowListInlineMaxContentLength)
		require.Equal(t, DefaultHTTPFlowListInlineMaxContentLength, GetHTTPFlowListInlineMaxContentLength())
	})

	t.Run("clamps values above 500K", func(t *testing.T) {
		SetHTTPFlowListInlineMaxContentLength(501 * 1024)
		require.Equal(t, MaximumHTTPFlowListInlineMaxContentLength, GetHTTPFlowListInlineMaxContentLength())
	})
}
