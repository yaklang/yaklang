package paraformer

import "context"

type Parameters struct {
	SampleRate int    `json:"sample_rate"`
	Format     string `json:"format"`
}

type StreamingFunc func(ctx context.Context, chunk []byte) error

type Request struct {
	Header      ReqHeader     `json:"header"`
	Payload     PayloadIn     `json:"payload"`
	StreamingFn StreamingFunc `json:"-"`
}

type ReqHeader struct {
	Streaming string `json:"streaming"`
	TaskID    string `json:"task_id"`
	Action    string `json:"action"`
}

type PayloadIn struct {
	Model      string                 `json:"model"`
	Parameters Parameters             `json:"parameters"`
	Input      map[string]interface{} `json:"input"`
	Task       string                 `json:"task"`
	TaskGroup  string                 `json:"task_group"`
	Function   string                 `json:"function"`
}

// ---------
// type Word struct {
// 	BeginTime   int    `json:"begin_time"`
// 	EndTime     int    `json:"end_time"`
// 	Text        string `json:"text"`
// 	Punctuation string `json:"punctuation"`
// }

type Output struct {
	Sentence Sentence `json:"sentence"`
}

type Usage struct {
	Duration int `json:"duration"`
}

type PayloadOut struct {
	Output Output `json:"output"`
	Usage  Usage  `json:"usage"`
}

type Attributes struct{}

type Header struct {
	TaskID     string     `json:"task_id"`
	Event      string     `json:"event"`
	Attributes Attributes `json:"attributes"`
}

type RecognitionResult struct {
	Header  Header     `json:"header"`
	Payload PayloadOut `json:"payload"`
}

// ===========
// 生成异步 task_id.
type AsyncTaskRequest struct {
	Model        string     `json:"model"`
	Input        AsyncInput `json:"input"`
	HasUploadOss bool       `json:"-"`
	Download     bool       `json:"-"`
}

type AsyncInput struct {
	FileURLs []string `json:"file_urls"`
}

type AsyncTaskResponse struct {
	RequestID string             `json:"request_id"`
	Output    TaskResultResponse `json:"output"`
}

// 根据 task_id 获取结果.
type TaskResultRequest struct {
	TaskID string `json:"task_id"`
}

type TaskResultResponse struct {
	TaskID        string      `json:"task_id,omitempty"`
	TaskStatus    string      `json:"task_status,omitempty"`
	SubmitTime    string      `json:"submit_time,omitempty"`
	ScheduledTime string      `json:"scheduled_time,omitempty"`
	EndTime       string      `json:"end_time,omitempty"`
	Results       []Result    `json:"results,omitempty"`
	TaskMetrics   TaskMetrics `json:"task_metrics,omitempty"`
}

type Result struct {
	FileURL          string `json:"file_url,omitempty"`
	TranscriptionURL string `json:"transcription_url,omitempty"`
	SubtaskStatus    string `json:"subtask_status,omitempty"`
}

type TaskMetrics struct {
	Total     int `json:"TOTAL,omitempty"`
	Succeeded int `json:"SUCCEEDED,omitempty"`
	Failed    int `json:"FAILED,omitempty"`
}

// =========== 最终结果 ===========.
type FileResult struct {
	FileURL     string       `json:"file_url"`
	Properties  Properties   `json:"properties"`
	Transcripts []Transcript `json:"transcripts"`
}

type Properties struct {
	Channels                       []interface{} `json:"channels"`
	OriginalSamplingRate           int           `json:"original_sampling_rate"`
	OriginalDurationInMilliseconds int           `json:"original_duration_in_milliseconds"`
}

type Transcript struct {
	ChannelID                     int        `json:"channel_id"`
	ContentDurationInMilliseconds int        `json:"content_duration_in_milliseconds"`
	Text                          string     `json:"text"`
	Sentences                     []Sentence `json:"sentences"`
}

type Sentence struct {
	BeginTime  int    `json:"begin_time"`
	EndTime    int    `json:"end_time"`
	SentenceID int    `json:"sentence_id"`
	Text       string `json:"text"`
	Words      []Word `json:"words"`
}

type Word struct {
	BeginTime   int    `json:"begin_time"`
	EndTime     int    `json:"end_time"`
	Text        string `json:"text"`
	Punctuation string `json:"punctuation"`
}
