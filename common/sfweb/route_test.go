package sfweb_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/yaklang/yaklang/common/sfweb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

var (
	serverUrl  string
	serverAddr string
)

func init() {
	var err error
	port := utils.GetRandomAvailableTCPPort()
	serverUrl, err = sfweb.NewSyntaxFlowWebServer(
		context.Background(),
		sfweb.WithHost("127.0.0.1"),
		sfweb.WithPort(port), sfweb.WithHttps(false), sfweb.WithDebug(true), sfweb.WithChatGLMAPIKey(os.Getenv(sfweb.CHAT_GLM_API_KEY)))
	if err != nil {
		panic(err)
	}
	serverAddr = utils.ExtractHostPort(serverUrl)
}

func debug() {
	sfweb.SfWebLogger.SetLevel("debug")
}

func DoResponse(method, path string, data any, opts ...poc.PocConfigOption) (*lowhttp.LowhttpResponse, error) {
	rsp, _, err := poc.Do(method, fmt.Sprintf("%s%s", serverUrl, path), opts...)
	if err != nil {
		return rsp, err
	}
	body := rsp.GetBody()
	err = json.Unmarshal(body, data)
	if err != nil {
		return rsp, utils.Wrap(err, "unmarshal json error")
	}
	return rsp, nil
}
