package main

import (
	"context"
	"log"
	"os"

	"github.com/yaklang/yaklang/common/ai/tongyi"
	"github.com/yaklang/yaklang/common/ai/tongyi/qwen"
)

func main() {
	model := qwen.QwenVLPlus
	token := os.Getenv("DASHSCOPE_API_KEY")

	if token == "" {
		panic("token is empty")
	}

	cli := dashscopego.NewTongyiClient(model, token).
		SetUploadCache(qwen.NewMemoryFileCache()) // 可以通过 UploadCacher 接口 自定义缓存实现 避免重复上传, 默认使用内存缓存

	homedir, _ := os.UserHomeDir()

	sysContent := qwen.VLContentList{
		{
			Text: "You are a helpful assistant.",
		},
	}

	userContent := qwen.VLContentList{
		{
			Text: "用唐诗体说明一下这张图片中的内容", //nolint:gosmopolitan
		},
		{
			Image: "file://" + homedir + "/Downloads/pandas_img.jpg",
			// Image: "https://dashscope.oss-cn-beijing.aliyuncs.com/images/dog_and_girl.jpeg",
		},
	}

	input := dashscopego.VLInput{
		Messages: []dashscopego.VLMessage{
			{Role: qwen.RoleSystem, Content: &sysContent},
			{Role: qwen.RoleUser, Content: &userContent},
		},
	}
	// callback function:  print stream result
	streamCallbackFn := func(_ context.Context, chunk []byte) error {
		log.Print(string(chunk))
		return nil
	}

	req := &dashscopego.VLRequest{
		Input:       input,
		StreamingFn: streamCallbackFn,
	}

	ctx := context.TODO()
	resp, err := cli.CreateVLCompletion(ctx, req)
	if err != nil {
		panic(err)
	}

	log.Println("\nnon-stream result: ")
	log.Println(resp.Output.Choices[0].Message.Content.ToString())
}
