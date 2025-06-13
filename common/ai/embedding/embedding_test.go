package embedding

import (
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestOpenaiEmbeddingClient_Embedding(t *testing.T) {
	client := NewOpenaiEmbeddingClient(aispec.WithBaseURL("http://127.0.0.1:8080"))
	embedding, err := client.Embedding("Hello, world!")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(embedding)
}
