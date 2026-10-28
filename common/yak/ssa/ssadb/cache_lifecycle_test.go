package ssadb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProgramRowCacheClosesChildrenAndReloadsFresh(t *testing.T) {
	parent := newProgramRowCache[*IrCode]()
	defer parent.Close()
	child := getProgramRowCache(parent, "program")
	child.Set(1, &IrCode{CodeID: 1})
	require.Equal(t, 1, child.Count())
	parent.Delete("program")
	require.Eventually(t, func() bool { return child.Count() == 0 }, time.Second, time.Millisecond,
		"parent removal must close and clear the child row cache")
	fresh := getProgramRowCache(parent, "program")
	require.NotSame(t, child, fresh, "a completed single-flight result must not resurrect a closed child")
	require.Zero(t, fresh.Count())
	fresh.Set(2, &IrCode{CodeID: 2})
	_, ok := fresh.Get(2)
	require.True(t, ok)
}

func TestProgramRowCacheExpirationClosesChild(t *testing.T) {
	parent := newProgramRowCache[*IrType]()
	defer parent.Close()
	parent.SetTTL(10 * time.Millisecond)
	child := getProgramRowCache(parent, "program")
	child.Set(1, &IrType{})
	require.Eventually(t, func() bool { return child.Count() == 0 }, time.Second, time.Millisecond,
		"TTL eviction must end the child cache's lifetime")
}
