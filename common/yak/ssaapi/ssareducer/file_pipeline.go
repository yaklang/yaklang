package ssareducer

import (
	"context"

	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/pipeline"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type FileHandler func(path string, content []byte)

type FileContent struct {
	Path    string
	Content []byte
	AST     ssa.FrontAST
}

func FilesHandler(
	ctx context.Context,
	filesystem filesys_interface.FileSystem,
	paths []string,
	handler func(path string, content []byte) (ssa.FrontAST, error),
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
			if err == nil {
				fileContent.AST = ast
			}
			return fileContent, nil
		},
	)
	parseASTPipe.FeedChannel(readFilePipe.Out())

	return parseASTPipe.Out()
}
