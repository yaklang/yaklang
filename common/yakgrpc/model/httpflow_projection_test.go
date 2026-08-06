package model

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestHTTPFlowProjectionExcludesResponseWithoutMutatingCanonicalCache(t *testing.T) {
	DropHTTPFlowCacheGRPCModelByFlow()
	t.Cleanup(DropHTTPFlowCacheGRPCModelByFlow)

	flow := &schema.HTTPFlow{
		Url:           "http://example.test/path",
		Path:          "/path",
		Method:        "GET",
		Request:       strconv.Quote("GET /path HTTP/1.1\r\nHost: example.test\r\n\r\n"),
		Response:      strconv.Quote("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n<title>projection</title>body"),
		RequestLength: 49,
		BodyLength:    36,
		StatusCode:    200,
		ContentType:   "text/html",
	}
	flow.ID = 42
	flow.CreatedAt = time.Unix(1, 0)
	flow.UpdatedAt = time.Unix(2, 0)

	projected, err := ToHTTPFlowGRPCModelWithoutResponseRaw(flow, false)
	require.NoError(t, err)
	require.NotEmpty(t, projected.GetRequest())
	require.Empty(t, projected.GetResponse())
	require.Empty(t, projected.GetRawResponseBodyBase64())
	require.Equal(t, "projection", projected.GetHtmlTitle())
	require.Equal(t, int64(36), projected.GetBodyLength())
	require.Nil(t, getCacheHTTPFlowGRPCModel(flow, false), "projected list rows must not retain raw responses in cache")

	packetProjected, err := ToHTTPFlowGRPCModelWithListProjection(flow, true, true, true)
	require.NoError(t, err)
	require.Empty(t, packetProjected.GetRequest())
	require.Empty(t, packetProjected.GetResponse())
	require.Empty(t, packetProjected.GetRequestHeader())
	require.Empty(t, packetProjected.GetResponseHeader())
	require.Empty(t, packetProjected.GetRawRequestBodyBase64())
	require.Empty(t, packetProjected.GetRawResponseBodyBase64())
	require.Equal(t, int64(49), packetProjected.GetRequestLength())
	require.Equal(t, int64(36), packetProjected.GetBodyLength())
	require.Equal(t, "projection", packetProjected.GetHtmlTitle())
	require.Nil(t, getCacheHTTPFlowGRPCModel(flow, false), "packet projections must bypass the canonical cache")

	canonical, err := ToHTTPFlowGRPCModel(flow, false)
	require.NoError(t, err)
	require.NotEmpty(t, canonical.GetRequest(), "projection must not mutate the cached canonical request")
	require.NotEmpty(t, canonical.GetResponse(), "projection must not mutate the cached canonical model")
}
