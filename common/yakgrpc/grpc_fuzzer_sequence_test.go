package yakgrpc

import (
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/cartesian"
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

	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("GREAT"))
		return
	})

	client, err := c.HTTPFuzzerSequence(
		utils.TimeoutContextSeconds(10),
		&ypb.FuzzerRequests{Requests: []*ypb.FuzzerRequest{
			{
				Request: fmt.Sprintf(`GET / HTTP/1.1
Host: %s

{{p(a)}}`, utils.HostPort(host, port)),
				IsHTTPS:                  false,
				PerRequestTimeoutSeconds: 5,
				RedirectTimes:            3,
				ForceFuzz:                true,
				Params: []*ypb.FuzzerParamItem{
					{
						Key:   "a",
						Value: "{{int(1-2)}}",
						Type:  "fuzztag",
					},
				},
			},
			{
				Request: fmt.Sprintf(`GET /verify HTTP/1.1
Host: %s

{{p(b)}}`, utils.HostPort(host, port)),
				IsHTTPS:                  false,
				PerRequestTimeoutSeconds: 5,
				RedirectTimes:            3,
				InheritVariables:         true,
				ForceFuzz:                true,
				Params: []*ypb.FuzzerParamItem{
					{
						Key:   "b",
						Value: "{{p(a)}}",
						Type:  "fuzztag",
					},
				},
			},
			{
				Request: fmt.Sprintf(`GET /verify2 HTTP/1.1
Host: %s

{{p(c)}}`, utils.HostPort(host, port)),
				IsHTTPS:                  false,
				PerRequestTimeoutSeconds: 5,
				RedirectTimes:            3,
				InheritVariables:         true,
				ForceFuzz:                true,
				Params: []*ypb.FuzzerParamItem{
					{
						Key:   "c",
						Value: "{{a+b}}",
						Type:  "nuclei-dsl",
					},
				},
			},
		}},
	)
	require.NoError(t, err)
	firstRequestParams := make([]string, 0)
	secondRequestParams := make([]string, 0)

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
		body := lowhttp.GetHTTPPacketBody(resp.Response.RequestRaw)
		req, err := lowhttp.ParseBytesToHttpRequest(resp.Response.RequestRaw)
		require.NoError(t, err)
		require.NotNil(t, req.URL)
		if req.URL.Path == "/" {
			firstRequestParams = append(firstRequestParams, string(body))
		} else if req.URL.Path == "/verify" {
			secondRequestParams = append(secondRequestParams, string(body))

			verifyParams := lo.Map(firstRequestParams, func(item string, index int) string {
				return item
			})
			require.Contains(t, verifyParams, string(body))
		} else if req.URL.Path == "/verify2" {
			params, err := cartesian.Product([][]string{firstRequestParams, secondRequestParams})
			require.NoError(t, err)
			verifyParams := lo.Map(params, func(item []string, index int) string {
				return item[0] + item[1]
			})
			require.Contains(t, verifyParams, string(body))
		}
	}
	require.Equal(t, 3+3, count, "count failed")
}

func TestGRPCMUSTPASS_HTTPFuzzer_FuzzerSequence_InheritKeyWithType(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("GREAT"))
		return
	})

	client, err := c.HTTPFuzzerSequence(
		utils.TimeoutContextSeconds(10000000),
		&ypb.FuzzerRequests{Requests: []*ypb.FuzzerRequest{
			{
				Request: fmt.Sprintf(`GET / HTTP/1.1
Host: %s

{{p(a)}}`, utils.HostPort(host, port)),
				IsHTTPS:                  false,
				PerRequestTimeoutSeconds: 5,
				RedirectTimes:            3,
				ForceFuzz:                true,
				Params: []*ypb.FuzzerParamItem{
					{
						Key:   "a",
						Value: "1.1",
						Type:  "nuclei-dsl",
					},
				},
			},
			{
				Request: fmt.Sprintf(`GET /verify HTTP/1.1
Host: %s

{{p(b)}}`, utils.HostPort(host, port)),
				IsHTTPS:                  false,
				PerRequestTimeoutSeconds: 5,
				RedirectTimes:            3,
				InheritVariables:         true,
				ForceFuzz:                true,
				Params: []*ypb.FuzzerParamItem{
					{
						Key:   "b",
						Value: "{{a+1}}",
						Type:  "nuclei-dsl",
					},
				},
			},
			{
				Request: fmt.Sprintf(`GET /verify2 HTTP/1.1
Host: %s

{{p(c)}}`, utils.HostPort(host, port)),
				IsHTTPS:                  false,
				PerRequestTimeoutSeconds: 5,
				RedirectTimes:            3,
				InheritVariables:         true,
				ForceFuzz:                true,
				Params: []*ypb.FuzzerParamItem{
					{
						Key:   "c",
						Value: "{{b+1}}",
						Type:  "nuclei-dsl",
					},
				},
			},
		}},
	)
	require.NoError(t, err)

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
		body := lowhttp.GetHTTPPacketBody(resp.Response.RequestRaw)
		req, err := lowhttp.ParseBytesToHttpRequest(resp.Response.RequestRaw)
		require.NoError(t, err)
		require.NotNil(t, req.URL)
		if req.URL.Path == "/verify" {
			require.Equal(t, "2.1", string(body))
		} else if req.URL.Path == "/verify2" {
			require.Equal(t, "3.1", string(body))
		}
	}
	require.Equal(t, 1+1+1, count, "count failed")
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

func TestGRPCMUSTPASS_HTTPFuzzer_FuzzerSequence_Extractor_OnlyOneResult(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	token := utils.RandStringBytes(32)

	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(fmt.Sprintf(`{"a":"%s"}`, token)))
		return
	})

	client, err := c.HTTPFuzzerSequence(
		utils.TimeoutContextSeconds(10000000),
		&ypb.FuzzerRequests{Requests: []*ypb.FuzzerRequest{
			{
				Request: fmt.Sprintf(`GET / HTTP/1.1
Host: %s
`, utils.HostPort(host, port)),
				IsHTTPS:                  false,
				PerRequestTimeoutSeconds: 5,
				RedirectTimes:            3,
				ForceFuzz:                true,
				Extractors: []*ypb.HTTPResponseExtractor{
					{
						Name:   "test",
						Type:   "json",
						Scope:  "body",
						Groups: []string{".a"},
					},
				},
			},
			{
				Request: fmt.Sprintf(`GET /verify HTTP/1.1
Host: %s

{{p(b)}}`, utils.HostPort(host, port)),
				IsHTTPS:                  false,
				PerRequestTimeoutSeconds: 5,
				RedirectTimes:            3,
				InheritVariables:         true,
				ForceFuzz:                true,
				Params: []*ypb.FuzzerParamItem{
					{
						Key:   "b",
						Value: `{{replace(test,"","")}}`,
						Type:  "nuclei-dsl",
					},
				},
			},
		}},
	)
	require.NoError(t, err)

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
		body := lowhttp.GetHTTPPacketBody(resp.Response.RequestRaw)
		req, err := lowhttp.ParseBytesToHttpRequest(resp.Response.RequestRaw)
		require.NoError(t, err)
		require.NotNil(t, req.URL)
		if req.URL.Path == "/verify" {
			require.Equal(t, token, string(body))
			require.NotContains(t, string(body), "[")
			require.NotContains(t, string(body), "]")
		}
	}
	require.Equal(t, 1+1, count, "count failed")
}
