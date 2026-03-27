package dbcache

import "github.com/yaklang/yaklang/common/utils"

type MemoryItem interface {
	GetId() int64
	SetId(int64)
}

type MarshalFunc[T MemoryItem, D any] func(T, utils.EvictionReason) (D, error)
type SaveFunc[D any] func([]D) error
type LoadFunc[T any] func(int64) (T, error)

const defaultBatchSize = 900
