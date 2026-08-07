package schema

// AIMidtermArchiveEntity 与 AIMemoryEntity 字段完全一致，仅 TableName 不同。
// 用于中期记忆（timeline midterm archive）的物理隔离存储，避免与普通长期记忆混表。
type AIMidtermArchiveEntity struct {
	AIMemoryEntity
}

// TableName 覆盖为独立的中期记忆表
func (a *AIMidtermArchiveEntity) TableName() string {
	return "ai_midterm_archive_entities_v1"
}

// AIMidtermArchiveCollection 与 AIMemoryCollection 字段完全一致，仅 TableName 不同。
// 用于中期记忆 HNSW 索引的物理隔离存储。
type AIMidtermArchiveCollection struct {
	AIMemoryCollection
}

// TableName 覆盖为独立的中期记忆 HNSW collection 表
func (a *AIMidtermArchiveCollection) TableName() string {
	return "ai_midterm_archive_collections_v1"
}
