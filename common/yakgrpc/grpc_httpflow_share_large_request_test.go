package yakgrpc

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestHTTPFlowsExtract_LargeRequestFlagsFromPacket(t *testing.T) {
	bodyLen := int64(531374322)
	reqPacket := "POST /upload HTTP/1.1\r\nHost: 127.0.0.1:8765\r\nContent-Length: " + strconv.FormatInt(bodyLen, 10) + "\r\n\r\n[[request too large(506.8MB), truncated]] use GetHTTPFlowBodyById(IsRequest=true) for full body"
	shareURL := "http://127.0.0.1:8765/upload-extract-quoted"
	shareJSON := `[{
		"method":"POST",
		"url":` + strconv.Quote(shareURL) + `,
		"path":"/upload",
		"status_code":200,
		"request":` + strconv.Quote(reqPacket) + `,
		"response":"HTTP/1.0 200 OK\r\n\r\nok",
		"request_length":` + strconv.FormatInt(bodyLen, 10) + `
	}]`

	client, err := NewLocalClientWithTempDatabase(t)
	require.NoError(t, err)
	grpcClient := client.(*Client)

	_, err = client.HTTPFlowsExtract(context.Background(), &ypb.HTTPFlowsExtractRequest{
		ShareExtractContent: shareJSON,
	})
	require.NoError(t, err)

	var flow schema.HTTPFlow
	require.NoError(t, grpcClient.GetProjectDatabase().Where("url = ?", shareURL).First(&flow).Error)
	require.True(t, flow.IsTooLargeRequest)
	require.Equal(t, bodyLen, flow.RequestLength)

	defer yakit.DeleteHTTPFlowByID(grpcClient.GetProjectDatabase(), int64(flow.ID))
}

func TestHTTPFlowsExtract_LargeRequestFlagsFromPlainRequestPacket(t *testing.T) {
	bodyLen := int64(531374322)
	reqPacket := "POST /upload HTTP/1.1\r\nHost: 127.0.0.1:8765\r\nContent-Length: " + strconv.FormatInt(bodyLen, 10) + "\r\n\r\n[[request-too-large(506.8M), truncated]] use GetHTTPFlowBodyById(IsRequest=true) for full body"
	shareURL := "http://127.0.0.1:8765/upload-extract-plain"
	shareJSON := `[{
		"method":"POST",
		"url":` + strconv.Quote(shareURL) + `,
		"path":"/upload",
		"status_code":200,
		"request":` + strconv.Quote(reqPacket) + `,
		"response":"HTTP/1.0 200 OK\r\n\r\nok",
		"request_length":` + strconv.FormatInt(bodyLen, 10) + `
	}]`

	client, err := NewLocalClientWithTempDatabase(t)
	require.NoError(t, err)
	grpcClient := client.(*Client)

	_, err = client.HTTPFlowsExtract(context.Background(), &ypb.HTTPFlowsExtractRequest{
		ShareExtractContent: shareJSON,
	})
	require.NoError(t, err)

	var flow schema.HTTPFlow
	require.NoError(t, grpcClient.GetProjectDatabase().Where("url = ?", shareURL).First(&flow).Error)
	require.True(t, flow.IsTooLargeRequest)
	require.Equal(t, bodyLen, flow.RequestLength)

	defer yakit.DeleteHTTPFlowByID(grpcClient.GetProjectDatabase(), int64(flow.ID))
}

func TestHTTPFlowShareJSON_UnmarshalLargeRequest(t *testing.T) {
	bodyLen := int64(300 * 1024)
	reqPacket := "POST /upload HTTP/1.1\r\nHost: example.com\r\nContent-Length: 999\r\n\r\n[[request too large(300KB), truncated]]"
	raw := `[{"method":"POST","request":` + strconv.Quote(reqPacket) + `,"request_length":` + strconv.FormatInt(bodyLen, 10) + `,"unique_index":"share-unmarshal-hash"}]`
	var shares []*HTTPFlowShare
	require.NoError(t, json.Unmarshal([]byte(raw), &shares))
	require.Len(t, shares, 1)
	require.NotNil(t, shares[0].HTTPFlow)
	require.Equal(t, "share-unmarshal-hash", shares[0].Hash)
	require.Equal(t, reqPacket, shares[0].GetRequest())
	require.Equal(t, bodyLen, shares[0].RequestLength)
}
