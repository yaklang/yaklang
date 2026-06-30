package yaklib

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

func (s *OnlineClient) CreateAIApiKeyByOnline(ctx context.Context, token string) (string, error) {

	rsp, _, err := poc.DoGET(
		fmt.Sprintf("%v/%v", consts.GetOnlineBaseUrl(), "api/apikey"),
		poc.WithReplaceHttpPacketHeader("Authorization", token),
		poc.WithReplaceHttpPacketHeader("Content-Type", "application/json"),
		//poc.WithReplaceHttpPacketBody(raw, true),
		poc.WithProxy(consts.GetOnlineBaseUrlProxy()),
		//poc.WithSave(false),
	)
	if err != nil {
		return "", utils.Wrapf(err, "CreatedApiKey failed: http error")
	}
	rawResponse := lowhttp.GetHTTPPacketBody(rsp.RawPacket)

	var responseData map[string]interface{}
	err = json.Unmarshal(rawResponse, &responseData)
	if err != nil {
		return "", utils.Errorf("unmarshal AIApiKey response failed: %s", err)
	}
	if !utils.MapGetBool(responseData, "ok") {
		return "", utils.Errorf("%s", utils.MapGetString(responseData, "reason"))
	}
	return "", nil
}
