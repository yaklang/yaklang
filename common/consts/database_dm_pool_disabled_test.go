//go:build !dameng

package consts

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/damenggorm"
)

func TestDMDatabasePoolFactoryStillRequiresNativeDriver(t *testing.T) {
	require.False(t, damenggorm.NativeEnabled())
	db, err := CreateSSAProjectDatabaseRaw(dmPoolTestURL + "&poolMaxOpenConns=1&poolMaxIdleConns=0")
	require.Nil(t, db)
	require.ErrorIs(t, err, damenggorm.ErrNativeUnavailable)
	require.NotContains(t, err.Error(), "private-password")
}
