package yakit

import (
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestCreateHTTPFlowParsedRequestParamCountsMatchRawPath(t *testing.T) {
	body := []byte(`{"top":{"nested":"value"},"items":[1,2,3],"plain":"text"}`)
	raw := []byte(fmt.Sprintf(
		"POST /path?plain=1&json=%%7B%%22a%%22%%3A1%%7D HTTP/1.1\r\n"+
			"Host: example.test\r\n"+
			"Content-Type: application/json\r\n"+
			"Cookie: session=value; flags=%%7B%%22debug%%22%%3Atrue%%7D\r\n"+
			"Content-Length: %d\r\n\r\n%s",
		len(body), body,
	))
	req, err := lowhttp.ParseBytesToHttpRequest(raw)
	require.NoError(t, err)

	rawFlow, err := CreateHTTPFlow(CreateHTTPFlowWithRequestRaw(raw))
	require.NoError(t, err)
	parsedFlow, err := CreateHTTPFlow(
		CreateHTTPFlowWithRequestRaw(raw),
		CreateHTTPFlowWithRequestIns(req),
	)
	require.NoError(t, err)

	require.Equal(t, rawFlow.GetParamsTotal, parsedFlow.GetParamsTotal)
	require.Equal(t, rawFlow.PostParamsTotal, parsedFlow.PostParamsTotal)
	require.Equal(t, rawFlow.CookieParamsTotal, parsedFlow.CookieParamsTotal)
	require.Positive(t, parsedFlow.GetParamsTotal)
	require.Positive(t, parsedFlow.PostParamsTotal)
	require.Positive(t, parsedFlow.CookieParamsTotal)

	bodyAfterCount, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.Equal(t, body, bodyAfterCount)
}
