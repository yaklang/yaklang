package dashscopego

import (
	"bufio"
	"context"
	"log"
	"strings"

	embedding "github.com/yaklang/yaklang/common/ai/tongyi/embedding"
	httpclient "github.com/yaklang/yaklang/common/ai/tongyi/httpclient"
	"github.com/yaklang/yaklang/common/ai/tongyi/paraformer"
	"github.com/yaklang/yaklang/common/ai/tongyi/qwen"
	"github.com/yaklang/yaklang/common/ai/tongyi/wanx"
)

type TongyiClient struct {
	Model       string
	token       string
	httpCli     httpclient.IHttpClient
	uploadCache qwen.UploadCacher
}

func NewTongyiClient(model string, token string) *TongyiClient {
	httpcli := httpclient.NewHTTPClient()
	return newTongyiCLientWithHTTPCli(model, token, httpcli)
}

func newTongyiCLientWithHTTPCli(model string, token string, httpcli httpclient.IHttpClient) *TongyiClient {
	return &TongyiClient{
		Model:       model,
		httpCli:     httpcli,
		token:       token,
		uploadCache: qwen.NewMemoryFileCache(),
	}
}

// setup upload cache for uploading files to prevent duplicate upload.
func (q *TongyiClient) SetUploadCache(uploadCache qwen.UploadCacher) *TongyiClient {
	q.uploadCache = uploadCache
	return q
}

// duplicate: CreateCompletion and CreateVLCompletion are the same but with different payload types.
// maybe this can be change in the future.
//
// nolint:lll
func (q *TongyiClient) CreateCompletion(ctx context.Context, payload *qwen.Request[*qwen.TextContent]) (*TextQwenResponse, error) {
	payload = payloadPreCheck(q, payload)
	return genericCompletion[*qwen.TextContent, *qwen.TextContent](ctx, payload, q.httpCli, qwen.URLQwen(), q.token)
}

//nolint:lll
func (q *TongyiClient) CreateVLCompletion(ctx context.Context, payload *qwen.Request[*qwen.VLContentList]) (*VLQwenResponse, error) {
	payload = payloadPreCheck(q, payload)

	for _, vMsg := range payload.Input.Messages {
		tmpImageContent, hasImg := vMsg.Content.PopImageContent()
		if hasImg && vMsg.Role == qwen.RoleUser {
			filepath := tmpImageContent.Image

			ossURL, hasUploadOss, err := checkIfNeedUploadFile(ctx, filepath, payload.Model, q.token, q.uploadCache)
			if err != nil {
				return nil, err
			}
			if hasUploadOss {
				payload.HasUploadOss = true
			}
			vMsg.Content.SetImage(ossURL)
		}
	}

	return genericCompletion[*qwen.VLContentList, *qwen.VLContentList](ctx, payload, q.httpCli, qwen.URLQwenVL(), q.token)
}

//nolint:lll
func (q *TongyiClient) CreateAudioCompletion(ctx context.Context, payload *qwen.Request[*qwen.AudioContentList]) (*AudioQwenResponse, error) {
	payload = payloadPreCheck(q, payload)
	for _, acMsg := range payload.Input.Messages {
		tmpAudioContent, hasAudio := acMsg.Content.PopAudioContent()

		if hasAudio && acMsg.Role == qwen.RoleUser {
			filepath := tmpAudioContent.Audio

			ossURL, hasUploadOss, err := checkIfNeedUploadFile(ctx, filepath, payload.Model, q.token, q.uploadCache)
			if err != nil {
				return nil, err
			}

			if hasUploadOss {
				payload.HasUploadOss = true
			}
			acMsg.Content.SetAudio(ossURL)
		}
	}

	return genericCompletion[*qwen.AudioContentList, *qwen.AudioContentList](ctx, payload, q.httpCli, qwen.URLQwenVL(), q.token)
}

// used for pdf_extracter plugin.
//
//nolint:lll
func (q *TongyiClient) CreateFileCompletion(ctx context.Context, payload *qwen.Request[*qwen.FileContentList]) (*FileQwenResponse, error) {
	payload = payloadPreCheck(q, payload)

	for _, vMsg := range payload.Input.Messages {
		tmpImageContent, hasImg := vMsg.Content.PopFileContent()
		if hasImg && vMsg.Role == qwen.RoleUser {
			filepath := tmpImageContent.File

			ossURL, hasUploadOss, err := checkIfNeedUploadFile(ctx, filepath, payload.Model, q.token, q.uploadCache)
			if err != nil {
				return nil, err
			}
			if hasUploadOss {
				payload.HasUploadOss = true
			}
			vMsg.Content.SetFile(ossURL)
		}
	}

	return genericCompletion[*qwen.FileContentList, *qwen.TextContent](ctx, payload, q.httpCli, qwen.URLQwen(), q.token)
}

