package mutate

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func TestFuzzQueryParams(t *testing.T) {
	type TestCase struct {
		testName   string
		request    string
		wantParams map[string]string
	}
	testCases := []TestCase{
		{
			testName: "get query params 1",
			request: `GET /?a=www.qp00.com!%#yx HTTP/1.1
Host: 127.0.0.1`,
			wantParams: map[string]string{
				"a": "www.qp00.com!%",
			},
		},
		{
			testName: "get query params 2",
			request: `GET /?a=%*&^(*&#@&*()@$%.66 HTTP/1.1
Host: 127.0.0.1`,
			wantParams: map[string]string{
				"a":   "%*",
				"^(*": "",
			},
		},
	}

	for _, testCase := range testCases {
		request, err := NewFuzzHTTPRequest(testCase.request)
		require.NoError(t, err)
		gotParams := request.GetCommonParams()
		require.Len(t, gotParams, len(testCase.wantParams), "[%s] got params length is not equal to %d", testCase.testName, len(testCase.wantParams))
		for _, param := range gotParams {
			key, value := utils.InterfaceToString(param.param), param.raw
			wantValue, ok := testCase.wantParams[key]
			require.True(t, ok, "[%s] got unexpected param %s", testCase.testName, key)
			require.Equal(t, wantValue, value, "[%s] got unexpected value %s for param %s", testCase.testName, value, key)
		}
	}
}
