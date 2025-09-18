package aireact

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

var mockEnhanceKnowledgeList = []*aicommon.BasicEnhanceKnowledge{
	aicommon.NewBasicEnhanceKnowledge(
		"Go is a statically typed, compiled programming language designed at Google.",
		"https://golang.org",
		0.95,
	),
	aicommon.NewBasicEnhanceKnowledge(
		"Goroutines are lightweight threads managed by the Go runtime.",
		"https://blog.golang.org/goroutines",
		0.90,
	),
	aicommon.NewBasicEnhanceKnowledge(
		"The Go standard library provides extensive support for networking and web servers.",
		"https://pkg.go.dev/std",
		0.85,
	),
	aicommon.NewBasicEnhanceKnowledge(
		"Channels in Go provide a way for goroutines to communicate with each other and synchronize execution.",
		"https://tour.golang.org/concurrency/2",
		0.88,
	),
	aicommon.NewBasicEnhanceKnowledge(
		"Go modules are the standard for dependency management in modern Go projects.",
		"https://blog.golang.org/using-go-modules",
		0.80,
	),
	aicommon.NewBasicEnhanceKnowledge(
		"The Go tooling includes powerful features for testing, benchmarking, and profiling code.",
		"https://golang.org/doc/go1.10#testing",
		0.78,
	),
	aicommon.NewBasicEnhanceKnowledge(
		"Interfaces in Go provide a way to specify the behavior of an object: if something can do this, then it can be used here.",
		"https://tour.golang.org/methods/9",
		0.82,
	),
}

func NewMockEnhanceHandler() func(ctx context.Context, query string) (<-chan aicommon.EnhanceKnowledge, error) {
	return func(ctx context.Context, query string) (<-chan aicommon.EnhanceKnowledge, error) {
		result := chanx.NewUnlimitedChan[aicommon.EnhanceKnowledge](ctx, 10)
		go func() {
			defer result.Close()
			for _, k := range mockEnhanceKnowledgeList {
				result.SafeFeed(k)
			}
		}()
		return result.OutputChannel(), nil
	}
}
