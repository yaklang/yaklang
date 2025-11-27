package yakgrpc

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httputil"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/bizhelper"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/davecgh/go-spew/spew"
	filter2 "github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	yakit.InitialDatabase()
}

func TestGRPCMUSTPASS_RawFuzztagBug(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\n" +
		"Content-Length: 1\r\n\r\n"))
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request: fmt.Sprintf(`GET / HTTP/1.1
Host: %v 

{{yak(handle|{{=xxx=}})}}
`, utils.HostPort(host, port)),
		ForceFuzz: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for {
		rsp, err := client.Recv()
		if err != nil {
			break
		}
		if !rsp.Ok {
			t.Fatal("request failed")
		}
	}
}

func TestGRPCMUSTPASS_CheckResponseValid(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\n" +
		"Content-Length: 1\r\n\r\n"))
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	// no fix check
	client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request: fmt.Sprintf(`GET / HTTP/1.1
Host: %v 

asdghasdjfgahjksdgf
`, utils.HostPort(host, port)),
		Concurrent:               10,
		IsHTTPS:                  false,
		ForceFuzz:                true,
		PerRequestTimeoutSeconds: 5,
		NoFixContentLength:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for {
		rsp, err := client.Recv()
		if err != nil {
			break
		}
		if !rsp.Ok {
			t.Fatal("request failed")
		}
	}
	// fix check
	client, err = c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request: fmt.Sprintf(`GET / HTTP/1.1
Host: %v 

asdghasdjfgahjksdgf
`, utils.HostPort(host, port)),
		Concurrent:               10,
		IsHTTPS:                  false,
		ForceFuzz:                true,
		PerRequestTimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	for {
		rsp, err := client.Recv()
		if err != nil {
			break
		}
		if !rsp.Ok {
			t.Fatal("request failed")
		}
	}
}
func TestGRPCMUSTPASS_FuzzerMatch(t *testing.T) {
	data := uuid.New().String()
	body, _ := utils.GzipCompress(data)
	host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\n" +
		"Content-Encoding: gzip\r\n" +
		"Content-Length: " + fmt.Sprint(len(body)) + "\r\n" +
		"\r\n" +
		string(body)))
	err := utils.WaitConnect(utils.HostPort(host, port), 3)
	if err != nil {
		t.Fatal(err)
	}

	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	stream, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request: `GET / HTTP/1.1
Host: ` + utils.HostPort(host, port) + `

`,
		Filter: &ypb.FuzzerResponseFilter{
			Keywords: []string{data},
		},
		Matchers: []*ypb.HTTPResponseMatcher{
			{
				MatcherType: "word",
				Condition:   "and",
				Group:       []string{data},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	matched := false
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		fmt.Println(string(rsp.ResponseRaw))
		if rsp.MatchedByMatcher {
			matched = true
		}
	}
	if !matched {
		t.Fatal("expect matched, got not matched")
	}
}

func TestSaveToDB(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte(`HTTP/1.1 200 OK

`))
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request: `POST /icons/.%%32%65/.%%32%65/.%%32%65/.%%32%65/.%%32%65/.%%32%65/.%%32%65/etc/passwd HTTP/1.1
Host: www.example.com
`,
		ActualAddr: utils.HostPort(host, port),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = stream
	time.Sleep(time.Second)
	tasks, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
		Pagination: &ypb.Paging{
			Page:    1,
			Limit:   1,
			Order:   "desc",
			OrderBy: "id",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "http://www.example.com/icons/.%%32%65/.%%32%65/.%%32%65/.%%32%65/.%%32%65/.%%32%65/.%%32%65/etc/passwd", tasks.Data[0].Url)
}

