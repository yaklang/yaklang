package yakvm

import (
	"sync"
	"testing"
)

func TestDebuggerConcurrentReferences(t *testing.T) {
	debugger := &Debugger{Reference: NewReference()}
	shared := &Scope{}
	const workers = 32
	const iterations = 100
	start := make(chan struct{})
	sharedIDs := make(chan int, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			sharedIDs <- debugger.AddScopeRef(shared)
			for range iterations {
				scope := &Scope{}
				id := debugger.AddScopeRef(scope)
				if got, ok := debugger.Reference.VarHM.get(id); !ok || got != scope {
					t.Errorf("scope handle %d resolves to %p, want %p", id, got, scope)
				}
				if got, ok := debugger.Reference.VarHM.getReverse(scope); !ok || got != id {
					t.Errorf("reverse scope handle = %d, want %d", got, id)
				}
				if got := debugger.AddVariableRef(scope); got != id {
					t.Errorf("repeated registration = %d, want %d", got, id)
				}
				debugger.ForceSetVariableRef(id, scope)
				frame := &Frame{}
				fid := debugger.AddFrameRef(frame)
				if got, ok := debugger.Reference.FrameHM.get(fid); !ok || got != frame {
					t.Errorf("frame handle %d did not round trip", fid)
				}
				bp := &Breakpoint{}
				bid := debugger.AddBreakPointRef(bp)
				if got, ok := debugger.Reference.BreakPointHM.get(bid); !ok || got != bp {
					t.Errorf("breakpoint handle %d did not round trip", bid)
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	close(sharedIDs)
	want := debugger.AddScopeRef(shared)
	for got := range sharedIDs {
		if got != want {
			t.Errorf("shared scope got handle %d, want %d", got, want)
		}
	}
}

func TestDebuggerReferenceReset(t *testing.T) {
	hs := newHandlesMap(1)
	value := &Scope{}
	id := hs.create(value)
	hs.reset()
	if _, ok := hs.get(id); ok {
		t.Fatal("reset retained a forward reference")
	}
	if _, ok := hs.getReverse(value); ok {
		t.Fatal("reset retained a reverse reference")
	}
	if got := hs.create(value); got != id {
		t.Fatalf("first handle after reset = %d, want %d", got, id)
	}
}
