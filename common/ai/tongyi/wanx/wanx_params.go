package wanx

import "fmt"

const (
	DashScopeBaseURL  = "https://dashscope.aliyuncs.com"
	ImageSynthesisURI = "/api/v1/services/aigc/text2image/image-synthesis"
	TaskURI           = "/api/v1/tasks/%s"
)

type ModelWanx = string

const (
	WanxV1             ModelWanx = "wanx-v1"
	WanxStyleRepaintV1 ModelWanx = "wanx-style-repaint-v1"
	WanxBgGenV2        ModelWanx = "wanx-background-generation-v2"
)

func ImageSynthesisURL() string {
	return DashScopeBaseURL + ImageSynthesisURI
}

func TaskURL(taskID string) string {
	return DashScopeBaseURL + fmt.Sprintf(TaskURI, taskID)
}