func TestGRPCMUSTPASS_ChangeToUpload(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	resp, err := c.HTTPRequestMutate(context.Background(), &ypb.HTTPRequestMutateParams{
		Request: []byte(`GET / HTTP/1.1
Host: www.example.com
`),
		UploadEncode: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	packet := string(resp.Result)
	if lowhttp.GetHTTPRequestMethod([]byte(packet)) != "POST" {
		t.Fatal("expect POST, got " + lowhttp.GetHTTPRequestMethod([]byte(packet)))
	}
	if !strings.Contains(packet, "Content-Type: multipart/form-data") {
		t.Fatal("expect multipart/form-data, got " + packet)
	}
	body := string(lowhttp.GetHTTPPacketBody(resp.Result))
	body = strings.TrimSpace(body)
	if !(strings.HasPrefix(body, "--") && strings.HasSuffix(body, "--")) {
		t.Fatal("expect body is a multipart/form-data, got " + body)
	}
	boundary := lowhttp.ExtractBoundaryFromBody(body)
	if !strings.Contains(lowhttp.GetHTTPPacketHeader(resp.Result, "content-type"), boundary) {
		t.Fatal("expect boundary in content-type, got " + lowhttp.GetHTTPPacketHeader(resp.Result, "content-type"))
	}
	fmt.Println(packet)
}

func TestGRPCMUSTPASS_HTTPFuzzer_WithNoFollowRedirect(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.RequestURI != "/admin" {
			http.Redirect(writer, request, "/admin", http.StatusMovedPermanently)
			return
		} else {
			writer.Write([]byte(`ok`))
			return
		}
	})

	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	noFollowRedirect := true
	for i := range make([]struct{}, 2) {
		client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
			Request: fmt.Sprintf(`GET / HTTP/1.1
Host: %v
`, utils.HostPort(host, port)),
			IsHTTPS:                  false,
			PerRequestTimeoutSeconds: 5,
			NoFollowRedirect:         noFollowRedirect,
			RedirectTimes:            3,
		})
		if err != nil {
			t.Fatal(err)
		}

		rsp, err := client.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			if rsp.StatusCode != 301 {
				t.Fatalf("expect 301, got %v", rsp.StatusCode)
			}
			noFollowRedirect = false
			_, err := client.Recv()
			if err.Error() != "EOF" {
				t.Fatalf("expect EOF, got %v", err)
			}
		} else {
			if rsp.StatusCode != 301 {
				t.Fatalf("expect 301, got %v", rsp.StatusCode)
			}
			rsp, err := client.Recv()
			if err != nil {
				t.Fatal(err)
			}
			if rsp.StatusCode != 200 {
				t.Fatalf("expect 200, got %v", rsp.StatusCode)
			}
			if string(lowhttp.GetHTTPPacketBody(rsp.ResponseRaw)) != "ok" {
				t.Fatal("expect response body is ok")
			}
			_, err = client.Recv()
			if err.Error() != "EOF" {
				t.Fatalf("expect EOF, got %v", err)
			}
		}
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_WITHPLUGIN(t *testing.T) {
	var token string
	name, clearFunc, err := httptpl.MockEchoPlugin(func(s string) {
		token = s
	})
	if err != nil {
		t.Fatal(err)
	}
	defer clearFunc()

	host, port := utils.DebugMockHTTP([]byte(`HTTP/1.1 200 OK
Content-Length: 12

{"abc": "111111", "qqq": "12"}`))

	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request: fmt.Sprintf(`GET /{{rs}} HTTP/1.1
Host: %v 

{{params(abc)}}
{{params(a1)}}
`, utils.HostPort(host, port)),
		Concurrent:               10,
		YamlPoCNames:             []string{name},
		IsHTTPS:                  false,
		ForceFuzz:                true,
		PerRequestTimeoutSeconds: 5,
		Params: []*ypb.FuzzerParamItem{
			{Key: "abc", Value: "123"},
			{Key: "a1", Value: "{{rand_int(1000,9999)}}"},
		},
		Extractors: []*ypb.HTTPResponseExtractor{
			{
				Name:   "test",
				Type:   "json",
				Scope:  "body",
				Groups: []string{".qqq"},
			},
		},
		Matchers: []*ypb.HTTPResponseMatcher{
			{
				MatcherType: "expr",
				Group:       []string{"test == '12'"},
				ExprType:    "nuclei-dsl",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	haveToken := false
	for {
		rsp, err := client.Recv()
		if err != nil {
			break
		}
		if strings.Contains(rsp.Url, token) {
			haveToken = true
		}
		// fmt.Println(rsp.Url)
		// fmt.Printf("%v: %v\n", rsp.GetUUID(), len(rsp.ResponseRaw))
		// fmt.Println(string(rsp.GetRequestRaw()))
		// spew.Dump(rsp.GetExtractedResults())
		// spew.Dump(rsp.GetMatchedByMatcher())
	}

	if !haveToken {
		t.Fatal("NO TOKEN FOUND, PLUGIN is not executed!")
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_WithNoFixGZIP(t *testing.T) {
	token := utils.RandStringBytes(200)
	body, _ := utils.GzipCompress(token)
	host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\n" +
		"Content-Length: " + fmt.Sprint(len(body)) + "\r\n" +
		"Content-Encoding: gzip\r\n\r\n" + string(body)))
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request: fmt.Sprintf(`GET /{{rs}} HTTP/1.1
Host: %v 

`, utils.HostPort(host, port)),
		Concurrent:               10,
		IsHTTPS:                  false,
		ForceFuzz:                true,
		PerRequestTimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	haveToken := false
	for {
		rsp, err := client.Recv()
		if err != nil {
			break
		}
		println(string(rsp.ResponseRaw))
		if strings.Contains(string(rsp.ResponseRaw), token) {
			haveToken = true
		}
	}

	if !haveToken {
		t.Fatal("NO TOKEN FOUND, PLUGIN is not executed!")
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_WithNoFixGZIP_Negative(t *testing.T) {
	token := utils.RandStringBytes(200)
	body, _ := utils.GzipCompress(token)
	host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\n" +
		"Content-Length: " + fmt.Sprint(len(body)) + "\r\n" +
		"X: 1\r\n\r\n" + string(body)))
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request: fmt.Sprintf(`GET /{{rs}} HTTP/1.1
Host: %v 

`, utils.HostPort(host, port)),
		Concurrent:               10,
		IsHTTPS:                  false,
		ForceFuzz:                true,
		PerRequestTimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	noToken := true
	for {
		rsp, err := client.Recv()
		if err != nil {
			break
		}
		println(string(rsp.ResponseRaw))
		if strings.Contains(string(rsp.ResponseRaw), token) {
			noToken = false
		}
	}

	if !noToken {
		t.Fatal("TOKEN FOUND")
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_BIG(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte(`HTTP/1.1 200 OK
Content-Length: 12

{"abc": "111111", "qqq": "12"}`))

	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request: fmt.Sprintf(`GET /{{rs(10,10,10)}} HTTP/1.1
Host: %v 

{{params(abc)}}
{{params(a1)}}
`, utils.HostPort(host, port)),
		Concurrent:               10,
		IsHTTPS:                  false,
		ForceFuzz:                true,
		PerRequestTimeoutSeconds: 5,
		Params: []*ypb.FuzzerParamItem{
			{Key: "abc", Value: "123"},
			{Key: "a1", Value: "{{rand_int(1000,9999)}}"},
		},
		Extractors: []*ypb.HTTPResponseExtractor{
			{
				Name:   "test",
				Type:   "json",
				Scope:  "body",
				Groups: []string{".qqq"},
			},
		},
		Matchers: []*ypb.HTTPResponseMatcher{
			{
				MatcherType: "expr",
				Group:       []string{"test == '12'"},
				ExprType:    "nuclei-dsl",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	for {
		rsp, err := client.Recv()
		if err != nil {
			break
		}
		_ = rsp
		// fmt.Printf("%v: %v\n", rsp.GetUUID(), len(rsp.ResponseRaw))
		// fmt.Println(string(rsp.GetRequestRaw()))
		// spew.Dump(rsp.GetExtractedResults())
		// spew.Dump(rsp.GetMatchedByMatcher())
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_ALL(t *testing.T) {
	var requestedCount int
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestedCount++
		writer.Write([]byte("abc"))
	})

	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request: string(lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /{{rs(10,10,10)}} HTTP/1.1
Host: www.baidu.com

`), "Host", utils.HostPort(host, port))),
		Concurrent:               10,
		ForceFuzz:                true,
		PerRequestTimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	var count int
	for {
		rsp, err := client.Recv()
		if err != nil {
			break
		}
		_ = rsp
		count++
	}
	if count != 10 {
		t.Fatalf("expect 10, got %v", count)
	}

	client, err = c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request: string(lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /{{rs(10,10,10)}} HTTP/1.1
Host: www.baidu.com

`), "Host", utils.HostPort(host, port))),
		Concurrent:               10,
		ForceFuzz:                true,
		PerRequestTimeoutSeconds: 5,
		ForceOnlyOneResponse:     true,
	})
	if err != nil {
		t.Fatal(err)
	}

	count = 0
	for {
		rsp, err := client.Recv()
		if err != nil {
			break
		}
		_ = rsp
		count++
	}
	if count != 1 {
		t.Fatalf("expect 1, got %v", count)
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_WithLegacyTag(t *testing.T) {
	var requestedCount int
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestedCount++
		writer.Write([]byte("abc"))
	})

	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request: string(lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /{{rs(10,10,10)}} HTTP/1.1
Host: www.baidu.com

`), "Host", utils.HostPort(host, port))),
		Concurrent:               10,
		FuzzTagMode:              "legacy",
		PerRequestTimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	var count int
	for {
		rsp, err := client.Recv()
		if err != nil {
			break
		}
		_ = rsp
		count++
	}
	if count != 10 {
		t.Fatalf("expect 10, got %v", count)
	}

	client, err = c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request: string(lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /{{rs(10,10,10)}} HTTP/1.1
Host: www.baidu.com

`), "Host", utils.HostPort(host, port))),
		Concurrent:               10,
		ForceFuzz:                true,
		PerRequestTimeoutSeconds: 5,
		ForceOnlyOneResponse:     true,
	})
	if err != nil {
		t.Fatal(err)
	}

	count = 0
	for {
		rsp, err := client.Recv()
		if err != nil {
			break
		}
		_ = rsp
		count++
	}
	if count != 1 {
		t.Fatalf("expect 1, got %v", count)
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_ExtractUrl(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		request    string
		concurrent int
		isHTTPS    bool
		expected   string
	}{
		{
			name: "HTTP Base64 Encoded",
			request: `GET /{{base64(aaaaa)}} HTTP/1.1
Host: www.baidu.com
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.45 Safari/537.36
Accept-Language: zh-CN,zh;q=0.9
Connection: close

`,
			isHTTPS:  false,
			expected: "http://www.baidu.com/" + base64.StdEncoding.EncodeToString([]byte("aaaaa")),
		},
		{
			name: "HTTPS Base64 Encoded with Int",
			request: `GET /{{base64(aaaaa)}}/{{int(1-10)}} HTTP/1.1
Host: www.baidu.com
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.45 Safari/537.36
Accept-Language: zh-CN,zh;q=0.9
Connection: close

`,
			isHTTPS:  true,
			expected: "https://www.baidu.com/" + base64.StdEncoding.EncodeToString([]byte("aaaaa")) + "/1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := c.ExtractUrl(context.Background(), &ypb.FuzzerRequest{
				Request: tt.request,
				IsHTTPS: tt.isHTTPS,
			})
			if err != nil {
				t.Fatal(err)
			}

			if client.GetUrl() != tt.expected {
				t.Fatalf("extract url failed, got %s, want %s", client.GetUrl(), tt.expected)
			}
		})
	}
}

func TestServer_HTTPFuzzerS2008(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request: `GET /{{yak(handle123|{{params(test)}})}} HTTP/1.1
Host: www.baidu.com
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.45 Safari/537.36
Accept-Language: zh-CN,zh;q=0.9
Connection: close


`,
		Concurrent:               10,
		IsHTTPS:                  false,
		ForceFuzz:                true,
		PerRequestTimeoutSeconds: 5,
		HotPatchCode: `
handle123 = func(a) {
	println(a)
	return sprintf("--------------%v",codec.Md5(a))
}
`,
		HotPatchCodeWithParamGetter: `
__getParams__ = func() {
	return {"test": ["ab", "asdfasdfasdfasdf", 123]}
}

`,
	})
	if err != nil {
		t.Fatal(err)
	}

	for {
		rsp, err := client.Recv()
		if err != nil {
			break
		}
		_ = rsp
		// spew.Dump(rsp)
	}
}

func TestServer_HTTPFuzzer2(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		HistoryWebFuzzerId: 6,
	})
	if err != nil {
		t.Fatal(err)
	}

	for {
		rsp, err := client.Recv()
		if err != nil {
			break
		}
		_ = rsp
		// spew.Dump(rsp)
	}
}

func TestServer_HTTPFuzzer3(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	client, err := c.QueryHistoryHTTPFuzzerTask(context.Background(), &ypb.Empty{})
	if err != nil {
		t.Fatal(err)
	}

	spew.Dump(client)
}

