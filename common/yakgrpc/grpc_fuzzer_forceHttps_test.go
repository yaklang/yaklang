package yakgrpc

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

type forceHttpsTestCases struct {
	forceHttps bool
	request    string
	response   string

	status int32
}

func TestForce(t *testing.T) {
	server, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\nConnection: close\r\nContent-Length: 10\r\nHello Http\r\n"))
	mockHTTPS, mockHttpsPort := utils.DebugMockHTTPS([]byte("HTTP/1.1 200 OK\r\nConnection: close\r\nContent-Length: 11\r\nHello Https\r\n"))
	testCase := []forceHttpsTestCases{
		//force https returns location is https
		{
			forceHttps: true,
			request:    fmt.Sprintf("GET / HTTP/1.1\r\nHost: %v:%v", server, port),
			response:   fmt.Sprintf("HTTP/1.1 302 Found\r\nLocation: https://%v:%v\r\n\r\n", mockHTTPS, mockHttpsPort),
			status:     200,
		},

		//is force https but returns location is http use Http
		{
			forceHttps: true,
			request:    fmt.Sprintf("GET / HTTP/1.1\r\nHost: %v:%v", server, port),
			response:   fmt.Sprintf("HTTP/1.1 302 Found\r\nLocation: http://%v:%v\r\n\r\n", server, port),
			status:     200,
		},
		//force https return part location use forceHttps eg. Location: /
		{
			forceHttps: true,
			request:    fmt.Sprintf("GET / HTTP/1.1\r\nHost: %v:%v", mockHTTPS, mockHttpsPort),
			response:   fmt.Sprintf("HTTP/1.1 302 Found\r\nLocation: /\r\n\r\n"),
			status:     200,
		},
	}
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	for _, httpsTestCase := range testCase {
		res, err := client.RedirectRequest(context.Background(), &ypb.RedirectRequestParams{
			IsHttps:  httpsTestCase.forceHttps,
			Request:  httpsTestCase.request,
			Response: httpsTestCase.response,
		})
		if err != nil {
			t.Fatalf("connect remote url fail: %s", err)
		}
		if res.StatusCode != httpsTestCase.status {
			t.Fatalf("response status code match fail, except: %v", httpsTestCase.status)
		}
	}
}
