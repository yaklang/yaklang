package paraformer

import (
	"context"
	"log"
	"time"

	httpclient "github.com/yaklang/yaklang/common/ai/tongyi/httpclient"
)

//

func AsyncVoiceFileRecognitionTask(ctx context.Context, request *AsyncTaskRequest, cli httpclient.IHttpClient, token string) (*AsyncTaskResponse, error) {
	tokenHeader := httpclient.WithTokenHeaderOption(token)
	header := httpclient.HeaderMap{
		"X-DashScope-Async": "enable",
		"Content-Type":      "application/json",
	}

	if request.HasUploadOss {
		header["X-DashScope-OssResourceResolve"] = "enable"
	}

	contentHeader := httpclient.WithHeader(header)
	resp := AsyncTaskResponse{}
	err := cli.Post(ctx, ParaformerAsyncURL, request, &resp, tokenHeader, contentHeader)
	if err != nil {
		return &resp, err
	}

	return &resp, nil
}

type VoiceFileResponse struct {
	AsyncTaskResp *AsyncTaskResponse
	FileResults   []*FileResult
}

func VoiceFileToTextGeneration(ctx context.Context, req *AsyncTaskRequest, cli httpclient.IHttpClient, token string) (*VoiceFileResponse, error) {
	resp := &VoiceFileResponse{}

	var resultList []*FileResult

	tokenHeader := httpclient.WithTokenHeaderOption(token)
	contentHeader := httpclient.WithHeader(httpclient.HeaderMap{
		"Accept": "application/json",
	})

	taskResp, err := AsyncVoiceFileRecognitionTask(ctx, req, cli, token)
	if err != nil {
		return nil, err
	}
	resp.AsyncTaskResp = taskResp

	if req.Download {
		taskReq := TaskResultRequest{TaskID: taskResp.Output.TaskID}

		taskStatusReap := &AsyncTaskResponse{}
		firstQuery := true
		for firstQuery ||
			taskStatusReap.Output.TaskStatus == "PENDING" ||
			taskStatusReap.Output.TaskStatus == "RUNNING" {
			firstQuery = false
			log.Println("TaskStatus: ", taskStatusReap.Output.TaskStatus)
			taskStatusReap, err = CheckTaskStatus(ctx, &taskReq, cli, tokenHeader, contentHeader)
			if err != nil {
				return nil, err
			}

			time.Sleep(1 * time.Second)
		}

		// use taskID download file and read json content.
		for _, resultInfo := range taskStatusReap.Output.Results {
			result, err := downloadJsonfile(ctx, resultInfo.TranscriptionURL, cli)
			if err != nil {
				return nil, err
			}

			resultList = append(resultList, result)
			resp.FileResults = resultList
		}
	}

	return resp, nil
}

//nolint:lll
func CheckTaskStatus(ctx context.Context, req *TaskResultRequest, httpcli httpclient.IHttpClient, options ...httpclient.HTTPOption) (*AsyncTaskResponse, error) {
	resp := AsyncTaskResponse{}
	err := httpcli.Get(ctx, TaskURL(req.TaskID), nil, &resp, options...)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

func downloadJsonfile(ctx context.Context, url string, httpcli httpclient.IHttpClient) (*FileResult, error) {
	contentHeader := httpclient.WithHeader(httpclient.HeaderMap{
		"Accept": "application/json",
	})

	resp := FileResult{}
	err := httpcli.Get(ctx, url, nil, &resp, contentHeader)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}