func TestGRPCMUSTPASS_Server_HTTPRequestMutate(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("From to POST", func(t *testing.T) {
		r, err := c.HTTPRequestMutate(context.Background(), &ypb.HTTPRequestMutateParams{
			Request: []byte(`POST /ofcms-admin/admin/cms/template/save.json HTTP/1.1
Host: localhost:8080
Content-Type: multipart/form-data; boundary=b4287c56364c86452c746bc63feb846cd10a9ddc1e9ed979996b3519a5a3

--b4287c56364c86452c746bc63feb846cd10a9ddc1e9ed979996b3519a5a3
Content-Disposition: form-data; name="key"

value
--b4287c56364c86452c746bc63feb846cd10a9ddc1e9ed979996b3519a5a3--`),
			FuzzMethods: []string{"POST"},
		})
		require.NoError(t, err)
		_, body := lowhttp.SplitHTTPPacketFast(r.Result)
		require.Equal(t, "key=value", string(body))
	})

	t.Run("same key", func(t *testing.T) {
		r, err := c.HTTPRequestMutate(context.Background(), &ypb.HTTPRequestMutateParams{
			Request: []byte(`POST / HTTP/1.1
Host: www.baidu.com
Content-Type: application/x-www-form-urlencoded
Content-Length: 11

key=1&key=1`),
			FuzzMethods: []string{"GET"},
		})
		require.NoError(t, err)
		headers, _ := lowhttp.SplitHTTPPacketFast(r.Result)
		require.Contains(t, headers, "?key=1&key=1")
	})

	t.Run("fix invalid host cause mutate failed", func(t *testing.T) {
		r, err := c.HTTPRequestMutate(context.Background(), &ypb.HTTPRequestMutateParams{
			Request: []byte(`GET /?a=1 HTTP/1.1
Host: {{payload(test)}}`),
			FuzzMethods: []string{"POST"},
		})
		require.NoError(t, err)
		headers, body := lowhttp.SplitHTTPPacketFast(r.Result)
		require.Contains(t, headers, "Content-Type: application/x-www-form-urlencoded")
		require.Equal(t, "a=1", string(body))
	})
}

func TestServer_HTTPRequestMutateWithoutConnection(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	r, err := c.HTTPRequestMutate(context.Background(), &ypb.HTTPRequestMutateParams{
		Request: []byte(`POST / HTTP/1.1
Host: www.baidu.com
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.45 Safari/537.36
Accept-Language: zh-CN,zh;q=0.9


`),
		FuzzMethods: []string{"GET"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(r.Result))
}

func TestServer_HTTPRequestMutateWithConnection(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	r, err := c.HTTPRequestMutate(context.Background(), &ypb.HTTPRequestMutateParams{
		Request: []byte(`POST / HTTP/1.1
Host: www.baidu.com
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.45 Safari/537.36
Accept-Language: zh-CN,zh;q=0.9
Connection: close


`),
		FuzzMethods: []string{"GET"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(r.Result))
}

func TestGRPCMUSTPASS_HTTPFuzzer_FuzztagVars(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	token := utils.RandStringBytes(100)
	targetHost, targetPort := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		time.Sleep(time.Second)
		writer.Write([]byte(token))
	})

	start := time.Now()
	client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		ForceFuzz: true,
		Params: []*ypb.FuzzerParamItem{
			{
				Key:   "a",
				Value: "{{int(1-10)}}-a",
				Type:  "fuzztag",
			},
		},
		Concurrent: 7,
		Request: `GET /?c=1&d={{rs(10,10,3)}}&c={{params(a)}} HTTP/1.1
Host: ` + utils.HostPort(targetHost, targetPort) + `
`,
	})
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	payloadDiffFilter := filter2.NewFilter()
	for {
		rsp, err := client.Recv()
		if err != nil {
			break
		}
		if len(rsp.Payloads) > 0 {
			if payloadDiffFilter.Exist(rsp.Payloads[0]) {
				continue
			}
			payloadDiffFilter.Insert(rsp.Payloads[0])
			count++
		}
		log.Infof("url: %v payloads: %v", rsp.Url, rsp.Payloads)
	}
	payloadDiffFilter.Close()
	if count != 30 {
		t.Fatal("expect 30, got " + fmt.Sprint(count))
	}

	if ret := time.Since(start); ret.Seconds() > 5 && ret.Seconds() < 6 {
		t.Log("time cost [" + ret.String() + "] is expected")
	} else {
		t.Fatalf("time cost is not expected: %v", ret)
	}
}

// nuclei-dsl type tags and raw type tags
func TestGRPCMUSTPASS_HTTPFuzzer_FuzztagVars2(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	token := utils.RandStringBytes(100)
	targetHost, targetPort := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		time.Sleep(time.Second)
		writer.Write([]byte(token))
	})

	start := time.Now()
	client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		ForceFuzz: true,
		Params: []*ypb.FuzzerParamItem{
			{
				Key:   "a",
				Value: "{{int(1-2)}}",
				Type:  "nuclei-dsl",
			},
			{
				Key:   "b",
				Value: "{{int(1-2)}}",
				Type:  "raw",
			},
		},
		Concurrent: 1,
		Request: `GET /?v=1&d={{rs(10,10)}}&a={{params(a)}}&b={{params(b)}} HTTP/1.1
Host: ` + utils.HostPort(targetHost, targetPort) + `
`,
	})
	if err != nil {
		t.Fatal(err)
	}

	// only 1 request
	for {
		rsp, err := client.Recv()
		if err != nil {
			break
		}
		if len(rsp.Payloads) != 3 {
			t.Fatal("expect payload count == 3, got " + fmt.Sprint(len(rsp.Payloads)))
		}
		log.Infof("url: %v payloads: %v", rsp.Url, rsp.Payloads)

		a, b := rsp.Payloads[1], rsp.Payloads[2]
		if a != "-1" {
			t.Fatal("expect params(a) == -1, got " + a)
		}
		if b != "{{int(1-2)}}" {
			t.Fatal("expect params(b) == {{int(1-2)}}, got " + b)
		}
	}

	if ret := time.Since(start); ret.Seconds() < 2 {
		t.Log("time cost [" + ret.String() + "] is expected")
	} else {
		t.Fatalf("time cost is not expected: %v", ret)
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_Matcher(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	targetHost, targetPort := lowhttp.DebugEchoServer()

	client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		ForceFuzz: true,
		Params: []*ypb.FuzzerParamItem{
			{
				Key:   "r1",
				Value: "{{rand_int(1000,4000)}}",
				Type:  "",
			},
			{
				Key:   "r2",
				Value: "{{rand_int(1000,4000)}}",
				Type:  "",
			},
			{
				Key:   "res",
				Value: "{{int(r1) + int(r2)}}",
				Type:  "",
			},
		},
		Matchers: []*ypb.HTTPResponseMatcher{
			{
				MatcherType: "word",
				Scope:       "body",
				Condition:   "and",
				Group:       []string{"{{res}}"},
				ExprType:    "nuclei-dsl",
			},
			{
				MatcherType: "word",
				Scope:       "body",
				Condition:   "and",
				Group:       []string{"{{xxxxx}}"},
				ExprType:    "nuclei-dsl",
			},
		},
		Concurrent: 7,
		Request: `GET / HTTP/1.1
Host: ` + utils.HostPort(targetHost, targetPort) + `
key1: {{params(r1)}}
key2: {{params(r2)}}
key3: {{params(res)}}

a={{base64dec(e3tyZXN9fQ==)}}&b={{base64dec(e3t4eHh4eH19)}}
`,
	})
	if err != nil {
		t.Fatal(err)
	}

	matched := false
	for {
		rsp, err := client.Recv()
		if err != nil {
			log.Error(err)
			break
		}

		matched = rsp.MatchedByMatcher
		// fmt.Printf("%v: %v\n", rsp.GetUUID(), len(rsp.ResponseRaw))
		// fmt.Println(string(rsp.GetRequestRaw()))
	}

	if !matched {
		t.Fatal("NO MATCHED")
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_Extractor_Kv(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	targetHost, targetPort := lowhttp.DebugEchoServer()

	client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		ForceFuzz: true,
		Extractors: []*ypb.HTTPResponseExtractor{
			{
				Name:   "test",
				Type:   "kv",
				Scope:  "body",
				Groups: []string{"int_2"},
			},
			{
				Name:   "test2",
				Type:   "kv",
				Scope:  "body",
				Groups: []string{"float"},
			},
			{
				Name:   "test4",
				Type:   "kv",
				Scope:  "body",
				Groups: []string{"map"},
			},
		},
		Concurrent: 7,
		Request: `GET / HTTP/1.1
Host: ` + utils.HostPort(targetHost, targetPort) + `
Content-Type: application/json

{
  "int": -2147483648,
  "int_2": 9223372036854775807,
  "string": "I AM STRING",
  "float": 0.000000000000001,
  "map": {
    "key": "value"
  }
}
`,
	})
	if err != nil {
		t.Fatal(err)
	}

	matchedCount := 0
	for {
		rsp, err := client.Recv()
		if err != nil {
			break
		}
		res := rsp.GetExtractedResults()

		for _, v := range res {
			value := strings.ReplaceAll(v.Value, " ", "")
			value = strings.ReplaceAll(value, "\n", "")
			if v.Key == "test" && value == "9223372036854775807" {
				matchedCount++
			}
			if v.Key == "test2" && value == "0.000000000000001" {
				matchedCount++
			}
			if v.Key == "test4" && value == `{"key":"value"}` {
				matchedCount++
			}
		}
		// fmt.Printf("%v: %v\n", rsp.GetUUID(), len(rsp.ResponseRaw))
		// fmt.Println(string(rsp.GetRequestRaw()))
	}

	if matchedCount != 3 {
		t.Fatalf("extractor kv failed")
	}
}

