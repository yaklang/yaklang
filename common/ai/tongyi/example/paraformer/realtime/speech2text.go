package main

import (
	"bufio"
	"context"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/yaklang/yaklang/common/ai/tongyi"
	"github.com/yaklang/yaklang/common/ai/tongyi/paraformer"
)

func main() {
	model := paraformer.ParaformerRealTimeV1
	token := os.Getenv("DASHSCOPE_API_KEY")
	if token == "" {
		panic("token is empty")
	}

	cli := dashscopego.NewTongyiClient(model, token)

	streamCallbackFn := func(_ context.Context, chunk []byte) error {
		log.Print(string(chunk))
		return nil
	}

	headerPara := paraformer.ReqHeader{
		Streaming: "duplex",
		TaskID:    paraformer.GenerateTaskID(),
		Action:    "run-task",
	}

	payload := paraformer.PayloadIn{
		Parameters: paraformer.Parameters{
			// seems like only support 16000 sample-rate.
			SampleRate: 16000,
			Format:     "pcm",
		},
		Input:     map[string]interface{}{},
		Task:      "asr",
		TaskGroup: "audio",
		Function:  "recognition",
	}

	req := &paraformer.Request{
		Header:      headerPara,
		Payload:     payload,
		StreamingFn: streamCallbackFn,
	}

	// 声音获取 实际使用时请替换成实时音频流.
	voiceReader := readAudioFromDesktop()

	reader := bufio.NewReader(voiceReader)

	if err := cli.CreateSpeechToTextGeneration(context.TODO(), req, reader); err != nil {
		panic(err)
	}

	// 等待语音识别结果输出
	time.Sleep(5 * time.Second)
}

// 读取音频文件中的录音 模拟实时语音流. 这里下载的官方文档中的示例音频文件.
// `https://dashscope.oss-cn-beijing.aliyuncs.com/samples/audio/paraformer/hello_world_male2.wav`.
func readAudioFromDesktop() *bufio.Reader {
	usr, err := user.Current()
	if err != nil {
		panic(err)
	}

	voiceFilePath := filepath.Join(usr.HomeDir, "Desktop", "hello_world_female2.wav")

	voice, err := os.OpenFile(voiceFilePath, os.O_RDONLY, 0640) //nolint:gofumpt
	if err != nil {
		panic(err)
	}

	reader := bufio.NewReader(voice)
	return reader
}
