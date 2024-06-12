package lowhttp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsValidUTF8(t *testing.T) {
	t.Run("valid utf8", func(t *testing.T) {
		b := []byte("\xce\xba\xe1\xbd\xb9\xcf\x83\xce\xbc\xce\xb5")
		valid, remindSize := IsValidUTF8WithRemind(b)
		require.True(t, valid)
		require.Equal(t, 0, remindSize)
	})

	t.Run("remind utf8 1", func(t *testing.T) {
		b := []byte("\xce\xba\xe1\xbd\xb9\xcf\x83\xce\xbc\xce\xb5\xf4")
		valid, remindSize := IsValidUTF8WithRemind(b)
		require.True(t, valid)
		require.Equal(t, 1, remindSize)
	})

	t.Run("remind utf8 2", func(t *testing.T) {
		b := []byte("\xce\xba\xe1\xbd\xb9\xcf\x83\xce\xbc\xce\xb5\xf4\x80")
		valid, remindSize := IsValidUTF8WithRemind(b)
		require.True(t, valid)
		require.Equal(t, 2, remindSize)
	})

	t.Run("invalid utf8 2", func(t *testing.T) {
		b := []byte("\xce\xba\xe1\xbd\xb9\xcf\x83\xce\xbc\xce\xb5\xf4\x90")
		valid, remindSize := IsValidUTF8WithRemind(b)
		require.False(t, valid)
		require.Equal(t, 2, remindSize)
	})

	t.Run("remind utf8 3", func(t *testing.T) {
		b := []byte("\xce\xba\xe1\xbd\xb9\xcf\x83\xce\xbc\xce\xb5\xf4\x80\x80")
		valid, remindSize := IsValidUTF8WithRemind(b)
		require.True(t, valid)
		require.Equal(t, 3, remindSize)
	})

	t.Run("valid utf8 4", func(t *testing.T) {
		b := []byte("\xce\xba\xe1\xbd\xb9\xcf\x83\xce\xbc\xce\xb5\xf4\x80\x80\x80")
		valid, remindSize := IsValidUTF8WithRemind(b)
		require.True(t, valid)
		require.Equal(t, 0, remindSize)
	})

	t.Run("remind utf8 6", func(t *testing.T) {
		b := []byte("\xce\xba\xe1\xbd\xb9\xcf\x83\xce\xbc\xce\xb5\xf4\x80\x80\x80\x80\x80")
		valid, remindSize := IsValidUTF8WithRemind(b)
		require.False(t, valid)
		require.Equal(t, 6, remindSize)
	})
}