func TestHTTPPacket_ToUnquoteFuzzTag(t *testing.T) {
	logoPngBytes, err := hex.DecodeString(`89504e470d0a1a0a0000000d4948445200000373000000b20806000000e81596f6000000097048597300000b1200000b1201d2dd7efc00001ec649444154789ceddd3f6fe4487ac7f1e278f29e8bec8ba44d9c4a9b1bd05ce24d0c4807383927dbfb0a46139bc648304303337a05db939c13032b8576b212fc02560a1d9d0407b6a3935ec01d8d6215a55693ddcd26abc8aaa7be1f40b8394abb3bfd8faa5fd553f5646559aa4ef26c5f2975aa949a2ba51e95520b559467ddfe610010cedc23bf28a58e955277f6cf97aa281f79e90100800fddc25c9e9d28a57e6a5c57ea9c4007207979f64e2975ab94da5b792a74a87b4fa00300003ebcd9faef34b3cd8bc675e3b4710500d273d212e4b403bb42070000e0dcf630a7cb84949a35ae1a331bf60020659bee83dfab3c9b37ae0200000cb439cce5d9999d59de64d3200600a057e798f80200008ebd5dfbaf33038f4f8debc0aef2ec7de4cfd9a32acadbc655e0c5e196e76266cb2d4f1adf010000e8697d985bbf4f0e31ca333dd87cd7f2375f17b4de6d19a0eeafd92324539e757d580f4aa9fbc6d517f71bbe7f6b4f8a7dad28af1bd7109ab6cfd6aae3ea30a9a2bc6c7c070000a087f6306756528e1ad7dbe99f65b0e9433380edb794b5b685b1aeaf1ddcdbdb1272777f6dda83645b686c0b83ab9fcd5b4e56f4a26b09e517bb0f19000060b0f6d6047976db61af5cedab2a4a36f7aff33a90adae76ad86b3c30d87cd003edcad04c0e5f0f76803628d20d8c6b425f863cb77d6f9411525950f000060b0e6ca9c3975ad6b90535b4af1e43203b843bb32b61aca5819432c563feb9bdfbb2fab84cb2b8375e9e875a225a1bbde03cf28630700002e3457e6f2ecba4718f94615e5ba7d407298007762bf8e7907020d4fb68c70914cb0cb335d3af9a1717d3356e70000c060af5b139892c03eab4a6dfbb6e4d021ceb469d081f547821cb0d6aceaaba6d4cf2acfeeab033fe4eb73ff3b6d5c010000d8d16a9fb9be030cb903363318bdb56d1ad8cf0674a70f82f9a95aed97da63cd3cae5dcad26b0776f20c0000a0b79730f75242d8c7b1c8c19a299ffa69cbe98400363baa264464aed20d596163750e00000cb2bc32773270e549ce8996a6acf2b6c73e1800ed6676954edac9b7431e0f0dc40100c020ab616e8853bbba1737f318ae7b964e01d8ec473181ce3c8e211360b344f6140200004f96c3dcd0434c6642ca862e09728057f1073a33e9f3a5717d77b20f8f0200005e99309767ef1d1dee711af5de397362253de200ff7e8cfc00905347f74cc21c0000e8ad5e997335a09845db0cd704da4f8deb007c89f55e71e8f05e7120a23c1d00004ca20e732e67c88f549ec5586ee9a2640a40770776353c36ae43282d0a0000402fae57e66a9fa32aa132fb77d827078c2fae8393f26ce1e15e41a9250000e8e58d1d48f968861d53a3e01857070009e23938c94cfa7cdfb83e1c2b730000a097b71e0712b3ea6448bd17ad281f1bdf0d855941a42978789eaa46d36ebd63053648f3e027544c90fbb171dd0df6cc0100805ede7a1e481cd815ba135594f78def86415a13e35ddcacf9d97bfbb5ceb6ef6b8faa285d87b1e9998372bad8f473fbf6ab4d8aa7a9ee55932aa1be5ffc0639c509ba0000a02f9f2b73b5836a85c5acd08538588ba969ef6af86a0b55b755905abd16f2ea684c8af2bae3dfb6ebcf75d31e220f5b2663567f6e3f9295e7130f2bb1c3e5993e18e943707f2f0000903c2d2bffb12a6f1aeb48fe8faa28c33935d2ec17fc63e3ba3f0f2be16b79c0ffb8329895b9b2856999f7fcf204ceea2ae16a401c6bd5e84615655b609d86799e2e477cfcdf045cbd00000002f576e4bfd6e7aae452973686317071bd2af9d506b29710d67d2507f0cfacd0f67b4f9a038deae0b76f4b945d859d704a0dcd3d6ae1e960a875f65b56d9010000361a3bcc293b68bbb5e54b5f262eff5bb76f69574f55791b2b6990cc4cc02c078e85ed29f959c4a336a5ac67ec610b8859213d6d291f8ecd75f595c2e49e99f499b7acf2c7e6d68e51a69b6431fb75df3b1cabc4ae9e8c5c24b975e4e57e18b3cb49c6cae6b9abef4b923e4ffaf3f0658a30a7ec8cf727db63ea8bfd604e71c374f5829e10e490245d366d42d071b40f9f1017b2d311b701f874543d8e3cfb4115a5eba6f3e130a7435f8fbcaaedcb911df84d3391607a5afa688512bbe3ea7e1dee390c3e2da2fe5d6bcc470f53b2ee4babaa71cb9bc6e571d5a1ee0f2acf2eab59a8787ad3d51e28a544e2e21b9cea9bbb9e48ca333d89f433412e58d27af0fdb8e630a3f899996f6903a669ee0bfe7a5a4a31ab564dd3137b9053a31fc8f6b2ff5d6290ab1d4eb532d7e6f8f98d9a6777b6c4e17e657f8feb53195d94805c36ae0069713399e1ba3dc1ebc35eea3fd733ed926fec924898895eb5b0ef7569656263ef331dc3c544ffdd984ed99eca5135f9cfc151d86c91402fe94548616ed9c15273e7d7253679d6f8e18971e43fd2a607a56e3e97bf04f8f91e0b03923645a92b36ae8405ba3d5bd61bfbde9717e6d02069a1fbc1be4ee3329350d29e4b5fde4759198271983dfdd23f4b17faf7e4d46596000066973799db43a624f920a6dcd2840f8903eaf944aba7acca75c7738576669fdcf89331e3ba5345594d0abe09b2512f0000eab99dc65ce073b1b041287632cb2ba7db0b4f40e9ee58c867082ebd4c3049de4ef1b4fc7bf10d6582001c90b67232a69b741e6a4fbadc52a9ab28ffeeebed453f734c79a50f9458ee86f08b55674b5bb5a43a5b3e63e00d27310ec6ca26c0e7600826d4baa1dc32249457ba67c23176c3738617e633f441f8337255b5855a52ef997b68fc28ba6220066008827017945b86466219d3d5c413dc0493dd516a0943ee04d3b287b6df83759863300100d3a03aa22bca2dc320b3bcf2a96d903432c25c3f3c6f5009f49353eb2a07087300302deebfbb915a6e1947837433fb2db16173eb2069342620d3ffb21f998df8d15d9e9d4dd6e47f3ce7eb2a07ea30d7fa4d00805777029b47fb659e2f393dda5ec4521e7426b009ef955df59d12ab4bfdf1dca5cc4c847d12fe0cdca8a25c5bc161c29c497a9c46d70f033180a6d77d3191d647512e049e027a606797c3650e6b9176b84008e5958a4032c88cc36312652a05a69e88f16deb3d6ab969b8f427c38fa5a341818411e6fae1bedb9fc472cb4fc1965b727aa53f26245362390c612e4dfa9e24ad526095be476d1c632d8739668801603c4febeadfd181f9e516779fb676a10626ca2bfd21880cc773989a3c9b27d097f1a2cb3dea25cc99b2154a2d01601cacca0d657aed506ee91be595be114486a3d4322579b62ff420a665775d272cdfacfc7f061700300ee9bf88c642b9a54f9457fa655e67e965626321cca523d936046d56c31c830b00f0ef8efdb68e506ee91be5957e496c843f155a14a420cfbe54150cb27ddc658cf03acc997f505ac90a00ff38d575374c9cb944b9a51f94578e81d52477f6a2e9d7887e64de93565dd9df699dadaecc29a1339cbedcc97c58c0ce5865eaeec1ee51865b945bba24b7bcf22c98de8e9458fac04aa754b42158ab19e6cce96aacce75c36a04805d3161e603e596ae492cafbcd975c6db33ca02dd63a553ae14f6c99df4996c6a863983c10600b8c7aa9c4f945bba4179e5585845728f524b89f2ec54297524fc519ef76d57d41ee6589d03001f18bcf9772af0318d576e29bbbc7263e3dd5199a3d5a51fe23015eeb392987bdf67e18f521f8ad67bd2ae3dcc1912f71f00c054ae68123e027390d7b9c0473656c0a2bc721c9403fac3732b85dcc9a5654f43dfb3ebc39cdcfd070030b627a12b466132339cd20ea8f25f6e4979e598583df267cfae7c227e29b421381d5a35b03ecc29b1fb0f00606c836fd6d899c4c1b2bf724bca2bc74389e518589d8b5d9ee9d7f07be18ff2ab8b7df49bc39c41b965e8f2ecf72acffe2bf5a7019322a8ace7e4668d1d516eb92bca2bc743d0f08f95cf9899090fe9bf371f5c55ec6c0f7366468b0f45bbe97b6be920a7d4ef94527fadf2ec7f1adf07c6c0aad33a7794574e8872cb6e28af1c1b2d09fc3ba0d4326ab421d841979539fd0b513fa91f1bd7316d9fb9972057fb2b021d100c33980ca54171ba28b7dc2ec4d5aba1c22baf54cfe5acc78debf08115d01899c92ae965c81f6df58813ddc29c7ade3ff7b5711dd36806b91a810e98de5335fbeef0668d9e28b7dc4ce6c0e92ed0f24a45c018155565b13155029f843f4ae7e5dfddc39caa7e29ce097401581fe46a043a605aa704b980506ed9ceacee491c38853c8827cc8d8752cb989855eb4be18f72701b8236bb853945a09bdcf6205723d001d3f881034f8224b5dc72c86055e2fbf43cd889144a2ca7c0fec4782c12d827e765ebc5ee614e11e826d33dc8d50874c078f48cdb6f097281a2dcf235b9e59521f7c765556e7c3ce731c8b3d304263a2eec1924cef50b73ea39d05d34aec38fdd835c8d4007f857ef91935e2212bb2ff63868498eec40a83bca2ba742b018dfb15d1145a8ccfd28e4491817f44493b793adfb87395505bad3aaa4883e747ef50f7235021de08fde8b75c81eb90898f21689e596673b965b525e390d4afea641880e9509dad2cb2bbdb7491916e654f5cb71510d64e46d2e0fc3f0205723d0c1b7142775f400f2903e7b1129ca6b815525b3ce018df2ca69e4d94902fb814245980bc9eb89a714da10783f106d789853b661b01ed0c8dc8fb089df019cbb205723d0c1a79456a674a9deb7c10f20b1ce5992e59694574e8940311d4a2dc37259b52030134b1f843fd6ab31f6d1bb09733533b0f936a1553a7f61ce7d90ab11e88061ce29ab8c5cbae59694574e8730372d9eff70e895b89f13e827f730d6ef19b7614ed913c3cc2add47f6d2f59467b9a7205723d001bbbb795e8df370b43046965ab925e595d3a1c43204ec57c4d8bcb42168e33eccd54c77f37d5a18f450948552eacf9eff2b043aa09b27db3bee3dab71e2482db7bc7c5ea1d3e56526c8515e391d5685a6c76b80319ddb09c351f80b73ca96b2981606bfb1b3dae8fedcfd05810e98dc79352945ef3899e4965bea7e4d7f5079562aa5fe2834c85d4434b9429098deccae9002bedd8c5d31e037ccd5743ad5b3da26d4499b05f58740074c4557147c4349650264965b4af7104d5f2a73e80c25966120ccc137ef6d08da8c13e66a26d4eddbde7484ba2e0874c098ea1037a7dd405224965b4a36da5e140724aefcc68a3007df26193b8c1be66aba64e925d4517eb90d810ef08d109732130c361feb8f505c8cb917c50102443828b5844ffade7439c5333c4d98ab995057975f7250ca26043ac0b527bb27ee578438d85fc2573c11418ba7bc523d9758ee35ae634a8439f87037e5bd69da305733e597f36a66dc0cae28776943a0035cb8b1a753be634f1c56cc69a913b498ca2b1525964122ccc1b5a7a9ef4d6184b99a9e1937832b5d82f95b56eb5a10e8803e1eec44d137b6c500a753a249eee99612c4565ea9080e419ad91553c095b3a94fd60d2bcc2dd3252f66b5ee57766f1de52f35021dd0c5833da5f0db6a82c84c14514a89cd28b70c515ce5954a5c89e5832d239382091bb87265fb6a4fea6df02fa799295dd82f6537afbeb7335ee9d6a2eb4097677ff21cc84da02bca5f37be0384499750eac1f8350dbe31801eecdd73a47c30622baf54769c22c1573bb1aec75ffa31fd2ce0319d70e0111c98a40d419bac2ccb96cb91c8b37d7bc3acbfc60e77bf99bcecc37fa0d3fe974087adf24c7f168e467ea26eaae066c25b6c255808999938fc89d76872babc32be81779ee9c9a483c6f5f8fcea5590ce333db1febd80c7f56d74137e7916f1805da4e9338015fecadc26a6646a79d5ee9d52ead0063bfdbffb426ea6ebb1428770f89c39d73360b7afbe5879834fbadc32cf74b9e531cff3649ea22baf54cf13cd12c61e572d2ba29742c2dc9cd5390c701ed20472dc616e95b9e95cdbaf17e6c6bafca543df87c63f1f2b021dc270693f5bca7ed6baae943fd892366503611dd26eabffcf8a1ba643b9e5b4622caf54820e3e69f6cc32931c4f023e13945aa2afbb6a0f7e40e22eb31cc2cd72f537411da840c92500b845b9e554f4aa509ca1486a89652dcf2e85ac58c7556a499965089eaacabfc00e530bf734cb188476321ea75c02805b9c6e3985600e16d8999c12cb9bd620673457ece224e5901a8c671ee2a9d8843969087400e0da29cdc447156b79a5125d62d9ed7b31a1450176f1d54eee0587302711810e00dc3133b1f11dc411a7ab50074c1d4959ed59ff1a98a02d61b5fac0aea402db3c84bcc792302715810e00dc318d616f7846bd8ab7bc523d9fa82d612fd95d8752b2f5612f2e525652e1d749c8d5028439c9087400e0d29c724baf622eaf548282c1a271a589524ba4e263e807e510e6a41b2fd0fdbe71150024a1dcd2a7d8cb2b5522fbe50c4a2d91861b5b951134c25c0ac60974bf6b5c01006928b7f421eef24a955c89654d4a0f504a2dd1e62996f706612e1563043aca2d01a481724bb7622faf54899558d6685100c982de27b72ccd309767697e70fd07babf6c5c01006928b774e9464079a51214e6baafb699cfc15de37a7c8eedca2a50bb504519cdca332b73a9f11be8b2c615009088724b17e22faf7c216192f8a1c7410fbbace4858c524bd474a971b06d08da10e65234ce1e3a00908e72cb61ce76d89f15ae3cd3416016fde3e85736498b0248135dd505610e00803e4c1009fea4b34045714a5c4729ee973328b5843cd1eddf25cca528cffec46b0f000e14e59990c1ec982495572a2161ae4f89658dd53948125da87fdbb802d9fc06b9b27105e9c8b3bf77b06fe45a15e5bf35ae0261d3c1e4175ea3ce649457aae44b2c97ffd94f8dabf13911b40710fd7d5179761dd309bb6986397d424d9ee0591dfe57e4feaf710529f917a5d4dec0c7fb774a29c21ce2a25734f2ec5cc880d63749e5952ae912cb9a79ff3f38b8ff4f8d160550f67dbc88e9b34da95d2ac628ad2cca5f37ae01400a28b7ec425a79a51212e69e069458d624945aceec4a2ba0f7504673af22cca5609c3d72ffdab80200699116545c93535ea9aadfad8794583ea34501a4d1e596fb313c26c29c746305b9a2fc87c65500488959dd38e7356f25adbc52090aefc3c39c79ef3f34aec7873087da2c961567c29c640439001817e596eb485cb5945262e96ac04aa925a4395079167cdf39c29c54043900980ae596af9d8b2aaf54cf2596b11ff8a11c07305a1440a24ff6f31e2cc29c44043900980ee596cbeeec6aa5349458aed227859b436e624798c3aacb909bca13e6a421c801c0f428b7ac495da5a4c4b29d9452cba0576230babdea40944011e62421c80140484e137f35ce1d1c791f1e392596d78d2bc34929b5a4541aabbe0f753f25616e883c0ba3c1649efdadcab33f13e4002020a6ecec22d197446a79a512d45cda7df0322b7d945a42aa4588ed0a0873b1d3414ea97f574a659e1f09410e00767726e4c8f65d495ed960bfdc661256e7f628b5448b59883d150973317b0972be11e400a08fa27c4cb0644b6679a5aa7eefee57c795c7efcabe377da0d412921d85d6ae8030172b821c00c421ad724bc9e5954a50f99dbfc0e5fe5095a9506a8975826a5740988b11410e0062934ab9a5f4d50c4a2cbbb9f2fcef1f03a596d82498760584b9d810e400203e69945bca2daf54a24a2c6f3c9658d6a4bc0fa41c7603f782695740988b09410e00e225bbdcf2417879a51254767734c28a8294152df6cd619320da15bc6d5c419808720020c1990d0512fa942d4b61d02b6995e6da1ee2e063854ebfbf8f1b57e37450adc816e5bd90c703f716f63de27bb57b2dc25c0c0872ebe5d97f2aa5fe66edf7435294bedb474cab2883ebbd020447ffc2cf331d7c7e16f4e25cd85547b9cc4a969480a26cb9e84f8dab6873124a391d8234b37b50279beca1cc327404390090c5049faf421ed3835d6d948e930dd345a925b6d1a5cba75b7ec61bc25cc80872002055708d677b5a4c595e3422c25cba0eece137c0269fa73afd9430172a821c0000d393576289dd11e6d1c5628a760584b91011e4000008050379d0a2005d1c4c51769e7298bb695c490b410e0080ed0873380ea5413482f761ec7605accc85a828ff4329f59dc7bf19410e00806e58958122d46307a3965b12e686f1b7d1d15fa023c80100d08599619ff15c8130871dd4ed0a4641981bc66fea761fe80872000074c7001e354a2ddd78aa7a539ab626928dd6ae80a6e1a1d3812ecfbe7370200a410e7ee5998ba6c1a7aa286f1b5701601a84392c3b11d456642a73559497d5effb3cbbb5878648f5b91a1b791ed710e662303cd011e430862307ff0d663d018481124b3411e6863241aea61bb25f0bff9c2dbc6ecba2cc3222fd4b2e09720000ec8e5539ace2301c97cc8ad5e847f98f4c379dffe2f33f49988bc9ee818e200700403f8439ac9a8d7decbc7845a983ce95f087a9db15789b0820ccc5a67ba023c80100d0479e1d526289350873eecdedc128925dfa3a40873017a3ed81ee9f08728814879f0008c19c57016b10e65c2bcac7049ed799affd962987b9b8078deb03dd77aa288bc6552006e6860e005363c08e7528b5f4a128af6dcb02c98e7db42b4839ccb918344e7bf25e33d07d67af01e3cab37d9e7100229812cb3d5e4c6c4098f3a12875d0b993f7c05e39b3f71867684d308cd7a3463b79695ba0120d72ff5c959586efbf23f83b0e41980320052596d88630e78ffefcfd22f5c12d955b3acb10843909525e8d338f9dd54800802b0cd4b1cdac5a5df1dc0c3a49fa39cdb38f55c36db94cbb02b312391807a0000000284a2cb11356707d31ed0a6e643eb867ceda1510e60000000c9a42a32b5670fd3aa15d4137843900a178e095003031565bd0d59eeb832cb0c49c6e2dfdf3e8a45d01610e4028ee7925004cc69cca7bc00b801d10fe7d2acacb44da150c7a1fd1670e000080b239ec8ef78c7f6709b42bf832a4c5137de686e13876c0609f0980d8b1ca825d516ae95b3ae596978dab1d5166390c275e0100103b4a2cd11f9399be9916101f653fc8aa5dc159e36a0784390000903acae5d0172bba6348a35dc1a73eed0a087300002075acaea0af8321fb9db09314da152c766d57409803108a6b5e0900a33303a7639e780cc0caee18d2d83fb7b76bbb02c21c0000481903710c45a9e5584cbb82afc21fe54eed0ad20d7345c92a00e00e252600624598c350945a8eeb5429f520fc31766e57c0cadc503d362a0202f14b0c407c28b1843b4c0a8cc5945b4a7fbe675dcb2ddf36ae000000a441d28050979edd37ae866f2ea4d5d3fb6a3505e3d0ed0af2ecbc3a0152aea3aa5d41516e6c594098030000a992525df3832aca9d0e4d08469ee900742b20d01d572bbd66d50863d021c754c81d097ebe75bb824bdb6baf1565960042b1f64605009e4858997b8836c8a9e79239292b5a945a8e6f9e40bb82cb4ded0a520f73d25f7c2026cc6602184f9e9dd87d29b1bbe43104833037b6a2bc4fa45dc1da098fd4c39c8b95000e7e00943ae4390010192903ef7857e56a66407ed7b81e9fe34d2b28f0248d7605dfdb09a886d4c39c0b843940c6ec3680b4b40e8c22f3b0692f4d64589dc31029b42b58b4b52b20cc010080b450621922c21cfa337b2fa5975bb6b62b20cc010080d45062191ab3c228616585fec35329ca6ba5d4b9f04769da152c21cc010883b90903c018240cb8259558d624accecdd6ed6dc2084c4f3609fb2f37d1ed0a9ecf2a483dccb9183c72f003d2b674430180e0997b968426d51227c0a4ac3412e6a6759252bb82d4c39c0b9c5c84d411e600c444cabe1a49fbe50c39a59684b92999d3514f853f4a3d2155955b12e6dcb826d02149e67dbfb6f709000448c240fbc91ec72e9194524bf6ce4dc934d2bf12fe283fe892ded4c39cab26c50755cfba3c9bf3e14532cc7bfdd6d18970d28f13065649d9eb14e3e39030f92a35c8295187ba606af304c617a76f1b97d2e2f297905eeefcb1fa539e2d5fbf59faf3bdfdaa2dd7bb3f0adcc88c989855b6e592c9fd953e8acb1315878e8ff5be6f5c0124d3c768e7d947a5d4e7881fe54da4ab438b6a463b6e722b22f45828cfeeec4479ac9e3c4c745c555b7be236ee3e36739f3db1e36da9fd70efb3b22c1b57939267a13e01ab3782c7961bc3eae6670261aa9a414cb584b1b69f390ae4193bb727500169312bdcf395cf6ae81eabdf3f45196fa0c8b3d348cb2defab2027fd77bdf99d368ff435bab5af91db49ca976d0d31dd2b964df7de7d793fbd17b2325fd3cfe519612ecf6e239ffdd966ddecd0ed9a32d375d755f541747d734a8939416ddd4d64ddf7daaebf13f89efd968908000080dd10e6ccec5ccc652ea179d8b164ae6dc53104abab5adb480c5863d1bd92629d69040000984cea7be694dd444c987367af47ff9ed86bc0310c9bdd0100007a48fd34cbba17c5d7c675006321cc010000f440983338780198c605fb30010000fa21cca9e7d5b98bc675003e3d3191020000d01f61eec5198d8b8151cdab1e30000000e88530573383ca93d11b1a0269ba88b4d930000040300873cb4c9fabd3c675002e7d5545c9e70c00006020c2dcaaa2d427ebfdd0b80ec0051de4e63c93000000c311e6da9840f71b4a2e01a72e0872000000ee10e6d629ca6ba5d4a152ea66cd4f00e8464f8afc96d24a000000b7b2b22c794ab7c933bd9af0452935dbf293005e3baf3e3b9c5a090000e01c61aeab3c7b674fbbd42d0cf6e2f84b0393d02d3e16d5170dc1010000bc21ccf591678736d81dda2fc21d5276a794d2a14d9f067b694f850500008067843957f26c5f29b56fff6def6cc8ab1dda6bf5f70ea27d9c90eec986b29afe735d22f9f8ea7b665f290000002642989bdaeb10a8ec9f97ffff6a3054f6ffb37f0f6dee96c2976a0430e37508239401000044893027459ebd6f79246dd7dac261eda87105be3cd8d2c4556de14bd99f5dfdf97bf6a4010000a48b30876e9a2b88ebb405c86dbafebb7de8bb2ab5fd9f63c50b000000be28a5fe1fd87e51412a822ade0000000049454e44ae426082`)
	if err != nil {
		t.Fatal(err)
	}

	fuzztag := lowhttp.ToUnquoteFuzzTag(logoPngBytes)
	results, err := mutate.QuickMutate(fuzztag, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatal("expect fuzztag to be mutated to 1 result")
	}
	if len(results[0]) != len(logoPngBytes) {
		t.Fatal("expect fuzztag length to be logoPngBytes length")
	}
	for i := 0; i < len(logoPngBytes); i++ {
		if results[0][i] != logoPngBytes[i] {
			t.Fatalf("%d byte error: %v(got) != %v(wanted)", i, results[0][i], logoPngBytes[i])
		}
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_FuzzTag(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(""))
	})
	target := utils.HostPort(host, port)
	for i, test := range []struct {
		tag       string
		expect    []string
		fuzzMode  string
		forceMode bool
	}{
		{ // 验证force
			tag: "{{base64({{url(yak)}})}}",
			expect: []string{
				"JTc5JTYxJTZi", "%79%61%6b",
			},
			forceMode: true,
		},
		{
			tag:       "{{base64({{url(yak)}})}}",
			expect:    []string{},
			forceMode: false,
		},
		{ // 验证fuzzMode close
			tag:      "{{base64({{url(yak)}})}}",
			expect:   []string{},
			fuzzMode: "close",
		},
		{ // 验证fuzzMode standard
			tag: "{{base64({{url(yak)}})}}",
			expect: []string{
				"JTc5JTYxJTZi", "%79%61%6b",
			},
			fuzzMode: "standard",
		},
		{
			tag: "{{base64(url(yak))}}",
			expect: []string{
				"dXJsKHlhayk=", // url(yak)
			},
			fuzzMode: "standard",
		},
		{
			tag: "{{base64({{url(yak)}})}}",
			expect: []string{
				"JTc5JTYxJTZi", "%79%61%6b",
			},
			fuzzMode: "standard",
		},
		{ // 验证fuzzMode legacy
			tag: "{{base64(url(yak))}}",
			expect: []string{
				"JTc5JTYxJTZi",
			},
			fuzzMode: "legacy",
		},
		{
			tag: "{{base64({{url(yak)}})}}",
			expect: []string{
				"JTc5JTYxJTZi", "%79%61%6b",
			},
			fuzzMode: "legacy",
		},
		{ // 验证优先级
			tag:       "{{base64({{url(yak)}})}}",
			expect:    []string{},
			forceMode: true,
			fuzzMode:  "close",
		},
		{
			tag: "{{base64({{url(yak)}})}}",
			expect: []string{
				"JTc5JTYxJTZi", "%79%61%6b",
			},
			forceMode: true,
			fuzzMode:  "",
		},
	} {
		t.Run(fmt.Sprintf("tets: %d", i), func(t *testing.T) {
			req := &ypb.FuzzerRequest{
				Request: "GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n" + test.tag,
			}
			req.ForceFuzz = test.forceMode
			req.FuzzTagMode = test.fuzzMode
			recv, err := client.HTTPFuzzer(utils.TimeoutContextSeconds(10), req)
			if err != nil {
				t.Fatal(err)
			}
			rsp, err := recv.Recv()
			if err != nil {
				t.Fatal(err)
			}
			if len(rsp.Payloads) != len(test.expect) {
				t.Fatalf("expect length %v, got %v", len(test.expect), len(rsp.Payloads))
			}
			for i, payload := range test.expect {
				assert.Equal(t, rsp.Payloads[i], payload)
			}
		})
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_SyncFuzzTag(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(""))
	})
	target := utils.HostPort(host, port)
	for _, test := range []struct {
		tag       string
		expect    [][]string
		params    map[string]string
		syncIndex bool
	}{
		{ // 同步
			tag: "{{array(1|2|3)}}{{array(1|2|3)}}",
			expect: [][]string{
				{
					"1", "1",
				},
				{
					"2", "2",
				},
				{
					"3", "3",
				},
			},
			syncIndex: true,
		},
		{ // 笛卡尔
			tag: "{{array(1|2)}}{{array(1|2)}}",
			expect: [][]string{
				{
					"1", "1",
				},
				{
					"2", "1",
				},
				{
					"1", "2",
				},
				{
					"2", "2",
				},
			},
			syncIndex: false,
		},
		{ // 设置变量
			tag: "{{p(a)}}{{p(b)}}",
			params: map[string]string{
				"a": "{{array(1|2)}}",
				"b": "{{array(1|2)}}",
			},
			expect: [][]string{
				{
					"1", "1",
				},
				{
					"2", "2",
				},
			},
			syncIndex: true,
		},
	} {
		req := &ypb.FuzzerRequest{
			Request: "GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n" + test.tag,
		}
		req.ForceFuzz = true
		req.FuzzTagSyncIndex = test.syncIndex
		req.Concurrent = 1
		for k, v := range test.params {
			req.Params = append(req.Params, &ypb.FuzzerParamItem{
				Key:   k,
				Value: v,
				Type:  "fuzztag",
			})
		}
		recv, err := client.HTTPFuzzer(utils.TimeoutContextSeconds(10), req)
		if err != nil {
			t.Fatal(err)
		}
		sortStringSlice := func(d []string) {
			sort.Slice(d, func(i, j int) bool {
				return d[i] < d[j]
			})
		}
		if len(test.params) != 0 { // params 变量的渲染结果不是幂等的
			expect := []string{}
			for _, v := range test.expect {
				sortStringSlice(v)
				expect = append(expect, strings.Join(v, ""))
			}
			sortStringSlice(expect)
			expectStr := strings.Join(expect, "")
			payloads := []string{}
			for i := 0; i < len(test.expect); i++ {
				rsp, err := recv.Recv()
				if err != nil {
					t.Fatal(err)
				}
				sortStringSlice(rsp.Payloads)
				payloads = append(payloads, strings.Join(rsp.Payloads, ""))
			}
			sortStringSlice(payloads)
			assert.Equal(t, expectStr, strings.Join(payloads, ""))
		} else {
			for _, expectPayload := range test.expect {
				rsp, err := recv.Recv()
				if err != nil {
					t.Fatal(err)
				}
				if len(rsp.Payloads) != len(expectPayload) {
					t.Fatalf("expect length %v, got %v", len(test.expect), len(rsp.Payloads))
				}
				for i, payload := range expectPayload {
					if rsp.Payloads[i] != payload {
						t.Fatalf("expect %v, got %v", payload, rsp.Payloads[i])
					}
				}
			}
		}

	}
}

