package yakgrpc

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_HTTPFuzzer_FuzzerSequence(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	var (
		redirect302done = false
		token           = utils.RandStringBytes(32)
		verified        = false
	)
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.RequestURI {
		case "/verify":
			if request.Header.Get("Authorization") == "Bearer "+token {
				verified = true
			}

		case "/abc":
			redirect302done = true
			writer.Write([]byte(`{"key": "` + token + `"}`))
			return
		case "/":
			writer.Header().Set("Location", "/abc")
			writer.WriteHeader(302)
			writer.Write([]byte("HELLO HTTP2"))
			return
		}
		writer.Write([]byte("GREAT"))
		return
	})

	client, err := c.HTTPFuzzerSequence(
		utils.TimeoutContextSeconds(10),
		&ypb.FuzzerRequests{Requests: []*ypb.FuzzerRequest{
			{
				Request: string(lowhttp.ReplaceHTTPPacketHeader([]byte(`GET / HTTP/1.1
Host: www.example.com

abc`), "Host", utils.HostPort(host, port))),
				IsHTTPS:                  false,
				PerRequestTimeoutSeconds: 5,
				RedirectTimes:            3,
				Extractors: []*ypb.HTTPResponseExtractor{
					{
						Name:   "test",
						Type:   "json",
						Scope:  "body",
						Groups: []string{".key"},
					},
				},
			},
			{
				Request: string(lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /verify HTTP/1.1
Host: www.example.com
Authorization: Bearer {{params(test)}}

abc`), "Host", utils.HostPort(host, port))),
				IsHTTPS:                  false,
				PerRequestTimeoutSeconds: 5,
				RedirectTimes:            3,
				InheritVariables:         true,
				ForceFuzz:                true,
			},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	for {
		resp, err := client.Recv()
		if err != nil {
			break
		}
		if resp == nil {
			break
		}
		_ = string(resp.Response.RequestRaw)
	}

	if !redirect302done {
		t.Fatal("redirect302done")
	}

	if !verified {
		t.Fatal("verified extractor ")
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_FuzzerSequence_InheritKey(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	token := utils.RandStringBytes(32)
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("GREAT"))
		return
	})

	client, err := c.HTTPFuzzerSequence(
		utils.TimeoutContextSeconds(10),
		&ypb.FuzzerRequests{Requests: []*ypb.FuzzerRequest{
			{
				Request: string(lowhttp.ReplaceHTTPPacketHeader([]byte(`GET / HTTP/1.1
Host: www.example.com

{{p(a)}}`), "Host", utils.HostPort(host, port))),
				IsHTTPS:                  false,
				PerRequestTimeoutSeconds: 5,
				RedirectTimes:            3,
				ForceFuzz:                true,
				Params: []*ypb.FuzzerParamItem{
					{
						Key:   "a",
						Value: "{{int(1-10)}}",
						Type:  "fuzztag",
					},
				},
			},
			{
				Request: string(lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /verify HTTP/1.1
Host: www.example.com
Authorization: Bearer {{params(test)}}

`+token+`_`+"{{p(a)}}"), "Host", utils.HostPort(host, port))),
				IsHTTPS:                  false,
				PerRequestTimeoutSeconds: 5,
				RedirectTimes:            3,
				InheritVariables:         true,
				ForceFuzz:                true,
			},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	for {
		resp, err := client.Recv()
		if err != nil {
			break
		}
		if resp == nil {
			break
		}
		count++
		if strings.Contains(string(resp.Response.RequestRaw), token+`_-9`) {
			// fmt.Println(string(resp.Response.RequestRaw))
			t.Fatalf("fuzztag variables passed failed. request:\n%s", string(resp.Response.RequestRaw))
		}
	}
	t.Logf("FETCH COUNT: %v", count)
	if count != 20 {
		t.Fatal("not 20 request")
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_FuzzerSequence_FuzzerWithTag(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(`{"path":` + strconv.Quote(request.URL.Path) + `}`))
		return
	})

	client, err := c.HTTPFuzzerSequence(
		utils.TimeoutContextSeconds(10),
		&ypb.FuzzerRequests{
			Concurrent: 1,
			Requests: []*ypb.FuzzerRequest{
				{
					Request: string(lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /aa={{int(1-10)}} HTTP/1.1
Host: www.example.com

abc`), "Host", utils.HostPort(host, port))),
					IsHTTPS:                  false,
					PerRequestTimeoutSeconds: 5,
					RedirectTimes:            3,
					ForceFuzz:                true,
					Extractors: []*ypb.HTTPResponseExtractor{
						{
							Name:   "test",
							Type:   "json",
							Scope:  "body",
							Groups: []string{`.path`},
						},
					},
				},
				{
					Request: string(lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /verify?a={{param(test)}}/{{int(1-10)}} HTTP/1.1
Host: www.example.com
Authorization: Bearer {{params(test)}}

abc`), "Host", utils.HostPort(host, port))),
					IsHTTPS:                  false,
					PerRequestTimeoutSeconds: 5,
					RedirectTimes:            3,
					InheritVariables:         true,
					ForceFuzz:                true,
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for {
		resp, err := client.Recv()
		if err != nil {
			break
		}
		if resp == nil {
			break
		}
		count++
	}
	if count != 100+10 {
		t.Fatal("Fuzztag COUNT: " + fmt.Sprint(count) + " failed")
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_FuzzerSequence_FuzzerWithTag2(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(`{"path":` + strconv.Quote(request.URL.Path) + `}`))
		return
	})

	client, err := c.HTTPFuzzerSequence(
		utils.TimeoutContextSeconds(1000),
		&ypb.FuzzerRequests{
			Concurrent: 1,
			Requests: []*ypb.FuzzerRequest{
				{
					FuzzerIndex: "a",
					Request: string(lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /aa={{int(1-3)}} HTTP/1.1
Host: www.example.com

abc`), "Host", utils.HostPort(host, port))),
					IsHTTPS:                  false,
					PerRequestTimeoutSeconds: 5,
					RedirectTimes:            3,
					ForceFuzz:                true,
					Extractors: []*ypb.HTTPResponseExtractor{
						{
							Name:   "test",
							Type:   "json",
							Scope:  "body",
							Groups: []string{`.path`},
						},
					},
				},
				{
					Request: string(lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /verify?a={{param(test)}}/{{int(1-3)}} HTTP/1.1
Host: www.example.com
Authorization: Bearer {{params(test)}}

abc`), "Host", utils.HostPort(host, port))),
					FuzzerIndex:              "b",
					IsHTTPS:                  false,
					PerRequestTimeoutSeconds: 5,
					RedirectTimes:            3,
					InheritVariables:         true,
					ForceFuzz:                true,
				},
				{
					Request: string(lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /verify?a={{param(test)}}/{{int(1-3)}} HTTP/1.1
Host: www.example.com
Authorization: Bearer {{params(test)}}

abc`), "Host", utils.HostPort(host, port))),
					IsHTTPS:                  false,
					PerRequestTimeoutSeconds: 5,
					FuzzerIndex:              "c",
					RedirectTimes:            3,
					InheritVariables:         true,
					ForceFuzz:                true,
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	var a int
	var b int
	var cCount int
	for {
		resp, err := client.Recv()
		if err != nil {
			break
		}
		if resp == nil {
			break
		}
		switch resp.Request.GetFuzzerIndex() {
		case "a":
			a++
		case "b":
			b++
		case "c":
			cCount++
		}
	}
	t.Logf("\n\nA: %v\nB: %v \nC: %v", a, b, cCount)
	if a != 3 {
		t.Fatal("A failed")
	}
	if b != 9 {
		t.Fatal("B failed")
	}
	if cCount != 27 {
		t.Fatal("C failed")
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_FuzzerSequence_FuzzerWithTag3(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(`{"path":` + strconv.Quote(request.URL.Path) + `}`))
		return
	})

	client, err := c.HTTPFuzzerSequence(
		utils.TimeoutContextSeconds(1000),
		&ypb.FuzzerRequests{
			Concurrent: 1,
			Requests: []*ypb.FuzzerRequest{
				{
					FuzzerIndex: "a",
					Request: string(lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /aa={{int(3)}} HTTP/1.1
Host: www.example.com

abc`), "Host", utils.HostPort(host, port))),
					IsHTTPS:                  false,
					PerRequestTimeoutSeconds: 5,
					RedirectTimes:            3,
					ForceFuzz:                true,
					Extractors: []*ypb.HTTPResponseExtractor{
						{
							Name:   "test",
							Type:   "json",
							Scope:  "body",
							Groups: []string{`.path`},
						},
						{
							Name:   "test1",
							Type:   "nuclei-dsl",
							Scope:  "body",
							Groups: []string{`"abc" + "111"`},
						},
						{
							Name:   "test2",
							Type:   "nuclei-dsl",
							Scope:  "body",
							Groups: []string{`"abc" + "111" + test`},
						},
					},
				},
			},
		},
	)
	if err != nil {
		panic(err)
	}
	var haveResult bool
	for {
		resp, err := client.Recv()
		if err != nil {
			log.Error(err)
			break
		}
		if resp == nil {
			break
		}
		haveResult = true

		results := resp.Response.GetExtractedResults()
		cond1 := results[1].Key == "test1" && results[1].Value == "abc111"
		cond2 := results[2].Key == "test2" && results[2].Value == results[1].Value+results[0].Value

		if cond1 && cond2 {
			t.Log("success")
		} else {
			t.Fatal("error for extractor order")
		}
	}

	if !haveResult {
		t.Fatal("no result response")
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_FuzzerSequence_FuzzerTagWithConcurrent(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		time.Sleep(time.Millisecond * 500)
		writer.Write([]byte(`{"path":` + strconv.Quote(request.URL.Path) + `}`))
		return
	})

	start := time.Now()
	client, err := c.HTTPFuzzerSequence(
		utils.TimeoutContextSeconds(10),
		&ypb.FuzzerRequests{
			Concurrent: 1,
			Requests: []*ypb.FuzzerRequest{
				{
					Request: string(lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /aa={{int(1-10)}} HTTP/1.1
Host: www.example.com

abc`), "Host", utils.HostPort(host, port))),
					IsHTTPS:                  false,
					PerRequestTimeoutSeconds: 5,
					RedirectTimes:            3,
					ForceFuzz:                true,
					Extractors: []*ypb.HTTPResponseExtractor{
						{
							Name:   "test",
							Type:   "json",
							Scope:  "body",
							Groups: []string{`.path`},
						},
					},
				},
				{
					Request: string(lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /verify?a={{param(test)}}/{{int(1-10)}} HTTP/1.1
Host: www.example.com
Authorization: Bearer {{params(test)}}

abc`), "Host", utils.HostPort(host, port))),
					IsHTTPS:                  false,
					PerRequestTimeoutSeconds: 5,
					RedirectTimes:            3,
					InheritVariables:         true,
					ForceFuzz:                true,
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for {
		resp, err := client.Recv()
		if err != nil {
			log.Error(err)
			break
		}
		if resp == nil {
			break
		}
		count++
	}
	if count != 100+10 {
		t.Fatal("Fuzztag COUNT: " + fmt.Sprint(count) + " failed")
	}
	if time.Now().Sub(start).Seconds() <= 5 {
		t.Fatal("concurrent(flowmax) is not working")
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_FuzzerSequence_InheritCookie(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	var (
		redirect302done = false
		token           = utils.RandStringBytes(32)
		verified        = false
	)

	token2 := utils.RandStringBytes(100)
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		raw, _ := utils.HttpDumpWithBody(request, true)

		switch request.URL.Path {
		case "/verify":
			if request.Header.Get("Authorization") == "Bearer "+token {
				if lowhttp.GetHTTPPacketCookie(raw, "test") == token2 {
					verified = true
				}
			}

		case "/abc":
			redirect302done = true
			if lowhttp.GetHTTPPacketCookie(raw, "test") == token2 {
				writer.Write([]byte(`{"key": "` + token + `"}`))
			}
			return
		case "/":
			writer.Header().Set("Location", "/abc")
			http.SetCookie(writer, &http.Cookie{
				Name:  "test",
				Value: token2,
			})
			writer.WriteHeader(302)
			writer.Write([]byte("HELLO HTTP2"))
			return
		}
		writer.Write([]byte("GREAT"))
		return
	})

	client, err := c.HTTPFuzzerSequence(
		utils.TimeoutContextSeconds(10),
		&ypb.FuzzerRequests{Requests: []*ypb.FuzzerRequest{
			{
				Request: string(lowhttp.ReplaceHTTPPacketHeader([]byte(`GET / HTTP/1.1
Host: www.example.com

abc`), "Host", utils.HostPort(host, port))),
				IsHTTPS:                  false,
				PerRequestTimeoutSeconds: 5,
				RedirectTimes:            3,
				Extractors: []*ypb.HTTPResponseExtractor{
					{
						Name:   "test",
						Type:   "json",
						Scope:  "body",
						Groups: []string{".key"},
					},
				},
				FuzzerIndex: "1",
			},
			{
				Request: string(lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /verify HTTP/1.1
Host: www.example.com
Authorization: Bearer {{params(test)}}

abc`), "Host", utils.HostPort(host, port))),
				IsHTTPS:                  false,
				PerRequestTimeoutSeconds: 5,
				RedirectTimes:            3,
				InheritVariables:         true,
				InheritCookies:           true,
				ForceFuzz:                true,
				FuzzerIndex:              "2",
			},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}

	checkFuzzerIndex := false
	checkFuzzerIndex2 := false
	for {
		resp, err := client.Recv()
		if err != nil {
			break
		}
		if resp == nil {
			break
		}
		if resp.Request.GetFuzzerIndex() == "1" && resp.Request.GetRequest() == "" && len(resp.Request.GetRequestRaw()) <= 0 {
			checkFuzzerIndex = true
		}
		if resp.Request.GetFuzzerIndex() == "2" && resp.Request.GetRequest() == "" && len(resp.Request.GetRequestRaw()) <= 0 {
			checkFuzzerIndex2 = true
		}
	}

	if !redirect302done {
		t.Fatal("redirect302done")
	}

	if !verified {
		t.Fatal("verified extractor ")
	}

	if !checkFuzzerIndex {
		t.Fatal("checkFuzzerIndex failed")
	}

	if !checkFuzzerIndex2 {
		t.Fatal("checkFuzzerIndex2 failed")
	}
}
