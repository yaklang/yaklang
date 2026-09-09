package aicommon

import (
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

func lookupPersistentCheckpoint(db *gorm.DB, runtime string, seq int64, lookup func(*gorm.DB, string, int64) (*schema.AiCheckpoint, bool)) (*schema.AiCheckpoint, bool) {
	if db == nil {
		return nil, false
	}
	return lookup(db, runtime, seq)
}

// WithEphemeralStorage keeps checkpoints in memory and never opens a database.
func WithEphemeralStorage() ConfigOption {
	return func(c *Config) error {
		c.BaseCheckpointableStorage = &BaseCheckpointableStorage{ephemeral: true}
		c.DisableCreateDBRuntime = true
		c.SaveEvent = false
		c.DisableLocalContext = true
		return nil
	}
}
