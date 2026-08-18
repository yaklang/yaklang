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
