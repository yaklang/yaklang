package main

import (
	"context"
	"log"
	"os"

	"github.com/yaklang/yaklang/common/ai/tongyi"
	"github.com/yaklang/yaklang/common/ai/tongyi/qwen"
)

func main() {
	model := qwen.QwenTurbo
	token := os.Getenv("DASHSCOPE_API_KEY")

	if token == "" {
		panic("token is empty")
	}

	cli := dashscopego.NewTongyiClient(model, token)

	sysContent := qwen.TextContent{
		Text: "You are a helpful assistant.",
	}

	userContent := qwen.TextContent{
		// Text: "有若干只鸡兔同在一个笼子里,从上面数,有100个头,从下面数,有334只脚。问笼中各有多少只鸡和兔?",
		Text: "使用代码画一个y=x^2的函数图",
	}

	input := dashscopego.TextInput{
		Messages: []dashscopego.TextMessage{
			{Role: qwen.RoleSystem, Content: &sysContent},
			{Role: qwen.RoleUser, Content: &userContent},
		},
	}

	// TODO: 暂时不支持使用 streaming 模式, 报错: {"code":"InvalidParameter","message":"Plugins=[['code_interpreter']] don't support incremental_output"}
	// callback function:  print stream result
	// streamCallbackFn := func(ctx context.Context, chunk []byte) error {
	// 	log.Print(string(chunk))
	// 	return nil
	// }
	req := &dashscopego.TextRequest{
		Input: input,
		// StreamingFn: streamCallbackFn,
		Plugins: qwen.Plugins{qwen.PluginCodeInterpreter: {}},
	}

	ctx := context.TODO()
	resp, err := cli.CreateCompletion(ctx, req)
	if err != nil {
		panic(err)
	}

	log.Println("\nnon-stream result: ")
	// log.Printf("%+v\n", resp.ToJSONStr())

	// 注意大部分plugin的返回结果是 Messages 不是 Message...
	for _, v := range resp.Output.Choices[0].Messages {
		if v.Content != nil {
			log.Printf("%+v\n", v.Content.ToString())
		}
		if v.PluginCall != nil {
			log.Printf("%+v\n", v.PluginCall.ToString())
		}
	}
}
