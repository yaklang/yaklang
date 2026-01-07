package vectorstore

import (
	"context"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
)

func (s *SQLiteVectorStoreHNSW) startDbActionQueue(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case fn := <-s.asyncDBQueue.OutputChannel():
				safeCall := func() {
					defer func() {
						if r := recover(); r != nil {
							log.Errorf("panic recovered in SQLiteVectorStoreHNSW db action: %v", r)
						}
						err := fn(s.db)
						if err != nil {
							log.Errorf("failed to execute db action in queue: %v", err)
						}
					}()
				}
				safeCall()
			}
		}
	}()
}

func (s *SQLiteVectorStoreHNSW) enqueueDbAction(fn func(db *gorm.DB) error) {
	s.asyncDBOnce.Do(func() {
		s.startDbActionQueue(context.Background())
	})
	s.asyncDBQueue.SafeFeed(fn)
}