func TestFuzzerBigRequest(t *testing.T) {
	uid := uuid.New().String()

	host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\n" +
		"Content-Length: 0\r\n\r\n"))
	target := utils.HostPort(host, port)
	origin := []byte(`GET /` + uid + ` HTTP/1.1
Host: ` + target + `
Content-Type: multipart/form-data; boundary=X-INSOMNIA-BOUNDARY

--X-INSOMNIA-BOUNDARY
Content-Disposition: form-data; name=""

` + strings.Repeat("\x99", 11000000) /* 11,000,000 B ~ 11M */ + `
--X-INSOMNIA-BOUNDARY
Content-Disposition: form-data; name=""; filename="small.jpg"
Content-Type: image/jpeg

11
`)
	client, _ := NewLocalClient()
	stream, _ := client.MITM(context.Background())
	portMITM := int64(utils.GetRandomAvailableTCPPort())
	stream.Send(&ypb.MITMRequest{
		Host: "127.0.0.1",
		Port: uint32(portMITM),
	})
	go func() {
		for {
			_, err := stream.Recv()
			if err != nil {
				break
			}
		}
	}()
	err := utils.WaitConnect("127.0.0.1:"+fmt.Sprint(portMITM), 5)
	if err != nil {
		t.Fatal(err)
	}
	_, reqRaw, _ := poc.HTTP(origin, poc.WithProxy("http://127.0.0.1:"+fmt.Sprint(portMITM)))
	if len(reqRaw) < 11000000 {
		t.Fatal("response raw is too small")
	}
	var rsp *ypb.QueryHTTPFlowResponse
	for i := 0; i < 20; i++ {
		rsp, _ = client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
			SourceType: "mitm",
			SearchURL:  uid,
		})
		if rsp == nil || len(rsp.GetData()) <= 0 {
			time.Sleep(time.Second)
		} else {
			break
		}
	}

	reqId := rsp.GetData()[0].Id
	flow, err := client.GetHTTPFlowById(context.Background(), &ypb.GetHTTPFlowByIdRequest{Id: int64(reqId)})
	if err != nil {
		t.Fatal(err)
	}
	if len(flow.Request) < 1000 {
		t.Fatal("request too small")
	}
	suffix := flow.Request[len(flow.Request)-200:]

	spew.Dump(suffix)
	if len(flow.Request) < 11000000 {
		t.Fatal("request is too small, truncated some reason got: " + utils.ByteSize(uint64(len(flow.Request))))
	}
}

