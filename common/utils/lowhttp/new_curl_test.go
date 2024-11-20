package lowhttp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func testCurlToRawHTTPRequest(t *testing.T, cmd string, includes []string, excludes []string) {
	t.Helper()

	req, err := CurlToRawHTTPRequest(cmd)
	require.NoError(t, err)

	println(string(req))

	for _, include := range includes {
		require.Contains(t, string(req), include)
	}
	for _, exclude := range excludes {
		require.NotContains(t, string(req), exclude)
	}
}

func TestCurlToRawHTTPRequestURL(t *testing.T) {
	t.Parallel()

	t.Run("url first", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl http://example.com -X POST -H "Content-Type: application/json" -d '{"a":1}'`, []string{`Host: example.com`}, []string{`{"a":1}=`})
	})
	t.Run("url in middle", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl -X POST http://example.com -H "Content-Type: application/json" -d '{"a":1}'`, []string{`Host: example.com`}, []string{`{"a":1}=`})
	})
	t.Run("url in middle", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl -X POST -H "Content-Type: application/json" -d '{"a":1}' http://example.com`, []string{`Host: example.com`}, []string{`{"a":1}=`})
	})
	t.Run("--url", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --url http://example.com -d "https://www.baidu.com"`, []string{`Host: example.com`}, nil)
	})

	t.Run("domain", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl example.com -d "https://www.baidu.com"`, []string{`Host: example.com`, "\r\nhttps://www.baidu.com"}, nil)
	})

	t.Run("ipv4", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl 8.8.8.8 -d "https://www.baidu.com"`, []string{`Host: 8.8.8.8`, "\r\nhttps://www.baidu.com"}, nil)
	})

	t.Run("ipv6", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl [::1] -d "https://www.baidu.com"`, []string{`Host: [::1]`, "\r\nhttps://www.baidu.com"}, nil)
	})

	t.Run("ws", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl ws://example.com/a/b/c`, []string{`Host: example.com`, `GET /a/b/c HTTP/1.1`}, nil)
	})
	t.Run("wss", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl wss://example.com/a/b/c`, []string{`Host: example.com`, `GET /a/b/c HTTP/1.1`}, nil)
	})
}

func TestCurlToRawHTTPRequestProtocol(t *testing.T) {
	t.Parallel()

	t.Run("http1.0", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --http1.0 http://example.com`, []string{`HTTP/1.0`}, nil)
	})
	t.Run("http1.1", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl http://example.com`, []string{`HTTP/1.1`}, nil)
	})
	t.Run("http2", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --http2 http://example.com`, []string{`HTTP/2`}, nil)
	})

	t.Run("http3", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --http3 http://example.com`, []string{`HTTP/3`}, nil)
	})
}

func TestCurlToRawHTTPRequestMethod(t *testing.T) {
	t.Parallel()

	t.Run("--head", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --head http://example.com`, []string{`HEAD / HTTP/1.`}, nil)
	})

	t.Run("--request", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --request POST http://example.com`, []string{`POST / HTTP/1.`}, nil)
	})

	t.Run("mix", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --request POST --head http://example.com`, []string{`HEAD / HTTP/1.`}, nil)
	})
}

func TestCurlToRawHTTPRequestHeader(t *testing.T) {
	t.Parallel()

	t.Run("--header", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --header "Content-Type: application/json" http://example.com`, []string{`Content-Type: application/json`}, nil)
	})
	t.Run("--user", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --user user:pass http://example.com`, []string{`Authorization: Basic dXNlcjpwYXNz`}, nil)
	})
	t.Run("--referer", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --referer "http://example.com" http://example.com`, []string{`Referer: http://example.com`}, nil)
	})
	t.Run("--user-agent", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --user-agent "a" http://example.com`, []string{`User-Agent: a`}, nil)
	})

	t.Run("--compressed", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --compressed http://example.com`, []string{`Accept-Encoding: gzip`}, nil)
	})

	t.Run("--range", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --range 1-2 http://example.com`, []string{`Range: bytes=1-2`}, nil)
	})

	t.Run("chunked", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --data "a=b" --header "Transfer-Encoding: chunked" http://example.com`, []string{`Transfer-Encoding: chunked`, "0\r\n\r\n"}, nil)
	})
	t.Run("cookie", func(t *testing.T) {
		t.Run("only cookie", func(t *testing.T) {
			testCurlToRawHTTPRequest(t, `curl --cookie "a=b" http://example.com`, []string{`Cookie: a=b`}, nil)
		})
		t.Run("Cookie Header", func(t *testing.T) {
			testCurlToRawHTTPRequest(t, `curl -H "Cookie: a=b" http://example.com`, []string{`Cookie: a=b`}, nil)
		})

		t.Run("mix", func(t *testing.T) {
			testCurlToRawHTTPRequest(t, `curl --cookie "a=b" -H "Cookie: c=d" http://example.com`, []string{`Cookie: a=b; c=d`}, nil)
		})
	})
}

