package sfvm

import (
	"github.com/yaklang/yaklang/common/utils"
)

type IterContext struct {
	// dynamic
	originValues chan ValueOperator
	results      []bool
	counter      int
}

// start iter
func (s *SFFrame) IterStart(vs ValueOperator) {
	channel := make(chan ValueOperator)
	go func() {
		defer close(channel)
		_ = vs.Recursive(func(vo ValueOperator) error {
			channel <- vo
			return nil
		})
	}()

	iter := &IterContext{}
	iter.counter = 0
	iter.originValues = channel
	s.iterStack.Push(iter)
}

// get next value, or return false if no more value, jump to end
func (s *SFFrame) IterNext() (ValueOperator, bool, error) {
	iter := s.iterStack.Pop()
	defer s.iterStack.Push(iter)
	if iter == nil {
		return nil, false, utils.Error("BUG: iterContext is nil")
	}

	if iter.originValues == nil {
		return nil, false, utils.Error("BUG: iterContext.originValues is nil")
	}

	val, ok := <-iter.originValues
	if !ok {
		iter.originValues = nil
		return nil, false, nil
	}
	iter.counter++
	return val, true, nil
}

// check value, and set result
func (s *SFFrame) IterLatch(val ValueOperator) error {
	iter := s.iterStack.Pop()
	defer s.iterStack.Push(iter)
	if iter == nil {
		return utils.Error("BUG: iterContext is nil")
	}

	s.debugSubLog("iter index: %d", iter.counter)
	iter.counter++

	if val.IsEmpty() {
		iter.results = append(iter.results, false)
	} else {
		iter.results = append(iter.results, true)
	}

	s.debugSubLog("idx: %v", iter.results[len(iter.results)-1])
	return nil
}

// end iter, pop and collect results to conditionStack
func (s *SFFrame) IterEnd() error {
	iter := s.iterStack.Pop() // abort this iter
	if iter == nil {
		return utils.Error("BUG: iterContext is nil")
	}
	//results := iter.results
	results := s.conditionStack.Peek()
	s.debugSubLog("<< push condition results[len: %v]", len(results))
	//s.conditionStack.Push(results)
	return nil
}
