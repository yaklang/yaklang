package yakgrpc

import (
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	filter2 "github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"strings"
	"testing"
	"time"
)

func init() {
	yakit.InitialDatabase()
}

func TestGRPCMUSTPASS_HTTPFuzzerWITHPLUGIN(t *testing.T) {
	var token string
	name, err := httptpl.MockEchoPlugin(func(s string) {
		token = s
	})
	if err != nil {
		panic(err)
	}

	host, port := utils.DebugMockHTTP([]byte(`HTTP/1.1 200 OK
Content-Length: 12

{"abc": "111111", "qqq": "12"}`))

	c, err := NewLocalClient()
	if err != nil {
		panic(err)
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
		panic(err)
	}

	haveToken := false
	for {
		rsp, err := client.Recv()
		if err != nil {
			log.Error(err)
			break
		}
		if strings.Contains(rsp.Url, token) {
			haveToken = true
		}
		fmt.Println(rsp.Url)
		fmt.Printf("%v: %v\n", rsp.GetUUID(), len(rsp.ResponseRaw))
		fmt.Println(string(rsp.GetRequestRaw()))
		spew.Dump(rsp.GetExtractedResults())
		spew.Dump(rsp.GetMatchedByMatcher())
	}

	if !haveToken {
		t.Log("NO TOKEN FOUND, PLUGIN is not executed!")
		t.FailNow()
	}
}

func TestGRPCMUSTPASS_HTTPFuzzerBIG(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte(`HTTP/1.1 200 OK
Content-Length: 12

{"abc": "111111", "qqq": "12"}`))

	c, err := NewLocalClient()
	if err != nil {
		panic(err)
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
		panic(err)
	}

	for {
		rsp, err := client.Recv()
		if err != nil {
			log.Error(err)
			break
		}
		fmt.Printf("%v: %v\n", rsp.GetUUID(), len(rsp.ResponseRaw))
		fmt.Println(string(rsp.GetRequestRaw()))
		spew.Dump(rsp.GetExtractedResults())
		spew.Dump(rsp.GetMatchedByMatcher())
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer(t *testing.T) {
	var requestedCount int
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestedCount++
		writer.Write([]byte("abc"))
	})

	c, err := NewLocalClient()
	if err != nil {
		panic(err)
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
		panic(err)
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
		panic(err)
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

func TestServer_HTTPFuzzerS2008(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		panic(err)
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
		panic(err)
	}

	for {
		rsp, err := client.Recv()
		if err != nil {
			break
		}
		spew.Dump(rsp)
	}
}

func TestServer_HTTPFuzzer2(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		HistoryWebFuzzerId: 6,
	})
	if err != nil {
		panic(err)
	}

	for {
		rsp, err := client.Recv()
		if err != nil {
			break
		}
		spew.Dump(rsp)
	}
}

func TestServer_HTTPFuzzer3(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	client, err := c.QueryHistoryHTTPFuzzerTask(context.Background(), &ypb.Empty{})
	if err != nil {
		panic(err)
	}

	spew.Dump(client)
}

func TestServer_HTTPFuzzerYYOA(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	client, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request: `GET /yyoa/DownExcelBeanServlet?contenttype=username&contentvalue=&state=1&per_id=0 HTTP/1.1
Host: 14.157.105.194:5002
Pragma: no-cache
Cache-Control: no-cache
DNT: 1
Upgrade-Insecure-Requests: 1
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/100.0.4896.88 Safari/537.36
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9
Referer: http://222.133.23.90:9000/yyoa/ext/https/getSessionList.jsp?cmd=getAll
Accept-Encoding: gzip, deflate
Accept-Language: zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7
Cookie: JSESSIONID=9A2AF446D35187ECF84CBE9B1254B0EE
sec-gpc: 1
Connection: close

`,
		Concurrent:               10,
		IsHTTPS:                  false,
		ForceFuzz:                true,
		PerRequestTimeoutSeconds: 5,
	})
	if err != nil {
		panic(err)
	}

	for {
		rsp, err := client.Recv()
		if err != nil {
			break
		}
		spew.Dump(rsp)
	}
}

func TestServer_HTTPRequestMutateWithoutConnection(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		panic(err)
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
		panic(err)
	}
	fmt.Println(string(r.Result))
}

func TestServer_HTTPRequestMutateWithConnection(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		panic(err)
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
		panic(err)
	}
	fmt.Println(string(r.Result))
}

func TestGRPCMUSTPASS_HTTPFuzzer_FuzztagVars(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		panic(err)
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
`})
	if err != nil {
		panic(err)
	}

	var count = 0
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
	if count != 30 {
		panic("expect 30, got " + fmt.Sprint(count))
	}

	if ret := time.Since(start); ret.Seconds() > 5 && ret.Seconds() < 6 {
		t.Log("time cost [" + ret.String() + "] is expected")
	} else {
		t.Fatalf("time cost is not expected: %v", ret)
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_Matcher(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		panic(err)
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
`})
	if err != nil {
		panic(err)
	}

	matched := false
	for {
		rsp, err := client.Recv()
		if err != nil {
			log.Error(err)
			break
		}

		matched = rsp.MatchedByMatcher
		fmt.Printf("%v: %v\n", rsp.GetUUID(), len(rsp.ResponseRaw))
		fmt.Println(string(rsp.GetRequestRaw()))
	}

	if !matched {
		t.Log("NO MATCHED")
		t.FailNow()
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_Extractor_Kv(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		panic(err)
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
`})
	if err != nil {
		panic(err)
	}

	matchedCount := 0
	for {
		rsp, err := client.Recv()
		if err != nil {
			log.Error(err)
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
		fmt.Printf("%v: %v\n", rsp.GetUUID(), len(rsp.ResponseRaw))
		fmt.Println(string(rsp.GetRequestRaw()))
	}

	if matchedCount != 3 {
		t.Log("extractor kv failed")
		t.FailNow()
	}
}