func TestCurlToRawHTTPRequestData(t *testing.T) {
	t.Parallel()

	t.Run("data", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --data "a=b" http://example.com`, []string{`a=b`, `POST / HTTP/1.1`, `application/x-www-form-urlencoded`}, nil)
	})
	t.Run("data multiple", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --data "a=b" --data "c=d" http://example.com`, []string{`a=b&c=d`, `POST / HTTP/1.1`, `application/x-www-form-urlencoded`}, nil)
	})

	t.Run("data with raw", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --data '{"a":"b"}' -H "Content-Type: application/json" http://example.com`, []string{`{"a":"b"}`, `POST / HTTP/1.1`, `application/json`}, []string{`{"a":1}=`})
	})

	t.Run("data binary", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --data-binary @file http://example.com`, []string{`{{file(file)}}`, `POST / HTTP/1.1`, `application/x-www-form-urlencoded`}, nil)
	})

	t.Run("data without urlencode", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --data "#" http://example.com`, []string{"\r\n#", `POST / HTTP/1.1`, `application/x-www-form-urlencoded`}, nil)
	})

	t.Run("data urlencode", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --data-urlencode "#" http://example.com`, []string{"\r\n%23", `POST / HTTP/1.1`, `application/x-www-form-urlencoded`}, nil)
	})

	t.Run("upload file", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --upload-file a.txt http://example.com`, []string{`{{file(a.txt)}}`, `PUT /a.txt HTTP/1.1`}, nil)
	})

	t.Run("upload file mix", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --upload-file a.txt --data "a=b" http://example.com`, []string{`{{file(a.txt)}}`, `PUT /a.txt HTTP/1.1`, `Expect: 100-continue`}, nil)
	})

	t.Run("Upload file with path", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --upload-file a.txt http://example.com/a/b`, []string{`{{file(a.txt)}}`, `PUT /a/b HTTP/1.1`}, nil)
	})
}

func TestCurlToRawHTTPRequestForm(t *testing.T) {
	t.Parallel()

	t.Run("form", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --form "a=b" http://example.com`, []string{`Content-Disposition: form-data; name="a"`, "b\r\n--", `POST / HTTP/1.1`, `multipart/form-data; boundary=`}, nil)
	})

	t.Run("form binary", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --form "a=@a.txt" http://example.com`, []string{`Content-Disposition: form-data; name="a"; filename="a.txt"`, `{{file(a.txt)}}`, `POST / HTTP/1.1`, `multipart/form-data; boundary=`}, nil)
	})

	t.Run("form string", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --form-string "a=b" http://example.com`, []string{`Content-Disposition: form-data; name="a"`, "b\r\n--", `POST / HTTP/1.1`, `multipart/form-data; boundary=`}, nil)
	})

	t.Run("form multiple", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl --form "a=b" --form "c=d" http://example.com`, []string{`Content-Disposition: form-data; name="a"`, "b\r\n--", `Content-Disposition: form-data; name="c"`, "d\r\n--", `POST / HTTP/1.1`, `multipart/form-data; boundary=`}, nil)
	})
}

func TestCurlToRawHTTPRequestPutDataInQuery(t *testing.T) {
	t.Parallel()

	testCurlToRawHTTPRequest(t, `curl -G -d a=b -d c=d http://example.com`, []string{`GET /?a=b&c=d HTTP/1.1`}, nil)
}