func checkIfNeedUploadFile(ctx context.Context, filepath string, model, token string, uploadCacher qwen.UploadCacher) (string, bool, error) {
	var err error
	var ossURL string
	var hasUploadOss bool
	switch {
	case strings.Contains(filepath, "dashscope.oss"):
		// 使用了官方案例中的格式(https://dashscope.oss...).
		ossURL = filepath
	case strings.HasPrefix(filepath, "oss://"):
		// 已经在 oss 中的不必上传.
		ossURL = filepath
	case strings.HasPrefix(filepath, "file://"):
		// 本地文件.
		filepath = strings.TrimPrefix(filepath, "file://")
		ossURL, err = qwen.UploadLocalFile(ctx, filepath, model, token, uploadCacher)
		hasUploadOss = true
	case strings.HasPrefix(filepath, "https://") || strings.HasPrefix(filepath, "http://"):
		// 文件的 URL 链接.
		ossURL, err = qwen.UploadFileFromURL(ctx, filepath, model, token, uploadCacher)
		hasUploadOss = true
	}

	return ossURL, hasUploadOss, err
}

//nolint:lll
func genericCompletion[T qwen.IQwenContent, U qwen.IQwenContent](ctx context.Context, payload *qwen.Request[T], httpcli httpclient.IHttpClient, url, token string) (*qwen.OutputResponse[U], error) {
	if payload.Model == "" {
		return nil, ErrModelNotSet
	}

	// use streaming if streaming func is set
	if payload.StreamingFn != nil {
		payload.Parameters.SetIncrementalOutput(true)
		return qwen.SendMessageStream[T, U](ctx, payload, httpcli, url, token)
	}

	return qwen.SendMessage[T, U](ctx, payload, httpcli, url, token)
}

// TODO: intergrate wanx.Request into qwen.IQwenContent(or should rename to ITongyiContent)
//
//nolint:lll
func (q *TongyiClient) CreateImageGeneration(ctx context.Context, payload *wanx.ImageSynthesisRequest) ([]*wanx.ImgBlob, error) {
	if payload.Model == "" {
		if q.Model == "" {
			return nil, ErrModelNotSet
		}
		payload.Model = q.Model
	}
	return wanx.CreateImageGeneration(ctx, payload, q.httpCli, q.token)
}

// voice file to text.
func (q *TongyiClient) CreateVoiceFileToTextGeneration(ctx context.Context, request *paraformer.AsyncTaskRequest) (*paraformer.VoiceFileResponse, error) {
	if request.Model == "" {
		if q.Model == "" {
			return nil, ErrModelNotSet
		}
		request.Model = q.Model
	}

	var RequestURLs []string
	for _, fileURL := range request.Input.FileURLs {
		ossURL, hasUploadOss, err := checkIfNeedUploadFile(ctx, fileURL, request.Model, q.token, q.uploadCache)
		if err != nil {
			return nil, err
		}
		if hasUploadOss {
			// upload file to oss
			RequestURLs = append(RequestURLs, ossURL)
			request.HasUploadOss = true
		} else {
			RequestURLs = append(RequestURLs, fileURL)
		}
	}

	request.Input.FileURLs = RequestURLs

	return paraformer.VoiceFileToTextGeneration(ctx, request, q.httpCli, q.token)
}

// realtime sppech to text.
func (q *TongyiClient) CreateSpeechToTextGeneration(ctx context.Context, request *paraformer.Request, reader *bufio.Reader) error {
	if request.Payload.Model == "" {
		if q.Model == "" {
			return ErrModelNotSet
		}
		request.Payload.Model = q.Model
	}

	wsCli, err := paraformer.ConnRecognitionClient(request, q.token)
	if err != nil {
		return err
	}

	// handle response by stream callback
	go paraformer.HandleRecognitionResult(ctx, wsCli, request.StreamingFn)

	for {
		// this buf can not be reused,
		// otherwise the data will be overwritten, voice became disorder.
		buf := make([]byte, 1024)
		n, errRead := reader.Read(buf)
		if n == 0 {
			break
		}
		if errRead != nil {
			log.Printf("read line error: %v\n", errRead)
			err = errRead
			return err
		}

		paraformer.SendRadioData(wsCli, buf)
	}

	return nil
}

func (q *TongyiClient) CreateEmbedding(ctx context.Context, r *embedding.Request) ([][]float64, int, error) {
	resp, err := embedding.CreateEmbedding(ctx, r, q.httpCli, q.token)
	if err != nil {
		return nil, 0, err
	}

	totslTokens := resp.Usgae.TotalTokens
	if len(resp.Output.Embeddings) == 0 {
		return nil, 0, ErrEmptyResponse
	}

	embeddings := make([][]float64, 0)
	for i := 0; i < len(resp.Output.Embeddings); i++ {
		embeddings = append(embeddings, resp.Output.Embeddings[i].Embedding)
	}
	return embeddings, totslTokens, nil
}

func payloadPreCheck[T qwen.IQwenContent](q *TongyiClient, payload *qwen.Request[T]) *qwen.Request[T] {
	if payload.Model == "" {
		payload.Model = q.Model
	}

	if payload.Parameters == nil {
		payload.Parameters = qwen.DefaultParameters()
	}

	return payload
}
