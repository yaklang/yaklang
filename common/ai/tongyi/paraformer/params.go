package paraformer

import "fmt"

type ModelParaformer = string

const (
	// detect from file.
	ParaformerV1    ModelParaformer = "paraformer-v1"
	Paraformer8KV1  ModelParaformer = "paraformer-8k-v1"
	ParaformerMtlV1 ModelParaformer = "paraformer-mtl-v1"
	// real time voice.
	ParaformerRealTimeV1   ModelParaformer = "paraformer-realtime-v1"
	ParaformerRealTime8KV1 ModelParaformer = "paraformer-realtime-8k-v1"
)

const (
	// real-time voice recognition.
	ParaformerWSURL = "wss://dashscope.aliyuncs.com/api-ws/v1/inference"
	// audio file to text.
	ParaformerAsyncURL = "https://dashscope.aliyuncs.com/api/v1/services/audio/asr/transcription"
	// audio file to text  async-task-result query.
	ParaformerTaskURL = "https://dashscope.aliyuncs.com/api/v1/tasks/%s"
)

func TaskURL(taskID string) string {
	return fmt.Sprintf(ParaformerTaskURL, taskID)
}
