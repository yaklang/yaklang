package sfweb_test

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/sfweb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

var (
	serverUrl  string
	serverAddr string
)

func init() {
	var err error
	port := utils.GetRandomAvailableTCPPort()
	serverUrl, err = sfweb.NewSyntaxFlowWebServer(context.Background(), false, "127.0.0.1", port, true)
	if err != nil {
		panic(err)
	}
	serverAddr = utils.ExtractHostPort(serverUrl)
}

func debug() {
	sfweb.SfWebLogger.SetLevel("debug")
}

func DoResponse(method, path string, data any, opts ...poc.PocConfigOption) error {
	rsp, _, err := poc.Do(method, fmt.Sprintf("%s%s", serverUrl, path), opts...)
	if err != nil {
		return err
	}
	body := rsp.GetBody()
	err = json.Unmarshal(body, data)
	if err != nil {
		return utils.Wrap(err, "unmarshal json error")
	}
	return nil
}
