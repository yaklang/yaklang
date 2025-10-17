package chunkmaker

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"io/fs"
	"os"
)

func NewChunkMakerFromFile(targetFile string, opts ...Option) (ChunkMaker, error) {
	if ok, err := filesys.NewLocalFs().Exists(targetFile); err != nil {
		return nil, utils.Errorf("failed to check if file[%v] exists", err)
	} else if !ok {
		return nil, utils.Errorf("file [%s] does not exist", targetFile)
	}

	isText, err := utils.IsGenericTextFile(targetFile)
	if err != nil {
		log.Errorf("failed to check if file is generic text file: %v", err)
		isText = false
	}
	cfg := NewConfig(opts...)
	if isText {
		fp, err := os.Open(targetFile)
		if err != nil {
			return nil, fmt.Errorf("failed to open file %s: %w", targetFile, err)
		}
		return NewTextChunkMakerEx(NewChunkChannelFromReader(cfg.ctx, fp), cfg)
	} else {
		return NewImageChunkMakerFromFileEx(targetFile, cfg)
	}
}

func NewChunkMakerFromPath(targetPath string, opts ...Option) (ChunkMaker, error) {
	if info, err := filesys.NewLocalFs().Stat(targetPath); err != nil {
		return nil, utils.Errorf("failed to check if path[%v] exists", err)
	} else if !info.IsDir() {
		return NewChunkMakerFromFile(targetPath, opts...)
	}

	cfg := NewConfig(opts...)
	cm := NewMergerChunkMaker(cfg.ctx)
	go func() {
		defer cm.Close()
		err := filesys.Recursive(targetPath, filesys.WithFileStat(func(path string, info fs.FileInfo) error {
			fileChunkMaker, err := NewChunkMakerFromFile(path, opts...)
			if err != nil {
				log.Errorf("failed to create [%s] file chunkMaker: %v", path, err)
				return err
			}
			cm.AddInput(fileChunkMaker.OutputChannel())
			return nil
		}))
		if err != nil {
			log.Errorf(err.Error())
			return
		}
	}()
	return cm, nil
}
