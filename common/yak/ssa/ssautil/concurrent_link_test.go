package ssautil

import (
	"sync"
	"testing"
)

func TestConcurrentReadTrim(t *testing.T) {
	scope := NewRootVersionedTable("*dummySSAValue", func(idx int, name string, local bool, parent ScopedVersionedTableIF[*dummySSAValue]) VersionedIF[*dummySSAValue] {
		return NewVersioned(idx, name, local, parent)
	})
	const seeded = 64
	old := make([]VersionedIF[*dummySSAValue], 0, seeded)
	for i := 0; i < seeded; i++ {
		variable := scope.CreateVariable("x", true)
		scope.AssignVariable(variable, &dummySSAValue{id: int64(i + 1)})
		old = append(old, variable)
	}

	var start sync.WaitGroup
	start.Add(1)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start.Wait()
			for n := 0; n < 2000; n++ {
				_ = scope.ReadVariable("x")
				_ = scope.GetAllVariablesByName("x", true)
				_ = scope.GetAllVariables()
				_ = scope.GetAllVariableNames()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		start.Wait()
		for i := 0; i < 400; i++ {
			variable := scope.CreateVariable("x", true)
			scope.AssignVariable(variable, &dummySSAValue{id: int64(1000 + i)})
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		start.Wait()
		for pass := 0; pass < 8; pass++ {
			for _, variable := range old {
				scope.TrimSettledVersion(variable)
			}
		}
	}()
	start.Done()
	wg.Wait()
	if scope.ReadVariable("x", true) == nil {
		t.Fatal("lexical head missing after concurrent trim")
	}
}
