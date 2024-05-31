package embedding

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/tongyi/httpclient"
)

//nolint:lll
func CreateEmbedding(ctx context.Context, req *Request, cli httpclient.IHttpClient, token string) (*Response, error) {
	if req.Model == "" {
		req.Model = TextEmbeddingV2
	}
	if req.Params.TextType == "" {
		req.Params.TextType = TypeDocument
	}

	resp := Response{}
	tokenOption := httpclient.WithTokenHeaderOption(token)
	headerOption := httpclient.WithHeader(httpclient.HeaderMap{"content-type": "application/json"})
	err := cli.Post(ctx, embeddingURL, req, &resp, tokenOption, headerOption)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}
