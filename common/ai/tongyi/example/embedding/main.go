package main

import (
	"context"
	"fmt"
	"os"

	"github.com/yaklang/yaklang/common/ai/tongyi"
	"github.com/yaklang/yaklang/common/ai/tongyi/embedding"
)

func main() {
	model := embedding.TextEmbeddingV2
	token := os.Getenv("DASHSCOPE_API_KEY")
	if token == "" {
		panic("token is empty")
	}

	textInputs := []string{"风急天高猿啸哀", "渚清沙白鸟飞回", "无边落木萧萧下", "不尽长江滚滚来"}

	cli := dashscopego.NewTongyiClient(model, token)
	ctx := context.TODO()

	req := &embedding.Request{
		Model: model,
		Params: embedding.Params{
			TextType: embedding.TypeDocument, // 默认值
		},
		Input: embedding.Input{
			Texts: textInputs,
		},
	}

	embeddings, totalToken, err := cli.CreateEmbedding(ctx, req)
	if err != nil {
		panic(err)
	}

	//nolint:all
	fmt.Println("embeddings:", embeddings)
	//nolint:all
	fmt.Println("used tokens:", totalToken)
}