func TestHTTPRequest_Fuzz_FromPlugin(t *testing.T) {
	t.Run("Request", func(t *testing.T) {
		pluginName := utils.RandStringBytes(10)
		server, port := utils.DebugMockHTTP([]byte(`HTTP/1.1 200 OK

aaa`))
		runtimeId := uuid.New().String()
		req, err := mutate.NewFuzzHTTPRequest(fmt.Sprintf(`
GET / HTTP/1.1
Host: %s:%d

`, server, port), mutate.OptFromPlugin(pluginName), mutate.OptRuntimeId(runtimeId))
		require.NoError(t, err)
		_, err = req.ExecFirst()
		require.NoError(t, err)
		var httpFlows []*schema.HTTPFlow
		db := bizhelper.ExactQueryString(consts.GetGormProjectDatabase(), "runtime_id", runtimeId)
		resDb := db.Find(&httpFlows)
		require.NoError(t, resDb.Error)
		for _, flow := range httpFlows {
			require.Equal(t, pluginName, flow.FromPlugin, "fuzz request form plugin not match")
		}
	})

	t.Run("BatchRequest", func(t *testing.T) {
		pluginName := utils.RandStringBytes(10)
		server, port := utils.DebugMockHTTP([]byte(`HTTP/1.1 200 OK

aaa`))
		runtimeId := uuid.New().String()
		req, err := mutate.NewFuzzHTTPRequest(fmt.Sprintf(`
GET /?a=123&a=46&b=123 HTTP/1.1
Host: %s:%d

{"abc": "123", "a": 123}
`, server, port), mutate.OptFromPlugin(pluginName), mutate.OptRuntimeId(runtimeId))
		require.NoError(t, err)
		params := req.GetCommonParams()

		for _, p := range params {
			_, err := p.Fuzz("test").ExecFirst()
			require.NoError(t, err)
		}
		var httpFlows []*schema.HTTPFlow
		db := bizhelper.ExactQueryString(consts.GetGormProjectDatabase(), "runtime_id", runtimeId)
		resDb := db.Find(&httpFlows)
		require.NoError(t, resDb.Error)
		for _, flow := range httpFlows {
			require.Equal(t, pluginName, flow.FromPlugin, "fuzz batch request form plugin not match")
		}
	})

}

