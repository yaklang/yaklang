package databasex

type DBItem interface {
	GetIdInt64() int64
}

type MemoryItem interface {
	GetId() int64
	SetId(int64)
}
