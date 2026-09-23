package rag

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiconfig"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestRAGEnhanceManagerConstructorsOnCanceledQuery(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{}, &schema.GeneralStorage{}).Error; err != nil {
		t.Fatal(err)
	}
	previous := consts.GetGormProfileDatabase()
	previousPath := consts.GetCurrentProfileDatabasePath()
	consts.BindProfileDatabase(db, "")
	t.Cleanup(func() { consts.BindProfileDatabase(previous, previousPath) })
	aiconfig.EnsureConfigLoaded()

	var captured *RAGSystemConfig
	var customOptionCalls int
	constructors := []struct {
		name  string
		new   func() *aicommon.EnhanceKnowledgeManager
		check func(context.Context)
	}{
		{name: "default", new: NewRagEnhanceKnowledgeManager},
		{name: "zero options", new: func() *aicommon.EnhanceKnowledgeManager {
			return NewRagEnhanceKnowledgeManagerWithOptions()
		}},
		{
			name: "custom options",
			new: func() *aicommon.EnhanceKnowledgeManager {
				return NewRagEnhanceKnowledgeManagerWithOptions(func(config *RAGSystemConfig) {
					customOptionCalls++
					captured = config
					config.ctx = context.Background()
					config.collectionNames = []string{"custom"}
				})
			},
			check: func(ctx context.Context) {
				if customOptionCalls != 1 || captured == nil {
					t.Fatal("custom option was not applied exactly once")
				}
				if captured.ctx != ctx || len(captured.collectionNames) != 1 || captured.collectionNames[0] != "missing" {
					t.Fatal("request context and collections must override constructor options")
				}
				if captured.everyQueryResultCallback == nil || captured.onQueryFinish == nil || captured.logReaderWithInfo == nil {
					t.Fatal("manager callbacks were not installed")
				}
				reader := strings.NewReader("query log")
				captured.logReaderWithInfo(reader, nil, nil)
				if reader.Len() != 0 {
					t.Fatal("nil emitter did not drain query log")
				}
			},
		},
	}
	for _, test := range constructors {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			results, err := test.new().FetchKnowledgeWithCollections(ctx, []string{"missing"}, "probe")
			if err != nil {
				t.Fatal(err)
			}
			if test.check != nil {
				test.check(ctx)
			}
			select {
			case _, ok := <-results:
				if ok {
					t.Fatal("canceled query yielded knowledge")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("canceled query did not close its result channel")
			}
		})
	}
}