func TestWebFuzzerAutoFixHeaderFlag(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte(`HTTP/1.1 200 OK
Content-Type: text/html
Content-Disposition: attachment; filename="example.pdf"
X-Content-Type-Options: nosniff

%PDF-1.4
%âãÏÓ
%%EOF`))
	client, err := NewLocalClient()
	require.NoError(t, err)
	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:%d", host, port), nil)
	require.NoError(t, err)
	dumpRequest, err := httputil.DumpRequest(req, false)
	require.NoError(t, err)
	fuzzer, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		RequestRaw: dumpRequest,
	})
	require.NoError(t, err)
	flag := false
	var (
		originContentType       string
		fixedContentType        string
		isSetContentTypeOptions bool
	)
	for {
		recv, err := fuzzer.Recv()
		if err != nil {
			break
		}
		if recv.IsAutoFixContentType {
			originContentType = recv.OriginalContentType
			fixedContentType = recv.FixContentType
			isSetContentTypeOptions = recv.IsSetContentTypeOptions
			flag = true
			break
		}
	}
	require.True(t, flag)
	require.True(t, "text/html" == originContentType)
	require.True(t, "application/pdf" == fixedContentType)
	require.True(t, isSetContentTypeOptions)
}

func TestWebFuzzerNoReadMultiResp(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte(`HTTP/1.1 200 OK
Content-Type: text/html
Content-Length: 1

2HTTP/1.1 200 OK
Content-Type: text/html
Content-Length: 10000000` + "\r\n\r\n" + strings.Repeat("a", 10*1024)))
	client, err := NewLocalClient()
	require.NoError(t, err)
	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:%d", host, port), nil)
	require.NoError(t, err)
	dumpRequest, err := httputil.DumpRequest(req, false)
	require.NoError(t, err)
	fuzzer, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		RequestRaw:         dumpRequest,
		NoFixContentLength: true,
		DisableUseConnPool: true,
	})
	require.NoError(t, err)
	for {
		recv, err := fuzzer.Recv()
		if err != nil {
			break
		}
		// lowhttp bufio read 4096 bytes, so the response raw should be less than 4096+100 bytes
		require.GreaterOrEqual(t, len(recv.ResponseRaw), 10*1024, "response raw is too large, got: "+utils.ByteSize(uint64(len(recv.ResponseRaw))))
	}

	fuzzer, err = client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		RequestRaw:          dumpRequest,
		NoFixContentLength:  true,
		DisableUseConnPool:  true,
		NoReadMultiResponse: true,
	})
	require.NoError(t, err)
	for {
		recv, err := fuzzer.Recv()
		if err != nil {
			break
		}
		// lowhttp bufio read 4096 bytes, so the response raw should be less than 4096+100 bytes
		require.Less(t, len(recv.ResponseRaw), 4096+100, "response raw is too large, got: "+utils.ByteSize(uint64(len(recv.ResponseRaw))))
	}
}

