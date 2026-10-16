package yakit

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPostInitFinalizersRunAfterImports(t *testing.T) {
	__mutexForInit.Lock()
	initializers, finalizers := __initializingDatabase, __finalizingDatabase
	__initializingDatabase, __finalizingDatabase = nil, nil
	__mutexForInit.Unlock()
	t.Cleanup(func() {
		__mutexForInit.Lock()
		__initializingDatabase, __finalizingDatabase = initializers, finalizers
		__mutexForInit.Unlock()
	})
	var order []string
	// Registration order must not make a derived group run before its plugins.
	RegisterPostInitDatabaseFinalizer(func() error { order = append(order, "groups"); return nil })
	RegisterPostInitDatabaseFunction(func() error { order = append(order, "plugins"); return nil })
	require.NoError(t, CallPostInitDatabase())
	require.Equal(t, []string{"plugins", "groups"}, order)

	order = nil
	failed := errors.New("import failed")
	RegisterPostInitDatabaseFunction(func() error { return failed })
	require.ErrorIs(t, CallPostInitDatabase(), failed)
	require.Equal(t, []string{"plugins"}, order, "do not build derived records after a failed import")
}
