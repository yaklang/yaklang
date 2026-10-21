package vectorstore

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/embedding"
)

var localEmbeddingProbe struct {
	sync.Once
	vector []float32
	err    error
}

// ProbeLocalEmbedding checks the running local service once within one three-second
// budget, including HTTP retries and response reads. Diagnostics must not start
// a model process or wait for model installation/loading. Subsequent calls reuse
// the cached probe result; use LocalEmbedding to embed actual new input.
func ProbeLocalEmbedding(text string) ([]float32, error) {
	localEmbeddingProbe.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		localEmbeddingProbe.vector, localEmbeddingProbe.err = probeLocalEmbedding(ctx, "127.0.0.1:11435", text)
	})
	return append([]float32(nil), localEmbeddingProbe.vector...), localEmbeddingProbe.err
}

func probeLocalEmbedding(ctx context.Context, address, text string) ([]float32, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	client := embedding.NewOpenaiEmbeddingClient(aispec.WithDomain(address),
		aispec.WithNoHTTPS(true), aispec.WithModel("embedding"),
		aispec.WithContext(ctx), aispec.WithTimeout(3))
	vector, err := client.Embedding(text)
	if err != nil {
		return nil, fmt.Errorf("local embedding probe failed at %s (budget 3s; no auto-start): %w", address, err)
	}
	if len(vector) == 0 {
		return nil, fmt.Errorf("local embedding probe returned an empty vector at %s", address)
	}
	return vector, nil
}
