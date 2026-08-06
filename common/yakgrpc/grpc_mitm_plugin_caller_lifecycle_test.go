//go:build !yakit_exclude

package yakgrpc

import (
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/yak"
)

func TestMITMPluginCallerGlobalRegistrationIsolation(t *testing.T) {
	first := new(yak.MixPluginCaller)
	second := new(yak.MixPluginCaller)
	firstNotify := registerMITMPluginCallerGlobal(first)
	secondNotify := registerMITMPluginCallerGlobal(second)

	unregisterMITMPluginCallerGlobal(first, firstNotify)
	select {
	case <-firstNotify:
	default:
		t.Fatal("superseded MITM lifecycle was not closed")
	}
	caller, notify := loadMITMPluginCallerGlobal()
	if caller != second || notify != secondNotify {
		t.Fatal("closing a superseded MITM lifecycle cleared the active caller")
	}

	unregisterMITMPluginCallerGlobal(second, secondNotify)
	caller, notify = loadMITMPluginCallerGlobal()
	if caller != nil || notify != nil {
		t.Fatal("closing the active MITM lifecycle retained global state")
	}
}

func TestMITMPluginCallerGlobalConcurrentLifecycle(t *testing.T) {
	const lifecycles = 64
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(lifecycles)
	for i := 0; i < lifecycles; i++ {
		go func() {
			defer wg.Done()
			<-start
			caller := new(yak.MixPluginCaller)
			notify := registerMITMPluginCallerGlobal(caller)
			loadedCaller, loadedNotify := loadMITMPluginCallerGlobal()
			if loadedCaller == caller && loadedNotify != notify {
				t.Errorf("caller and lifecycle notification were not loaded atomically")
			}
			unregisterMITMPluginCallerGlobal(caller, notify)
		}()
	}
	close(start)
	wg.Wait()

	caller, notify := loadMITMPluginCallerGlobal()
	if caller != nil || notify != nil {
		t.Fatal("concurrent MITM lifecycles retained global state")
	}
}
