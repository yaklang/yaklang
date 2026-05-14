package mutate

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
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

// TestGetQueryJsonParamName verifies that when a GET parameter's value is a JSON object,
// Name() on each expanded sub-field param returns the JSON field name (e.g. "uid", "id"),
// NOT the outer GET parameter name (e.g. "data").
//
// Regression test for the missing param2nd field in PosGetQueryJson params.
func TestGetQueryJsonParamName(t *testing.T) {
	type wantParam struct {
		position string
		name     string // expected Name()
		value    string // expected Value()
		path     string // expected Path()
	}

	tests := []struct {
		testName   string
		request    string
		wantParams []wantParam
	}{
		{
			testName: "flat json value in GET query",
			request:  "GET /user?id=" + url.QueryEscape(`{"uid":111,"id":"111"}`) + " HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n",
			wantParams: []wantParam{
				{position: string(lowhttp.PosGetQueryJson), name: "uid", value: "111", path: "$.uid"},
				{position: string(lowhttp.PosGetQueryJson), name: "id", value: "111", path: "$.id"},
				{position: string(lowhttp.PosGetQuery), name: "id", value: `{"uid":111,"id":"111"}`},
			},
		},
		{
			testName: "nested json value in GET query",
			request:  "GET /test?data=" + url.QueryEscape(`{"uid":222,"name":"alice"}`) + " HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n",
			wantParams: []wantParam{
				{position: string(lowhttp.PosGetQueryJson), name: "uid", value: "222", path: "$.uid"},
				{position: string(lowhttp.PosGetQueryJson), name: "name", value: "alice", path: "$.name"},
				{position: string(lowhttp.PosGetQuery), name: "data", value: `{"uid":222,"name":"alice"}`},
			},
		},
		{
			testName: "plain GET param is unaffected",
			request:  "GET /?foo=bar&baz=qux HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n",
			wantParams: []wantParam{
				{position: string(lowhttp.PosGetQuery), name: "foo", value: "bar"},
				{position: string(lowhttp.PosGetQuery), name: "baz", value: "qux"},
			},
		},
		{
			// POST form body: id={"uid":111,"id":"111"}  (application/x-www-form-urlencoded)
			// Same two-level structure as the GET case; PosPostQueryJson must also return sub-field names.
			testName: "json value in POST form body",
			request: "POST /user HTTP/1.1\r\nHost: 127.0.0.1\r\n" +
				"Content-Type: application/x-www-form-urlencoded\r\n" +
				"Content-Length: " + func() string {
				body := "id=" + url.QueryEscape(`{"uid":111,"id":"111"}`)
				return utils.InterfaceToString(len(body))
			}() + "\r\n\r\n" +
				"id=" + url.QueryEscape(`{"uid":111,"id":"111"}`),
			wantParams: []wantParam{
				{position: string(lowhttp.PosPostQueryJson), name: "uid", value: "111", path: "$.uid"},
				{position: string(lowhttp.PosPostQueryJson), name: "id", value: "111", path: "$.id"},
				{position: string(lowhttp.PosPostQuery), name: "id", value: `{"uid":111,"id":"111"}`},
			},
		},
		{
			// Deep nesting: a={"abc":"123","a":123,"c":{"q":"123"}}
			// walk visits: abc(leaf), a(leaf), c(intermediate object), c.q(leaf)
			// The intermediate node "c" surfaces with its JSON string as value.
			testName: "deeply nested json value in GET query",
			request:  "GET /?a=" + url.QueryEscape(`{"abc":"123","a":123,"c":{"q":"123"}}`) + " HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n",
			wantParams: []wantParam{
				{position: string(lowhttp.PosGetQueryJson), name: "abc", value: "123", path: "$.abc"},
				{position: string(lowhttp.PosGetQueryJson), name: "a", value: "123", path: "$.a"},
				{position: string(lowhttp.PosGetQueryJson), name: "c", value: `{"q":"123"}`, path: "$.c"},
				{position: string(lowhttp.PosGetQueryJson), name: "q", value: "123", path: "$.c.q"},
				{position: string(lowhttp.PosGetQuery), name: "a", value: `{"abc":"123","a":123,"c":{"q":"123"}}`},
			},
		},
		{
			// Multiple GET params, one plain and one with a JSON value.
			// Plain param must not be affected; JSON param must expand correctly.
			testName: "mixed plain and json GET params",
			request:  "GET /?page=2&filter=" + url.QueryEscape(`{"status":"active","limit":10}`) + " HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n",
			wantParams: []wantParam{
				{position: string(lowhttp.PosGetQuery), name: "page", value: "2"},
				{position: string(lowhttp.PosGetQueryJson), name: "status", value: "active", path: "$.status"},
				{position: string(lowhttp.PosGetQueryJson), name: "limit", value: "10", path: "$.limit"},
				{position: string(lowhttp.PosGetQuery), name: "filter", value: `{"status":"active","limit":10}`},
			},
		},
		{
			// POST body JSON (no outer key): sub-field names come directly from JSON keys.
			// Nesting produces both intermediate and leaf nodes.
			testName: "nested POST body JSON",
			request: func() string {
				body := `{"abc":"123","a":123,"c":{"q":"123"}}`
				return "POST / HTTP/1.1\r\nHost: 127.0.0.1\r\nContent-Type: application/json\r\n" +
					"Content-Length: " + utils.InterfaceToString(len(body)) + "\r\n\r\n" + body
			}(),
			wantParams: []wantParam{
				{position: string(lowhttp.PosPostJson), name: "abc", value: "123", path: "$.abc"},
				{position: string(lowhttp.PosPostJson), name: "a", value: "123", path: "$.a"},
				{position: string(lowhttp.PosPostJson), name: "c", value: `{"q":"123"}`, path: "$.c"},
				{position: string(lowhttp.PosPostJson), name: "q", value: "123", path: "$.c.q"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			req, err := NewFuzzHTTPRequest(tt.request)
			require.NoError(t, err)

			params := req.GetCommonParams()
			require.Len(t, params, len(tt.wantParams),
				"param count mismatch: got %d, want %d", len(params), len(tt.wantParams))

			for i, p := range params {
				w := tt.wantParams[i]
				require.Equal(t, w.position, p.Position(),
					"[param %d] position mismatch", i)
				require.Equal(t, w.name, p.Name(),
					"[param %d] Name() mismatch: want %q got %q", i, w.name, p.Name())
				require.Equal(t, w.value, utils.InterfaceToString(p.Value()),
					"[param %d] Value() mismatch", i)
				if w.path != "" {
					require.Equal(t, w.path, p.Path(),
						"[param %d] Path() mismatch", i)
				}
			}
		})
	}
}
