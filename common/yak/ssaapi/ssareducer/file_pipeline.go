package ssareducer

import (
	"context"
	"slices"

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

type FileHandler func(path string, content []byte)

type FileContent struct {
	Path    string
	Content []byte
	AST     ssa.FrontAST
	Err     error
	Editor  *memedit.MemEditor
}

func FilesHandler(
	ctx context.Context,
	filesystem filesys_interface.FileSystem,
	paths []string,
	handler func(path string, content []byte) (ssa.FrontAST, error),
	orderType int,
) <-chan *FileContent {
	bufSize := len(paths)
	readFilePipe := pipeline.NewPipe[string, *FileContent](
		ctx, bufSize, func(path string) (*FileContent, error) {
			content, err := filesystem.ReadFile(path)
			if err != nil {
				return nil, err
			}
			return &FileContent{
				Path:    path,
				Content: content,
			}, nil
		},
	)
	readFilePipe.FeedSlice(paths)

	parseASTPipe := pipeline.NewPipe[*FileContent, *FileContent](
		ctx, bufSize, func(fileContent *FileContent) (*FileContent, error) {
			ast, err := handler(fileContent.Path, fileContent.Content)
			fileContent.AST = ast
			fileContent.Err = err
			return fileContent, nil
		},
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
	case int(OutOfOrder):
		return parseASTPipe.Out()
	case int(Order):
		return sort(-1)
	case int(ReverseOrder):
		return sort(1)
	}

	return parseASTPipe.Out()
}
