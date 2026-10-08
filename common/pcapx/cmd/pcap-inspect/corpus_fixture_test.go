package main

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"testing"
)

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	path, err := trafficfixture.Materialize(name, t.TempDir())
	require.NoError(t, err)
	return path
}
