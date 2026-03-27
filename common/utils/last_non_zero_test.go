package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLastNonZero(t *testing.T) {
	require.Equal(t, 0, LastNonZero[int]())
	require.Equal(t, 0, LastNonZero(0, 0, 0))
	require.Equal(t, 3, LastNonZero(0, 2, 0, 3))
	require.Equal(t, 5*time.Second, LastNonZero(time.Duration(0), 3*time.Second, 5*time.Second))
}
