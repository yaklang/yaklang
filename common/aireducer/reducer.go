package aireducer

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"io"
)

type Reducer struct {
	config *Config
	input  chunkmaker.ChunkMaker
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
				if r.config.finishCallback != nil {
					return r.config.finishCallback(r.config, r.config.Memory)
				}
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

func NewReducerEx(maker chunkmaker.ChunkMaker, opts ...Option) (*Reducer, error) {
	config := NewConfig(opts...)
	if maker == nil {
		return nil, errors.New("input chunk maker is nil, not right")
	}

	if config.callback == nil {
		return nil, errors.New("reducer callback is nil, not right")
	}
	return &Reducer{
		input:  maker,
		config: config,
	}, nil
}

func NewReducerFromInputChunk(chunk *chanx.UnlimitedChan[chunkmaker.Chunk], opts ...Option) (*Reducer, error) {
	config := NewConfig(opts...)
	if chunk == nil {
		return nil, errors.New("failed to create chunk channel from reader")
	}
	cm, err := chunkmaker.NewTextChunkMakerEx(chunk, chunkmaker.NewConfig(
		chunkmaker.WithTimeTrigger(config.TimeTriggerInterval),
		chunkmaker.WithChunkSize(config.ChunkSize),
		chunkmaker.WithSeparatorTrigger(config.SeparatorTrigger),
	))
	if err != nil {
		return nil, fmt.Errorf("failed to create chunk maker: %w", err)
	}
	return NewReducerEx(cm, opts...)
}

func NewReducerFromReader(r io.Reader, opts ...Option) (*Reducer, error) {
	config := NewConfig(opts...)
	cm, err := chunkmaker.NewTextChunkMaker(r, config.ChunkMakerOption()...)
	if err != nil {
		return nil, fmt.Errorf("failed to create chunk maker: %w", err)
	}
	return NewReducerEx(cm, opts...)
}

func NewReducerFromString(i string, opts ...Option) (*Reducer, error) {
	return NewReducerFromReader(bytes.NewReader([]byte(i)), opts...)
}

func NewReducerFromFile(filename string, opts ...Option) (*Reducer, error) {
	config := NewConfig(opts...)
	cm, err := chunkmaker.NewChunkMakerFromPath(filename, config.ChunkMakerOption()...)
	if err != nil {
		return nil, err
	}
	return NewReducerEx(cm, opts...)
}
