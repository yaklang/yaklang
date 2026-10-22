package ssa

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func testBuilderNamed(t *testing.T, name string) *FunctionBuilder {
	t.Helper()
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(name))
	require.NoError(t, err)
	prog := NewProgram(cfg, ProgramCacheMemory, Application, nil, "", 0)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	require.NotNil(t, builder)
	return builder
}

// RegisterMagicMethod used to hold a package-level mutex across storeField.
// Member lowering re-enters GetMagicMethod on the same goroutine, which
// deadlocked ARTEX compiles. This must return.
func TestBlueprintMagicMethodRegisterDoesNotDeadlock(t *testing.T) {
	builder := testBuilderNamed(t, t.Name())
	done := make(chan Value, 1)
	go func() {
		blueprint := builder.CreateBlueprintAndSetConstruct("Reenter")
		done <- blueprint.GetMagicMethod(Constructor, builder)
	}()
	select {
	case got := <-done:
		require.False(t, utils.IsNil(got))
	case <-time.After(5 * time.Second):
		t.Fatal("RegisterMagicMethod deadlocked while materializing a constructor")
	}
}

// Several builders may materialize one shared blueprint's constructor.
// The SafeMap publication must not race and must not stick on a lock.
func TestBlueprintMagicMethodConcurrentMaterialize(t *testing.T) {
	blueprint := NewBlueprint("SharedCtor")
	name := t.Name()
	const n = 8
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(fmt.Sprintf("%s-%d", name, i)))
			if err != nil {
				errCh <- err
				return
			}
			prog := NewProgram(cfg, ProgramCacheMemory, Application, nil, "", 0)
			builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
			if builder == nil {
				errCh <- fmt.Errorf("nil builder %d", i)
				return
			}
			got := blueprint.GetMagicMethod(Constructor, builder)
			if utils.IsNil(got) {
				errCh <- fmt.Errorf("nil constructor %d", i)
				return
			}
			again := blueprint.GetMagicMethod(Constructor, builder)
			if utils.IsNil(again) || again.GetId() == 0 {
				errCh <- fmt.Errorf("second lookup failed %d", i)
			}
		}(i)
	}
	finished := make(chan struct{})
	go func() {
		wg.Wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent GetMagicMethod deadlocked")
	}
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
}
