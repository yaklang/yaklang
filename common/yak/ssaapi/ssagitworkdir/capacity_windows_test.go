//go:build windows

package ssagitworkdir

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestWindowsDiskFullErrorsAreCapacityErrors(t *testing.T) {
	require.True(t, isCapacityError(windows.ERROR_DISK_FULL))
	require.True(t, isCapacityError(fmt.Errorf("wrapped: %w", windows.ERROR_HANDLE_DISK_FULL)))
}
