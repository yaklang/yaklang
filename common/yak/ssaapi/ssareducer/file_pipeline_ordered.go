package ssareducer

import (
	"context"
	"github.com/yaklang/yaklang/common/utils"
	"sync"
)

// boundedOrderedFiles keeps at most window files between admission and consumer
// Release. Admission follows input order, so a slow head cannot be starved by
// later files holding all slots. Workers retain private parser stores.
func boundedOrderedFiles(ctx context.Context, paths []string, workers, window int,
	initWorker func() *utils.SafeMap[any], parse func(string, *utils.SafeMap[any]) *FileContent,
) <-chan *FileContent {
	out := make(chan *FileContent)
	if len(paths) == 0 {
		close(out)
		return out
	}
	window = max(1, min(window, len(paths)))
	workers = max(1, min(workers, window))
	slots := make(chan struct{}, window)
	jobs := make(chan int)
	type result struct {
		index int
		file  *FileContent
	}
	results := make(chan result, window)
	go func() {
		defer close(jobs)
		for index := range paths {
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				return
			}
			select {
			case jobs <- index:
			case <-ctx.Done():
				<-slots
				return
			}
		}
	}()
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var store *utils.SafeMap[any]
			if initWorker != nil {
				store = initWorker()
			}
			for index := range jobs {
				if ctx.Err() != nil {
					<-slots
					continue
				}
				file := parse(paths[index], store)
				file.setRelease(func() { <-slots })
				select {
				case results <- result{index, file}:
				case <-ctx.Done():
					file.Release()
				}
			}
		}()
	}
	go func() { wg.Wait(); close(results) }()
	go func() {
		defer close(out)
		pending := make(map[int]*FileContent, window)
		defer func() {
			for _, file := range pending {
				file.Release()
			}
		}()
		next := 0
		for result := range results {
			if ctx.Err() != nil {
				result.file.Release()
				continue
			}
			pending[result.index] = result.file
			for {
				file, exists := pending[next]
				if !exists {
					break
				}
				select {
				case out <- file:
					delete(pending, next)
					next++
				case <-ctx.Done():
					break
				}
				if ctx.Err() != nil {
					break
				}
			}
		}
	}()
	return out
}
