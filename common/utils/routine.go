package utils

import "sync"

func WaitRoutinesFromSlice[T any](arg []T, job func(T)) {
	var wg sync.WaitGroup
	for _, v := range arg {
		wg.Add(1)
		go func(v T) {
			defer wg.Done()
			job(v)
		}(v)
	}
	wg.Wait()
}