func TestCancelStringFuzzer(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	flagKey := uuid.New().String()
	flagValue := uuid.New().String()

	ctx, cancel := context.WithCancel(context.Background())

	var cancelTime time.Time
	var cancelDuration time.Duration
	fuzzTaskEnd := make(chan struct{})
	go func() {
		defer func() {
			if !cancelTime.IsZero() {
				cancelDuration = time.Since(cancelTime)
			} else {
				cancelDuration = 0
			}
			yakit.DelKey(consts.GetGormProfileDatabase(), flagKey)
			close(fuzzTaskEnd)
		}()
		_, err := client.StringFuzzer(ctx, &ypb.StringFuzzerRequest{
			Template: "{{yak(handle)}}",
			Limit:    100000000,
			HotPatchCode: `
	handle = s => {
		db.SetKey("` + flagKey + `", "` + flagValue + `")
		for {
			sleep(1)
			println("hot patch code is running")
		}
	}
	`,
		})
		assert.Contains(t, err.Error(), "context canceled")
	}()

	timeout := 10 * time.Second
	after := time.After(timeout)

	db := consts.GetGormProfileDatabase()
	if db == nil {
		t.Fatal("database is nil")
	}

LOOP:
	for {
		select {
		case <-after:
			t.Fatal("timeout")
		default:
			value := yakit.GetKey(db, flagKey)
			if value == flagValue {
				cancelTime = time.Now()
				cancel()
				break LOOP
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	<-fuzzTaskEnd
	log.Infof("cancel duration: %s", cancelDuration.String())
	require.Less(t, cancelDuration, 10*time.Second)
}
