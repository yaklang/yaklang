package aiforge

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/schema"
)

// Exercise the actual memory -> Config scheduler -> LiteForge validator path.
// An otherwise valid prefix of a malformed array must never be persisted.
func TestAuxiliaryMemoryValidationRetriesBeforePersistence(t *testing.T) {
	for _, mode := range []string{"invalid", "repair-empty", "repair-valid"} {
		t.Run(mode, func(t *testing.T) {
			db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "memory.db"))
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			db.DB().SetMaxOpenConns(1)
			require.NoError(t, db.AutoMigrate(&schema.AIMemoryEntity{}, &schema.AIMemoryCollection{}, &schema.ProjectGeneralStorage{},
				&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{}).Error)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var speedCalls, qualityCalls atomic.Int32
			const accepted = "Go channels support communication between goroutines."
			cfg := aicommon.NewConfig(ctx,
				aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true),
				aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(2),
				aicommon.WithAIRetryWaitFunc(func(context.Context, time.Duration) error { return nil }),
				aicommon.WithQualityPriorityAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					qualityCalls.Add(1)
					return nil, errors.New("unexpected Intelligence call")
				}),
				aicommon.WithSpeedPriorityAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					attempt := speedCalls.Add(1)
					require.Equal(t, "liteforge[memory-triage]", req.GetCallerLabel())
					var count int
					require.NoError(t, db.Model(&schema.AIMemoryEntity{}).Count(&count).Error)
					require.Zero(t, count, "rejected response must not persist its valid prefix before retry")
					body := `{"@action":"memory-triage","memory_entities":[{"content":"rejected prefix must not be saved"},[]]}`
					if attempt > 1 {
						require.Contains(t, req.GetPrompt(), "array item 1")
						switch mode {
						case "repair-empty":
							body = `{"@action":"memory-triage","memory_entities":[]}`
						case "repair-valid":
							body = `{"@action":"memory-triage","memory_entities":[{"content":"` + accepted + `","tags":["Go"],"potential_questions":[],"t":0.9,"a":0.8,"p":0.5,"o":0.9,"e":0.5,"r":0.8,"c":0.7}]}`
						}
					}
					response := c.NewAIResponse()
					response.EmitOutputStream(strings.NewReader(body))
					response.Close()
					return response, nil
				}),
			)
			invoker := mock.NewMockInvoker(ctx)
			invoker.SetConfig(cfg)
			memory, err := aimem.NewAIMemory("validation-test",
				aimem.WithInvoker(invoker), aimem.WithDatabase(db),
				aimem.WithRAGOptions(rag.WithEmbeddingClient(vectorstore.NewDefaultMockEmbedding())),
			)
			require.NoError(t, err)
			defer memory.Close()
			err = memory.HandleMemory("Store a durable fact about Go channels.")
			if mode == "invalid" {
				require.ErrorContains(t, err, "array item 1")
			} else {
				require.NoError(t, err)
			}
			require.EqualValues(t, 2, speedCalls.Load(), "validation retry must use the bounded transaction budget")
			require.Zero(t, qualityCalls.Load())
			var stored []schema.AIMemoryEntity
			require.NoError(t, db.Find(&stored).Error)
			if mode == "repair-valid" {
				require.Len(t, stored, 1)
				require.Equal(t, accepted, stored[0].Content)
			} else {
				require.Empty(t, stored)
			}
		})
	}
}