func TestCurlToRawHTTPRequestNegative(t *testing.T) {
	t.Parallel()

	t.Run("invalid closure", func(t *testing.T) {
		_, err := CurlToRawHTTPRequest(`curl -X POST http://example.com -d '{"a":1`)
		require.Error(t, err)
	})

	t.Run("invalid request method", func(t *testing.T) {
		_, err := CurlToRawHTTPRequest(`curl http://example.com -X`)
		require.Error(t, err)
	})

	t.Run("invalid user", func(t *testing.T) {
		_, err := CurlToRawHTTPRequest(`curl http://example.com --user`)
		require.Error(t, err)
	})

	t.Run("invalid cookie", func(t *testing.T) {
		_, err := CurlToRawHTTPRequest(`curl http://example.com --cookie`)
		require.Error(t, err)
	})

	t.Run("invalid form", func(t *testing.T) {
		_, err := CurlToRawHTTPRequest(`curl http://example.com --form`)
		require.Error(t, err)
	})

	t.Run("invalid header", func(t *testing.T) {
		_, err := CurlToRawHTTPRequest(`curl http://example.com --header`)
		require.Error(t, err)
	})
	t.Run("invalid referer", func(t *testing.T) {
		_, err := CurlToRawHTTPRequest(`curl http://example.com --referer`)
		require.Error(t, err)
	})
	t.Run("invalid user-agent", func(t *testing.T) {
		_, err := CurlToRawHTTPRequest(`curl http://example.com --user-agent`)
		require.Error(t, err)
	})

	t.Run("invalid range", func(t *testing.T) {
		_, err := CurlToRawHTTPRequest(`curl http://example.com --range`)
		require.Error(t, err)
	})

	t.Run("invalid data", func(t *testing.T) {
		_, err := CurlToRawHTTPRequest(`curl http://example.com --data`)
		require.Error(t, err)
	})

	t.Run("invalid upload-file", func(t *testing.T) {
		_, err := CurlToRawHTTPRequest(`curl http://example.com --upload-file`)
		require.Error(t, err)
	})

	t.Run("invalid url", func(t *testing.T) {
		_, err := CurlToRawHTTPRequest(`curl --url`)
		require.Error(t, err)
	})

	t.Run("missing url", func(t *testing.T) {
		_, err := CurlToRawHTTPRequest(`curl`)
		require.Error(t, err)
	})
}

func TestCurlToRawHTTPRequestMisc(t *testing.T) {
	t.Parallel()

	t.Run("args split", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, "curl \\\n -H 'Referer: https://example.com' \\\n https://example.com", []string{`GET / HTTP/1.1`, `Referer: https://example.com`}, nil)
	})
}

