package asyncdb

import "context"

type DBItem interface {
	GetIdInt64() int64
}

type MemoryItem interface {
	GetId() int64
	SetId(int64)
}

type MarshalFunc[T MemoryItem, D DBItem] func(T, D)
type FetchFunc[D DBItem] func(context.Context, int) <-chan D
type DeleteFunc[D DBItem] func([]D)
type SaveFunc[D DBItem] func([]D)
type LoadFunc[T MemoryItem, D DBItem] func(int64) (T, D, error)

const defaultBatchSize = 900
