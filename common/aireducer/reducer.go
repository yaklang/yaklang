package aireducer

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"io"
	"os"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/chunkmaker"
)

type Reducer struct {
	config *Config
	input  *chunkmaker.ChunkMaker
}

func (r *Reducer) Run() error {
	if r.config.Memory == nil {
		r.config.Memory = aid.GetDefaultMemory()
	}
	ch := r.input.OutputChannel()
	for {
		select {
		case chunk, ok := <-ch:
			if !ok {
				return nil
			}
			if r.config.callback != nil {
				err := r.config.callback(r.config, r.config.Memory, chunk)
				if err != nil {
					return fmt.Errorf("reducer callback error: %w", err)
				}
				continue
			}
			fmt.Println(spew.Sdump(string(chunk.Data())))
		}
	}
}

func NewReducerFromReader(r io.Reader, opts ...Option) (*Reducer, error) {
	config := NewConfig(opts...)
	chunk := chunkmaker.NewChunkChannelFromReader(config.ctx, r)
	if chunk == nil {
		return nil, errors.New("failed to create chunk channel from reader")
	}
	cm, err := chunkmaker.NewChunkMakerEx(chunk, chunkmaker.NewConfig(
		chunkmaker.WithTimeTrigger(config.TimeTriggerInterval),
		chunkmaker.WithChunkSize(config.ChunkSize),
	))
	if err != nil {
		return nil, fmt.Errorf("failed to create chunk maker: %w", err)
	}

	if config.callback == nil {
		return nil, errors.New("reducer callback is nil, not right")
	}

	return &Reducer{
		input:  cm,
		config: config,
	}, nil
}

func NewReducerFromFile(filename string, opts ...Option) (*Reducer, error) {
	if ok, err := filesys.NewLocalFs().Exists(filename); err != nil {
		return nil, utils.Errorf("failed to check if file[%v] exists", err)
	} else if !ok {
		return nil, utils.Errorf("file [%s] does not exist", filename)
	}

	fp, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filename, err)
	}

	pr, pw := utils.NewPipe()
	go func() {
		defer func() {
			pw.Close()
			fp.Close()
		}()
		io.Copy(pw, fp)
	}()
	return NewReducerFromReader(pr, opts...)
}

func NewReducerFromString(i string, opts ...Option) (*Reducer, error) {
	return NewReducerFromReader(bytes.NewReader([]byte(i)), opts...)
}
