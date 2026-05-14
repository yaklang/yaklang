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

// TestGetQueryJsonParamName verifies the naming semantics for JSON-value GET/POST parameters:
//
//  1. Name() always returns the outer HTTP parameter name (e.g. the GET key "id"), consistent
//     with how plugins use it to report which HTTP parameter is vulnerable.
//
//  2. JsonFieldName() returns the JSON sub-field name (e.g. "uid", "id") for *Json positions,
//     falling back to the outer param name for non-JSON positions.
//
// Regression test for the missing param2nd field in PosGetQueryJson params (ef39e1b3e).
func TestGetQueryJsonParamName(t *testing.T) {
	type wantParam struct {
		position      string
		name          string // expected Name()  — always outer HTTP param name
		jsonFieldName string // expected JsonFieldName() — JSON sub-field name when applicable
		value         string // expected Value()
		path          string // expected Path()
	}

	tests := []struct {
		testName   string
		request    string
		wantParams []wantParam
	}{
		{
			// ?id={"uid":111,"id":"111"}
			// Name() = "id" (outer GET key) for all three params.
			// JsonFieldName() distinguishes the two JSON sub-fields: "uid" and "id".
			testName: "flat json value in GET query",
			request:  "GET /user?id=" + url.QueryEscape(`{"uid":111,"id":"111"}`) + " HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n",
			wantParams: []wantParam{
				{position: string(lowhttp.PosGetQueryJson), name: "id", jsonFieldName: "uid", value: "111", path: "$.uid"},
				{position: string(lowhttp.PosGetQueryJson), name: "id", jsonFieldName: "id", value: "111", path: "$.id"},
				{position: string(lowhttp.PosGetQuery), name: "id", jsonFieldName: "id", value: `{"uid":111,"id":"111"}`},
			},
		},
		{
			// ?data={"uid":222,"name":"alice"}
			// Name() = "data"; JsonFieldName() = "uid" / "name".
			testName: "nested json value in GET query",
			request:  "GET /test?data=" + url.QueryEscape(`{"uid":222,"name":"alice"}`) + " HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n",
			wantParams: []wantParam{
				{position: string(lowhttp.PosGetQueryJson), name: "data", jsonFieldName: "uid", value: "222", path: "$.uid"},
				{position: string(lowhttp.PosGetQueryJson), name: "data", jsonFieldName: "name", value: "alice", path: "$.name"},
				{position: string(lowhttp.PosGetQuery), name: "data", jsonFieldName: "data", value: `{"uid":222,"name":"alice"}`},
			},
		},
		{
			// Plain GET params: Name() == JsonFieldName() (no JSON expansion).
			testName: "plain GET param is unaffected",
			request:  "GET /?foo=bar&baz=qux HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n",
			wantParams: []wantParam{
				{position: string(lowhttp.PosGetQuery), name: "foo", jsonFieldName: "foo", value: "bar"},
				{position: string(lowhttp.PosGetQuery), name: "baz", jsonFieldName: "baz", value: "qux"},
			},
		},
		{
			// POST form: id={"uid":111,"id":"111"}
			// PosPostQueryJson: Name()="id", JsonFieldName()="uid"/"id".
			testName: "json value in POST form body",
			request: "POST /user HTTP/1.1\r\nHost: 127.0.0.1\r\n" +
				"Content-Type: application/x-www-form-urlencoded\r\n" +
				"Content-Length: " + func() string {
				body := "id=" + url.QueryEscape(`{"uid":111,"id":"111"}`)
				return utils.InterfaceToString(len(body))
			}() + "\r\n\r\n" +
				"id=" + url.QueryEscape(`{"uid":111,"id":"111"}`),
			wantParams: []wantParam{
				{position: string(lowhttp.PosPostQueryJson), name: "id", jsonFieldName: "uid", value: "111", path: "$.uid"},
				{position: string(lowhttp.PosPostQueryJson), name: "id", jsonFieldName: "id", value: "111", path: "$.id"},
				{position: string(lowhttp.PosPostQuery), name: "id", jsonFieldName: "id", value: `{"uid":111,"id":"111"}`},
			},
		},
		{
			// a={"abc":"123","a":123,"c":{"q":"123"}} — deep nesting.
			// Intermediate node "c" and leaf "c.q" both appear; Name() stays "a" throughout.
			testName: "deeply nested json value in GET query",
			request:  "GET /?a=" + url.QueryEscape(`{"abc":"123","a":123,"c":{"q":"123"}}`) + " HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n",
			wantParams: []wantParam{
				{position: string(lowhttp.PosGetQueryJson), name: "a", jsonFieldName: "abc", value: "123", path: "$.abc"},
				{position: string(lowhttp.PosGetQueryJson), name: "a", jsonFieldName: "a", value: "123", path: "$.a"},
				{position: string(lowhttp.PosGetQueryJson), name: "a", jsonFieldName: "c", value: `{"q":"123"}`, path: "$.c"},
				{position: string(lowhttp.PosGetQueryJson), name: "a", jsonFieldName: "q", value: "123", path: "$.c.q"},
				{position: string(lowhttp.PosGetQuery), name: "a", jsonFieldName: "a", value: `{"abc":"123","a":123,"c":{"q":"123"}}`},
			},
		},
		{
			// Mixed: plain "page" param + JSON-value "filter" param.
			testName: "mixed plain and json GET params",
			request:  "GET /?page=2&filter=" + url.QueryEscape(`{"status":"active","limit":10}`) + " HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n",
			wantParams: []wantParam{
				{position: string(lowhttp.PosGetQuery), name: "page", jsonFieldName: "page", value: "2"},
				{position: string(lowhttp.PosGetQueryJson), name: "filter", jsonFieldName: "status", value: "active", path: "$.status"},
				{position: string(lowhttp.PosGetQueryJson), name: "filter", jsonFieldName: "limit", value: "10", path: "$.limit"},
				{position: string(lowhttp.PosGetQuery), name: "filter", jsonFieldName: "filter", value: `{"status":"active","limit":10}`},
			},
		},
		{
			// POST body JSON: no outer key, Name() == JsonFieldName() == JSON field name directly.
			testName: "nested POST body JSON",
			request: func() string {
				body := `{"abc":"123","a":123,"c":{"q":"123"}}`
				return "POST / HTTP/1.1\r\nHost: 127.0.0.1\r\nContent-Type: application/json\r\n" +
					"Content-Length: " + utils.InterfaceToString(len(body)) + "\r\n\r\n" + body
			}(),
			wantParams: []wantParam{
				{position: string(lowhttp.PosPostJson), name: "abc", jsonFieldName: "abc", value: "123", path: "$.abc"},
				{position: string(lowhttp.PosPostJson), name: "a", jsonFieldName: "a", value: "123", path: "$.a"},
				{position: string(lowhttp.PosPostJson), name: "c", jsonFieldName: "c", value: `{"q":"123"}`, path: "$.c"},
				{position: string(lowhttp.PosPostJson), name: "q", jsonFieldName: "q", value: "123", path: "$.c.q"},
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
				require.Equal(t, w.jsonFieldName, p.JsonFieldName(),
					"[param %d] JsonFieldName() mismatch: want %q got %q", i, w.jsonFieldName, p.JsonFieldName())
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
