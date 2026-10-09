package mutate

import (
	"context"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
)

type payloadDatabaseContextKey struct{}

// WithPayloadDatabaseContext binds dictionary rendering to one invocation.
// It preserves request cancellation without changing the global profile DB.
func WithPayloadDatabaseContext(ctx context.Context, db *gorm.DB) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, payloadDatabaseContextKey{}, db)
}

func payloadDatabaseFromContext(ctx context.Context) *gorm.DB {
	if ctx != nil {
		if db, ok := ctx.Value(payloadDatabaseContextKey{}).(*gorm.DB); ok {
			return db
		}
	}
	return consts.GetGormProfileDatabase()
}

func Fuzz_WithPayloadDatabase(db *gorm.DB) FuzzConfigOpt {
	return func(c *FuzzTagConfig) { c.payloadDatabase = db }
}
