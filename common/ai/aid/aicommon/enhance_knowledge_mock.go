package aicommon

import (
	"context"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

func NewMockEKManagerAndToken() (*EnhanceKnowledgeManager, string) {
	tokenUUID := uuid.NewString()
	checkData := NewBasicEnhanceKnowledge(
		tokenUUID,
		"mock",
		0.82,
	)

	return NewEnhanceKnowledgeManager(func(ctx context.Context, e *Emitter, query string) (<-chan EnhanceKnowledge, error) {
		result := chanx.NewUnlimitedChan[EnhanceKnowledge](ctx, 10)
		go func() {
			defer result.Close()
			for _, k := range []EnhanceKnowledge{checkData} {
				result.SafeFeed(k)
			}
		}()
		return result.OutputChannel(), nil
	}), tokenUUID
}

func NewDifferentResultEKManager(token, okToken string) *EnhanceKnowledgeManager {
	checkData1 := NewBasicEnhanceKnowledge(
		token,
		"mock",
		0.82,
	)

	checkData2 := NewBasicEnhanceKnowledge(
		okToken,
		"mock",
		0.82,
	)

	first := true

	return NewEnhanceKnowledgeManager(func(ctx context.Context, e *Emitter, query string) (<-chan EnhanceKnowledge, error) {
		if first {
			first = false
			result := chanx.NewUnlimitedChan[EnhanceKnowledge](ctx, 10)
			go func() {
				defer result.Close()
				for _, k := range []EnhanceKnowledge{checkData1} {
					result.SafeFeed(k)
				}
			}()
			return result.OutputChannel(), nil
		}

		result := chanx.NewUnlimitedChan[EnhanceKnowledge](ctx, 10)
		go func() {
			defer result.Close()
			for _, k := range []EnhanceKnowledge{checkData2} {
				result.SafeFeed(k)
			}
		}()
		return result.OutputChannel(), nil

	})
}
