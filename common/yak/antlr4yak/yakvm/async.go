package yakvm

import (
	"errors"
	"fmt"
	"sync"
)

const maxAsyncErrors = 64

// asyncErrors bounds retained errors independently of the number of workers.
type asyncErrors struct {
	mu      sync.Mutex
	errors  []error
	dropped int
}

func (e *asyncErrors) add(err error) {
	if err == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.errors) < maxAsyncErrors {
		e.errors = append(e.errors, err)
	} else {
		e.dropped++
	}
}

func (e *asyncErrors) result(clear bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	errs := append([]error(nil), e.errors...)
	if e.dropped != 0 {
		errs = append(errs, fmt.Errorf("%d additional async errors omitted", e.dropped))
	}
	if clear {
		e.errors = nil
		e.dropped = 0
	}
	return errors.Join(errs...)
}

type asyncExecution struct {
	wait   sync.WaitGroup
	errors asyncErrors
}

// WaitAsync waits for this execution's workers, including nested workers, and
// returns their failures. Call after scheduling, outside an async worker.
// Independent executions of a shared VM have independent error collections.
func (f *Frame) WaitAsync() error {
	f.asyncExecution.wait.Wait()
	return f.asyncExecution.errors.result(false)
}

// AsyncWaitError drains the VM-wide error aggregate after the legacy AsyncWait.
// Frame.WaitAsync provides execution isolation. Finish scheduling before waiting.
func (v *VirtualMachine) AsyncWaitError() error {
	v.AsyncWait()
	return v.asyncErrors.result(true)
}

func (v *VirtualMachine) startAsync(f *Frame, recoverPanic bool, run func() error) {
	execution := f.asyncExecution
	execution.wait.Add(1)
	v.AsyncStart()
	go func() {
		defer v.AsyncEnd()
		defer execution.wait.Done()
		record := func(err error) {
			execution.errors.add(err)
			v.asyncErrors.add(err)
		}
		if recoverPanic {
			defer func() {
				if p := recover(); p != nil {
					if err, ok := p.(error); ok {
						record(fmt.Errorf("async call panic: %w", err))
					} else {
						record(fmt.Errorf("async call panic: %v", p))
					}
				}
			}()
		}
		record(run())
	}()
}
