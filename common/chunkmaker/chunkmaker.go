package chunkmaker

import "github.com/yaklang/yaklang/common/utils/chanx"

type Chunk interface {
	IsUTF8() bool
	Data() []byte
}

type UnlimitedInputChan chanx.UnlimitedChan[Chunk]

func NewChunkMaker()