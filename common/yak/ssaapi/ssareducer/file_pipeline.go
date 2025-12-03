package ssareducer

import (
	"context"
	"slices"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/pipeline"
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
	Path    string
	Content []byte
	AST     ssa.FrontAST
	Status  FileStatus
	Err     error
	Editor  *memedit.MemEditor
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
	bufSize := len(paths)
	readFilePipe := pipeline.NewPipe[string, *FileContent](
		ctx, bufSize, func(path string) (*FileContent, error) {
			// check file size with maxFileSize
			info, err := filesystem.Stat(path)
			if err != nil {
				log.Errorf("stat file[%s] error: %s", path, err)
				return &FileContent{
					Path:   path,
					Err:    err,
					Status: FileStatusFsError,
				}, nil
			}
			if info.Size() > maxFileSize {
				err := utils.Errorf("file size %d exceeds max limit %d", info.Size(), maxFileSize)
				log.Errorf("%s %s", err, path)
				return &FileContent{
					Path:   path,
					Err:    err,
					Status: FileStatusFsError,
				}, nil
			}

			content, err := filesystem.ReadFile(path)
			if err != nil {
				log.Errorf("read file[%s] error: %s", path, err)
				return &FileContent{
					Path:   path,
					Err:    err,
					Status: FileStatusFsError,
				}, nil
			}
			var fileErr error = err
			// Check if content is a text file
			return &FileContent{
				Path:    path,
				Content: content,
				Err:     fileErr,
				Status:  FileStatusSuccess,
			}, nil
		},
		concurrency,
	)
	readFilePipe.FeedSlice(paths)

	parseASTPipe := pipeline.NewPipeWithStore[*FileContent, *FileContent](
		ctx, bufSize, func(fileContent *FileContent, store *utils.SafeMap[any]) (*FileContent, error) {
			if fileContent.Status == FileStatusFsError {
				return fileContent, nil
			}
			ast, err := handler(fileContent.Path, fileContent.Content, store)
			fileContent.AST = ast
			fileContent.Err = err
			if err != nil {
				log.Errorf("parse file[%s] error: %s", fileContent.Path, err)
				fileContent.Status = FileParseASTError
			} else {
				fileContent.Status = FileParseASTSuccess
			}
			return fileContent, nil
		},
		initWorker,
		concurrency,
	)

	sort := func(index int) <-chan *FileContent {
		out := make([]*FileContent, 0, len(paths))
		for fc := range parseASTPipe.Out() {
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

	parseASTPipe.FeedChannel(readFilePipe.Out())
	switch orderType {
	case OutOfOrder:
		return parseASTPipe.Out()
	case Order:
		return sort(-1)
	case ReverseOrder:
		return sort(1)
	}

	return parseASTPipe.Out()
}
