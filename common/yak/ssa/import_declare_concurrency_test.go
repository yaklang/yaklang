package ssa

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// Prehandler goroutines import types and values into one program while queries
// read them. The maps inside an importDeclareItem and the deferred repair list
// must survive that without "concurrent map read and map write".
func TestImportDeclareConcurrentAccess(t *testing.T) {
	prog := NewProgram(nil, ProgramCacheMemory, ssadb.Application, nil, "", 0)
	prog.Name = "concurrent-import-prog"
	lib := NewProgram(nil, ProgramCacheMemory, ssadb.Application, nil, "", 0)
	lib.Name = "concurrent-import-lib"

	const goroutines = 8
	const rounds = 100

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		g := g
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				name := fmt.Sprintf("T%d_%d", g, i)
				if err := prog.ImportTypeFromLib(lib, name, nil); err != nil {
					t.Errorf("import type %s: %v", name, err)
					return
				}
				prog.ReadImportType(name)
				prog.ReadImportTypeWithPkg(lib.Name, name)
				if err := prog.ImportValueFromLib(lib, name); err != nil {
					t.Errorf("import value %s: %v", name, err)
					return
				}
				prog.ReadImportValue(name)
				prog.ReadImportValueWithPkg(lib.Name, name)
			}
		}()
	}
	wg.Wait()

	for g := 0; g < goroutines; g++ {
		name := fmt.Sprintf("T%d_%d", g, rounds-1)
		_, ok := prog.ReadImportTypeWithPkg(lib.Name, name)
		require.True(t, ok, "type %s must survive concurrent imports", name)
		_, ok = prog.ReadImportValueWithPkg(lib.Name, name)
		require.True(t, ok, "value %s must survive concurrent imports", name)
	}
}
