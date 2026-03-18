package ssareducer

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type ASTSequenceType int

const (
	OutOfOrder ASTSequenceType = iota
	Order
	ReverseOrder
)

const maxFileSize = 5 * 1024 * 1024 // 5MB

type FileHandler func(path string, content []byte)

type FileStatus int

const (
	None FileStatus = iota
	FileStatusSuccess
	FileStatusFsError
	FileParseASTError
	FileParseASTSuccess
)

type FileContent struct {
	Path     string
	Content  []byte
	AST      ssa.FrontAST
	Status   FileStatus
	Err      error
	Editor   *memedit.MemEditor
	Duration time.Duration
}

func FilesHandler(
	ctx context.Context,
	filesystem filesys_interface.FileSystem,
	paths []string,
	handler func(path string, content []byte, store *utils.SafeMap[any]) (ssa.FrontAST, error),
	initWorker func() *utils.SafeMap[any],
	orderType ASTSequenceType,
	concurrency int,
) <-chan *FileContent {
	if ctx == nil {
		ctx = context.Background()
	}
	if concurrency <= 0 {
		concurrency = 10
	}
	if len(paths) == 0 {
		ch := make(chan *FileContent)
		close(ch)
		return ch
	}

	// Bound in-flight file contents (content + AST) to avoid unbounded memory growth when
	// parsing is faster than downstream consumption (e.g. pre-handler/build).
	outBuf := concurrency * 2
	if outBuf < 16 {
		outBuf = 16
	}
	if outBuf > len(paths) {
		outBuf = len(paths)
	}

	type fileJob struct {
		index int
		path  string
	}
	type fileResult struct {
		index int
		fc    *FileContent
	}

	jobs := make(chan fileJob, outBuf)
	results := make(chan fileResult, outBuf)

	go func() {
		defer close(jobs)
		for i, p := range paths {
			select {
			case <-ctx.Done():
				return
			case jobs <- fileJob{index: i, path: p}:
			}
		}
	}()

	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			var store *utils.SafeMap[any]
			if initWorker != nil {
				store = initWorker()
			}
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-jobs:
					if !ok {
						return
					}

					fc := &FileContent{Path: job.path}
					info, err := filesystem.Stat(job.path)
					if err != nil {
						log.Errorf("stat file[%s] error: %s", job.path, err)
						fc.Err = err
						fc.Status = FileStatusFsError
					} else if info.Size() > maxFileSize {
						err := utils.Errorf("file size %d exceeds max limit %d", info.Size(), maxFileSize)
						log.Errorf("%s %s", err, job.path)
						fc.Err = err
						fc.Status = FileStatusFsError
					} else {
						content, err := filesystem.ReadFile(job.path)
						if err != nil {
							log.Errorf("read file[%s] error: %s", job.path, err)
							fc.Err = err
							fc.Status = FileStatusFsError
						} else {
							fc.Content = content
							fc.Status = FileStatusSuccess
							start := time.Now()
							ast, err := handler(job.path, content, store)
							fc.Duration = time.Since(start)
							fc.AST = ast
							fc.Err = err
							if err != nil {
								fc.Status = FileParseASTError
							} else {
								fc.Status = FileParseASTSuccess
							}
						}
					}

					select {
					case <-ctx.Done():
						return
					case results <- fileResult{index: job.index, fc: fc}:
					}
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	out := make(chan *FileContent, outBuf)
	go func() {
		defer close(out)
		switch orderType {
		case OutOfOrder:
			for r := range results {
				if r.fc == nil {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case out <- r.fc:
				}
			}
			return
		case Order, ReverseOrder:
			ordered := make([]*FileContent, len(paths))
			for r := range results {
				ordered[r.index] = r.fc
			}
			if orderType == ReverseOrder {
				slices.Reverse(ordered)
			}
			for _, fc := range ordered {
				if fc == nil {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case out <- fc:
				}
			}
			return
		default:
			for r := range results {
				if r.fc == nil {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case out <- r.fc:
				}
			}
			return
		}
	}()

	return out
}
