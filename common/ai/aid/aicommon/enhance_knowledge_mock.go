package aicommon

import (
	"context"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

var mockEnhanceKnowledgeList = []*BasicEnhanceKnowledge{
	NewBasicEnhanceKnowledge(
		"Go is a statically typed, compiled programming language designed at Google.",
		"https://golang.org",
		0.95,
	),
	NewBasicEnhanceKnowledge(
		"Goroutines are lightweight threads managed by the Go runtime.",
		"https://blog.golang.org/goroutines",
		0.90,
	),
	NewBasicEnhanceKnowledge(
		"The Go standard library provides extensive support for networking and web servers.",
		"https://pkg.go.dev/std",
		0.85,
	),
	NewBasicEnhanceKnowledge(
		"Channels in Go provide a way for goroutines to communicate with each other and synchronize execution.",
		"https://tour.golang.org/concurrency/2",
		0.88,
	),
	NewBasicEnhanceKnowledge(
		"Go modules are the standard for dependency management in modern Go projects.",
		"https://blog.golang.org/using-go-modules",
		0.80,
	),
	NewBasicEnhanceKnowledge(
		"The Go tooling includes powerful features for testing, benchmarking, and profiling code.",
		"https://golang.org/doc/go1.10#testing",
		0.78,
	),
	NewBasicEnhanceKnowledge(
		"Interfaces in Go provide a way to specify the behavior of an object: if something can do this, then it can be used here.",
		"https://tour.golang.org/methods/9",
		0.82,
	),
}

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
