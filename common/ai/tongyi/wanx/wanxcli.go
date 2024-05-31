package wanx

import (
	"context"
	"errors"
	"log"
	"time"

	httpclient "github.com/yaklang/yaklang/common/ai/tongyi/httpclient"
)

var (
	ErrEmptyResponse = errors.New("empty response")
	ErrEmptyTaskID   = errors.New("wanx task id is empty")
	ErrTaskUnsuccess = errors.New("wanx task is not success")
	ErrModelNotSet   = errors.New("model is not set")
)

//nolint:lll
func CreateImageGeneration(ctx context.Context, payload *ImageSynthesisRequest, httpcli httpclient.IHttpClient, token string) ([]*ImgBlob, error) {
	header := map[string]string{
		"Content-Type": "application/json",
	}

	tokenOpt := httpclient.WithTokenHeaderOption(token)
	headerOpt := httpclient.WithHeader(header)
	resp, err := SyncCall(ctx, payload, httpcli, tokenOpt, headerOpt)
	if err != nil {
		return nil, err
	}

	blobList := make([]*ImgBlob, 0, len(resp.Results))
	for _, img := range resp.Results {
		tmpImgBlob := &ImgBlob{
			ImgType: "image/png",
			ImgURL:  img.URL,
		}

		if payload.Download {
			imgByte, err := httpcli.GetImage(ctx, img.URL, tokenOpt)
			if err != nil {
				return nil, err
			}

			tmpImgBlob.Data = imgByte
		}

		blobList = append(blobList, tmpImgBlob)
	}

	return blobList, nil
}

// tongyi-wanx-api only support AsyncCall, so we need to warp it to be Sync.
//
//nolint:lll
func SyncCall(ctx context.Context, req *ImageSynthesisRequest, httpcli httpclient.IHttpClient, options ...httpclient.HTTPOption) (*Output, error) {
	rsp, err := AsyncCall(ctx, req, httpcli, options...)
	if err != nil {
		return nil, err
	}

	currentTaskStatus := TaskStatus(rsp.Output.TaskStatus)

	taskID := rsp.Output.TaskID
	if taskID == "" {
		return nil, ErrEmptyTaskID
	}

	taskReq := TaskRequest{TaskID: taskID}
	taskResp := &TaskResponse{}

	for currentTaskStatus == TaskPending ||
		currentTaskStatus == TaskRunning ||
		currentTaskStatus == TaskSuspended {
		delayDurationToCheckStatus := 800
		log.Println("TaskStatus: ", currentTaskStatus)
		time.Sleep(time.Duration(delayDurationToCheckStatus) * time.Millisecond)

		// log.Println("TaskStatus: ", currentTaskStatus)
		taskResp, err = CheckTaskStatus(ctx, &taskReq, httpcli, options...)
		if err != nil {
			return nil, err
		}
		currentTaskStatus = TaskStatus(taskResp.Output.TaskStatus)
	}

	if currentTaskStatus == TaskFailed ||
		currentTaskStatus == TaskCanceled {
		return nil, ErrTaskUnsuccess
	}

	if len(taskResp.Output.Results) == 0 {
		return nil, ErrEmptyResponse
	}

	return &taskResp.Output, nil
}

// calling tongyi-wanx-api to get image-generation async task id.
//
//nolint:lll
func AsyncCall(ctx context.Context, req *ImageSynthesisRequest, httpcli httpclient.IHttpClient, options ...httpclient.HTTPOption) (*ImageResponse, error) {
	header := map[string]string{"X-DashScope-Async": "enable"}
	headerOpt := httpclient.WithHeader(header)
	options = append(options, headerOpt)

	if req.Model == "" {
		return nil, ErrModelNotSet
	}

	resp := ImageResponse{}
	err := httpcli.Post(ctx, ImageSynthesisURL(), req, &resp, options...)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

//nolint:lll
func CheckTaskStatus(ctx context.Context, req *TaskRequest, httpcli httpclient.IHttpClient, options ...httpclient.HTTPOption) (*TaskResponse, error) {
	resp := TaskResponse{}
	err := httpcli.Get(ctx, TaskURL(req.TaskID), nil, &resp, options...)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}
