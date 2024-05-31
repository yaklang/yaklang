### dashscopego forked in yaklang

阿里云平台 dashscope api 的 golang 封装 (非官方)

[开通DashScope并创建API-KEY](https://help.aliyun.com/zh/dashscope/developer-reference/activate-dashscope-and-create-an-api-key)

### Examples:
#### 通义千问
- [x] [大语言模型](./example/qwen/stream_call.go)
- [x] [千问VL(视觉理解模型)](./example/qwen_vl/stream_call.go)
- [x] [千问Audio(音频语言模型)](./example/qwen_audio/stream_call.go)
	##### 模型插件调用
	- [x] [pdf解析](./example/qwen_plugins/pdf_extracter/main.go)
	- [x] [Python代码解释器](./example/qwen_plugins/code_interpreter/main.go)
	- [ ] 计算器
	- [ ] 图片生成
	##### Function-Call
	- [x] [自定义工具调用](./example/function_call/main.go)
#### 通义万相
- [x] [文本生成图像](./example/wanx/img_generation.go)
- [ ] 涂鸦作画
- [ ] AnyText图文融合
- [ ] 人像风格重绘
- [ ] 图像背景生成
#### Paraformer(语音识别文字)
- [x] [实时语音识别](./example/paraformer/realtime/speech2text.go)
- [x] [录音文件识别](./example/paraformer/voice_file/recordfile2text.go)
#### 语音合成
- [ ] 文本至语音的实时流式合成
#### 通用文本向量 Embedding
- [x] [同步接口](./example/embedding/main.go)
- [ ] 批处理接口

#### langchaingo-Agent 
- TODO...


```go
import (
	"context"
	"fmt"
	"os"

	"github.com/yaklang/yaklang/common/ai/tongyi"
	"github.com/yaklang/yaklang/common/ai/tongyi/qwen"
)

func main() {
	// 定义客户端
	model := qwen.QwenTurbo
	token := os.Getenv("DASHSCOPE_API_KEY")

	if token == "" {
		panic("token is empty")
	}

	cli := dashscopego.NewTongyiClient(model, token)

	/*
	 * 开启流式输出:
	 * 通过该 Callback Function 获取流式输出的结果, 如果没有定义该回调函数则默认使用非流式输出
	 * 流式输出结果的 request_id/finish_reason/token_usage 等信息在调用完成后返回的 resp 结果中统一获取
	 */
	streamCallbackFn := func(ctx context.Context, chunk []byte) error {
		// 也可以通过外部定义的 channel 将结果传递出去
		fmt.Print(string(chunk))
		return nil
	}

	// 定义请求内容
	// 请求具体字段说明请查阅官方文档的 HTTP调用接口
	content := qwen.TextContent{Text: "讲个冷笑话"}

	input := dashscopego.TextInput{
		Messages: []dashscopego.TextMessage{
			{Role: "user", Content: &content},
		},
	}

	req := &dashscopego.TextRequest{
		Input:       input,             // 请求内容
		StreamingFn: streamCallbackFn,  // 流式输出的回调函数
	}

	ctx := context.TODO()
	resp, err := cli.CreateCompletion(ctx, req)
	if err != nil {
		panic(err)
	}

	/*
	获取结果
	详细字段说明请查阅 HTTP调用接口的出参描述
	如果request中没有定义流式输出的回调函数 StreamingFn, 则使用此方法获取应答内容
	*/ 
	fmt.Println(resp.Output.Choices[0].Message.Content.ToString())

	// 获取 RequestcID, Token 消耗， 结束标识等信息
	fmt.Println(resp.RequestID)
	fmt.Println(resp.Output.Choices[0].FinishReason)
	fmt.Println(resp.Usage.TotalTokens)
}
```
