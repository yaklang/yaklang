package main

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"github.com/yaklang/yaklang/common/ai/tongyi"
	"github.com/yaklang/yaklang/common/ai/tongyi/paraformer"
)

func main() {
	model := paraformer.ParaformerV1
	token := os.Getenv("DASHSCOPE_API_KEY")
	if token == "" {
		panic("token is empty")
	}

	cli := dashscopego.NewTongyiClient(model, token)

	usr, err := user.Current()
	if err != nil {
		panic(err)
	}

	voiceFile := filepath.Join(usr.HomeDir, "Desktop", "hello_world_female2.wav")
	filePath := "file://" + voiceFile

	req := &paraformer.AsyncTaskRequest{
		Model: paraformer.ParaformerV1,
		Input: paraformer.AsyncInput{
			// 官方示例中使用的远程文件.
			// FileURLs: []string{"https://dashscope.oss-cn-beijing.aliyuncs.com/samples/audio/paraformer/hello_world_female2.wav"},
			// 本地文件.
			FileURLs: []string{filePath},
		},
		Download: true, // 是否下载异步任务结果.
	}

	resp, err := cli.CreateVoiceFileToTextGeneration(context.TODO(), req)
	if err != nil {
		panic(err)
	}

	// 如果不需要下载异步任务的结果，仅获取异步任务的 task_id 后自行轮询结果.
	fmt.Println("taskInfo: ", resp.AsyncTaskResp) //nolint:all
	// 当 request 中设置了 Download = true 时, 等待语音识别结果输出.
	fmt.Println("等待语音识别结果输出...") //nolint:all
	for _, v := range resp.FileResults {
		for _, v2 := range v.Transcripts {
			fmt.Println(v2.Text) //nolint:all
		}
	}
}
