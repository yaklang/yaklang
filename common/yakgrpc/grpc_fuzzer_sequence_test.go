package yakgrpc

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"testing"
)

func TestGRPCMUSTPASS_FuzzerSequence(t *testing.T) {
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
		panic(err)
	}
	for {
		resp, err := client.Recv()
		if err != nil {
			break
		}
		if resp == nil {
			break
		}
		println(string(resp.Response.RequestRaw))
	}

	if !redirect302done {
		t.Fatal("redirect302done")
	}

	if !verified {
		t.Fatal("verified extractor ")
	}
}
