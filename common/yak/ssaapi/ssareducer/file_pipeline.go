package ssareducer

import (
	"context"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

type ASTSequenceType = ssaconfig.ASTSequenceType

const (
	OutOfOrder                  = ssaconfig.OutOfOrder
	Order                       = ssaconfig.Order
	ReverseOrder                = ssaconfig.ReverseOrder
	defaultPipeConcurrency      = 10
	defaultOrderedASTBufferFile = 1024
	maxSourceQueueSize          = 8192
)

const maxFileSize = 5 * 1024 * 1024 // 5MB

// pipeInitBufSize caps source-content queue capacity before AST parse. AST
// output itself is unbuffered in OutOfOrder mode, so parsed trees cannot queue
// ahead of the pre-handler consumer.
func pipeInitBufSize(pathCount, compileConcurrency int) int {
	if pathCount < 1 {
		pathCount = 1
	}
	workers := effectivePipeConcurrency(compileConcurrency)
	cand := workers * 2
	if cand < 8 {
		cand = 8
	}
	if raw := strings.TrimSpace(os.Getenv("YAK_SSA_AST_IN_FLIGHT_FILES")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			cand = v
		}
	}
	if cand < 1 {
		cand = 1
	}
	if cand > maxSourceQueueSize {
		cand = maxSourceQueueSize
	}
	if pathCount < cand {
		return pathCount
	}
	return cand
}

func effectivePipeConcurrency(concurrency int) int {
	if concurrency > 0 {
		return concurrency
	}
	return defaultPipeConcurrency
}

func orderedASTBufferFileLimit() int {
	limit := defaultOrderedASTBufferFile
	if raw := strings.TrimSpace(os.Getenv("YAK_SSA_ORDERED_AST_MAX_FILES")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			limit = v
		}
	}
	if limit < 0 {
		return 0
	}
	return limit
}

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
	bufSize := pipeInitBufSize(len(paths), concurrency)
	concurrency = effectivePipeConcurrency(concurrency)

	orderType = effectiveASTSequence(orderType, len(paths))

	readFile := func(path string) *FileContent {
		info, err := filesystem.Stat(path)
		if err != nil {
			log.Errorf("stat file[%s] error: %s", path, err)
			return &FileContent{
				Path:   path,
				Err:    err,
				Status: FileStatusFsError,
			}
		}
		if info.Size() > maxFileSize {
			err := utils.Errorf("file size %d exceeds max limit %d", info.Size(), maxFileSize)
			log.Errorf("%s %s", err, path)
			return &FileContent{
				Path:   path,
				Err:    err,
				Status: FileStatusFsError,
			}
		}

		content, err := filesystem.ReadFile(path)
		if err != nil {
			log.Errorf("read file[%s] error: %s", path, err)
			return &FileContent{
				Path:   path,
				Err:    err,
				Status: FileStatusFsError,
			}
		}
		return &FileContent{
			Path:    path,
			Content: content,
			Err:     err,
			Status:  FileStatusSuccess,
		}
	}

	parseFile := func(fileContent *FileContent, store *utils.SafeMap[any]) *FileContent {
		if fileContent.Status == FileStatusFsError {
			return fileContent
		}
		start := time.Now()
		ast, err := handler(fileContent.Path, fileContent.Content, store)
		fileContent.Duration = time.Since(start)
		fileContent.AST = ast
		fileContent.Err = err
		if err != nil {
			log.Errorf("parse file[%s] error: %s", fileContent.Path, err)
			fileContent.Status = FileParseASTError
		} else {
			fileContent.Status = FileParseASTSuccess
		}
		return fileContent
	}

	parseOut := runBoundedFilePipeline(ctx, paths, bufSize, concurrency, readFile, parseFile, initWorker)

	sort := func(index int) <-chan *FileContent {
		out := make([]*FileContent, 0, len(paths))
		for fc := range parseOut {
			out = append(out, fc)
		}

		pathIndex := make(map[string]int, len(paths))
		for i, p := range paths {
			pathIndex[p] = i
		}

		slices.SortFunc(out, func(a, b *FileContent) int {
			indexA := pathIndex[a.Path]
			indexB := pathIndex[b.Path]
			if indexA < indexB {
				return index
			}
			if indexA > indexB {
				return -index
			}
			return 0
		})
		ch := make(chan *FileContent, bufSize)
		go func() {
			defer close(ch)
			for _, fc := range out {
				ch <- fc
			}
		}()
		return ch
	}

	switch orderType {
	case OutOfOrder:
		return parseOut
	case Order:
		return sort(-1)
	case ReverseOrder:
		return sort(1)
	}

	return parseOut
}

func effectiveASTSequence(orderType ASTSequenceType, pathCount int) ASTSequenceType {
	if orderType == OutOfOrder {
		return orderType
	}
	limit := orderedASTBufferFileLimit()
	if limit > 0 && pathCount <= limit {
		return orderType
	}
	log.Warnf(
		"[ssa-compile] AST order mode buffers all parsed trees; downgrade to OutOfOrder for %d files (limit=%d)",
		pathCount,
		limit,
	)
	return OutOfOrder
}

func runBoundedFilePipeline(
	ctx context.Context,
	paths []string,
	bufSize int,
	concurrency int,
	readFile func(string) *FileContent,
	parseFile func(*FileContent, *utils.SafeMap[any]) *FileContent,
	initWorker func() *utils.SafeMap[any],
) <-chan *FileContent {
	pathCh := make(chan string, bufSize)
	readCh := make(chan *FileContent, bufSize)
	parseCh := make(chan *FileContent)

	go func() {
		defer close(pathCh)
		for _, path := range paths {
			select {
			case <-ctx.Done():
				return
			case pathCh <- path:
			}
		}
	}()

	var readWG sync.WaitGroup
	readWG.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer readWG.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case path, ok := <-pathCh:
					if !ok {
						return
					}
					fileContent := readFile(path)
					select {
					case <-ctx.Done():
						return
					case readCh <- fileContent:
					}
				}
			}
		}()
	}
	go func() {
		readWG.Wait()
		close(readCh)
	}()

	var parseWG sync.WaitGroup
	parseWG.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer parseWG.Done()
			var store *utils.SafeMap[any]
			if initWorker != nil {
				store = initWorker()
			}
			for {
				select {
				case <-ctx.Done():
					return
				case fileContent, ok := <-readCh:
					if !ok {
						return
					}
					fileContent = parseFile(fileContent, store)
					select {
					case <-ctx.Done():
						return
					case parseCh <- fileContent:
					}
				}
			}
		}()
	}
	go func() {
		parseWG.Wait()
		close(parseCh)
	}()

	return parseCh
}
