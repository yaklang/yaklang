//go:build dameng

package damenggorm

import (
	"database/sql"
	"database/sql/driver"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm/dialects/dameng"
)

func TestNativeDriverDelegation(t *testing.T) {
	require.True(t, NativeEnabled())
	require.Equal(t, dameng.NativeEnabled(), NativeEnabled())
	require.Same(t, dameng.ErrNativeUnavailable, ErrNativeUnavailable)
	db, err := sql.Open(Dialect, "dm://user:password@127.0.0.1:1?schema=SSA_IR")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.Implements(t, (*driver.DriverContext)(nil), db.Driver())
}