func TestCurlToRawHTTPRequestOldTest(t *testing.T) {
	t.Parallel()

	t.Run("1", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl -X POST http://baidu.com -H "Content-Type: application/json" -d '{"a":1}'`, []string{`POST / HTTP/1.`, "application/json"}, nil)
	})

	t.Run("2", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl -X POST http://baidu.com -H "Content-Type: application/json" -d '{"a":1}' -H "User-Agent: abasdfasdfasdfasf" `, []string{`POST / HTTP/1.`, "application/json", "abasdfasdfasdfasf"}, nil)
	})
	t.Run("3", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl -X POST -H "Content-Type: application/json" https://baidu.com/abcaaa -d '{"a":1}' -H "User-Agent: abasdfasdfasdfasf" `, []string{`POST`, `HTTP/1.`, "tion/json", "dfasdfasf", "/abcaaa"}, nil)
	})

	t.Run("4", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl -X POST https://baidu.com/abcaaa -H "User-Agent: abasdfasdfasdfasf" -F "filename=@/tmp/file.txt"`, []string{`POST`, `HTTP/1.`, "dfasdfasf", "abcaaa", "/tmp/file.txt)}}", "boundary", "multipart"}, nil)
	})

	t.Run("cookie1", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl -X POST https://baidu.com/abcaaa -b abc=1 -H "User-Agent: abasdfasdfasdfasf" -F "filename=@/tmp/file.txt"`, []string{`POST`, `HTTP/1.`, `Cookie`, `abc=1`, "dfasdfasf", "abcaaa", "/tmp/file.txt)}}", "boundary", "multipart"}, nil)
	})

	t.Run("cookie2", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl -X POST https://baidu.com/abcaaa -b abc=1 -b ccc=1 -H "User-Agent: abasdfasdfasdfasf" -F "filename=@/tmp/file.txt"`, []string{`POST`, `HTTP/1.`, `Cookie`, `abc=1`, `ccc=1`, "dfasdfasf", "abcaaa", "/tmp/file.txt)}}", "boundary", "multipart"}, nil)
	})

	t.Run("basic-auth", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl -X POST https://baidu.com/abcaaa -b abc=1 -b ccc=1 -u admin:password -H "User-Agent: abasdfasdfasdfasf" -F "filename=@/tmp/file.txt"`, []string{`POST`, `Authorization`, "Basic", `YWRtaW46cGFzc3dvcmQ=`, `HTTP/1.`, `Cookie`, `abc=`, `ccc=1`, "dfasdfasf", "/abcaaa", "{{file(/tmp/file.txt)}}", "boundary", "multipart"}, nil)
	})

	t.Run("Head", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl -X POST https://baidu.com/abcaaa -H "User-Agent: abasdfasdfasdfasf" -F "filename=@/tmp/file.txt" -I`, []string{`HEAD`, `HTTP/1.`, "dfasdfasf", "abcaaa", "/tmp/file.txt)}}", "boundary", "multipart"}, nil)
	})

	t.Run("data raw", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl 'https://api.github.com/_private/browser/stats' \
-b 'b=xxxxxx;xxx' \
-H 'cookie: _octo=222; xxx=333;a=222' \
-H 'user-agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36' \
--data-raw 'abcd' \
--compressed`, []string{`POST`, `HTTP/1.`, "xxxxx", "_octo=222", "\r\nabcd"}, nil)
	})

	t.Run("AOrAgent", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl 'https://8.8.8.8/api/graphql/QueryRiskList' \
-H 'user-agent: -H setting' \
-H 'authority: audio-consideration-rc2ldz.cn.goofy.app' \
-A '-A setting' \
--data-raw $'{"query":"query QueryRiskList($req: QueryRiskListReqInput\u0021) {\\n  QueryRiskList(req: $req) {\\n    Data\\n    TotalCount\\n  }\\n}\\n","variables":{"req":{"Filters":[{"FieldName":"basic_info.source","DataType":"String","Operator":"IN","Value":"[\\"BLACKBOX\\"]"},{"FieldName":"basic_info.created_at","DataType":"Int","Operator":"GE","Value":"1689004800"},{"FieldName":"basic_info.created_at","DataType":"Int","Operator":"LE","Value":"1689091199"},{"FieldName":"basic_info.status","DataType":"String","Operator":"IN","Value":"[\\"PENDING\\"]"},{"FieldName":"basic_info.business_tree_id","DataType":"Int","Operator":"EQ","Value":"6"},{"FieldName":"basic_info.risk_vuln_type","DataType":"String","Operator":"EQ","Value":"\\"auth_bypass\\""}],"Category":"ALL","CurrentPage":"1","PerPageItems":"20","OrderField":"basic_info.created_at","OrderType":"DESC"}}}' \
--compressed`, []string{`POST`, `HTTP/1.`, "-A setting", "H setting", "QueryRiskListReqInput\u0021)"}, nil)
	})
}

func TestCurlToRawHTTPRequestBUG(t *testing.T) {
	t.Parallel()

	t.Run("ANSI-C string", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl $'https://xxx.com/xxx.aspx?\60=\61' \ -H 'sec-ch-ua-platform: "Windows"'`, []string{`GET /xxx.aspx?0=1 HTTP/1.1`, `Sec-Ch-Ua-Platform: "Windows"`, `Host: xxx.com`}, nil)
	})

	t.Run("fix1", func(t *testing.T) {
		testCurlToRawHTTPRequest(t, `curl 'https://xxx.com/1' \   -H 'viewport-width: 894'`, []string{`GET /1 HTTP/1.1`, `Viewport-Width: 894`, `Host: xxx.com`}, nil)
	})
}
