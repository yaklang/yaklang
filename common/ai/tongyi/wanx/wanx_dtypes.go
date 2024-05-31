package wanx

type ImageSynthesisParams struct {
	/*
	  The style of the output image, currently supports the following style values:
	  "<auto>" default,
	  "<3d cartoon>" 3D cartoon,
	  "<anime>" animation,
	  "<oil painting>" oil painting,
	  "<watercolor>" watercolor,
	  "<sketch>" sketch,
	  "<chinese painting>" Chinese painting,
	  "<flat illustration>" flat illustration,
	*/
	Style string `json:"style,omitempty"`
	/*
	  The resolution of the generated image,
	  currently only supports '1024*1024', '720*1280', '1280*720' three resolutions,
	  default is 1024*1024 pixels.
	*/
	Size string `json:"size,omitempty"`
	// The number of images generated, currently supports 1~4, default is 1.
	N int `json:"n,omitempty"`
	// seed.
	Seed int `json:"seed,omitempty"`
}

type TaskStatus string

const (
	TaskSucceeded TaskStatus = "SUCCEEDED"
	TaskFailed    TaskStatus = "FAILED"
	TaskCanceled  TaskStatus = "CANCELED"
	TaskPending   TaskStatus = "PENDING"
	TaskSuspended TaskStatus = "SUSPENDED"
	TaskRunning   TaskStatus = "RUNNING"
)

type ImageSynthesisInput struct {
	Prompt        string `json:"prompt"`
	NegativePromp string `json:"negative_promp,omitempty"`
}

type ImageSynthesisRequest struct {
	Model    string               `json:"model"`
	Input    ImageSynthesisInput  `json:"input"`
	Params   ImageSynthesisParams `json:"parameters"`
	Download bool                 `json:"-"`
}

type Output struct {
	TaskID     string `json:"task_id"`
	TaskStatus string `json:"task_status"`
	Results    []struct {
		URL string `json:"url"`
	} `json:"results"`
	TaskMetrics struct {
		Total     int `json:"TOTAL"`
		Succeeded int `json:"SUCCEEDED"`
		Failed    int `json:"FAILED"`
	} `json:"task_metrics"`
}

type Usage struct {
	ImageCount int `json:"image_count"`
}

type ImageResponse struct {
	StatusCode int    `json:"status_code"`
	RequestID  string `json:"request_id"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	Output     Output `json:"output"`
	Usage      Usage  `json:"usage"`
}

type ImgBlob struct {
	//	types include: "image/png".
	ImgType string `json:"img_type"`
	ImgURL  string `json:"img_url"`
	// Raw bytes for media formats.
	Data []byte `json:"-"`
}

type TaskRequest struct {
	TaskID string `json:"task_id"`
}

type TaskResponse struct {
	RequestID string `json:"request_id"`
	Output    Output `json:"output"`
	Usage     Usage  `json:"usage"`
}
