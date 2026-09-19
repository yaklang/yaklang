package ssaapi

import "github.com/yaklang/yaklang/common/utils"

// compileBatchPipeline hands completed IR to a background scan/persist stage.
// There is at most one outstanding batch. The producer may parse the next
// batch's bounded AST window, but must Wait before mutating the shared IR.
// Program/type state is still mutable across packages, so reading it from a
// scanner while another builder modifies it is not safe.
type compileBatchPipeline struct {
	done chan error
}

func (p *compileBatchPipeline) Start(work func() error) {
	if p.done != nil {
		panic("compile batch handed off before previous batch was drained")
	}
	p.done = make(chan error, 1)
	done := p.done
	go func() {
		var err error
		defer func() {
			if recovered := recover(); recovered != nil {
				err = utils.Errorf("compile batch scan/persist panic: %v", recovered)
			}
			done <- err
			close(done)
		}()
		err = work()
	}()
}

func (p *compileBatchPipeline) Wait() error {
	if p.done == nil {
		return nil
	}
	err := <-p.done
	p.done = nil
	return err
}
