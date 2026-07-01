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

func (s *OnlineClient) GetAIApiKeyByOnline(ctx context.Context, token string) (string, error) {
	rsp, _, err := poc.DoPOST(
		fmt.Sprintf("%v/%v", consts.GetOnlineBaseUrl(), "api/apikey"),
		poc.WithReplaceHttpPacketHeader("Authorization", token),
		poc.WithReplaceHttpPacketHeader("Content-Type", "application/json"),
		poc.WithProxy(consts.GetOnlineBaseUrlProxy()),
	)
	if err != nil {
		return "", utils.Wrapf(err, "GetAIApiKeyByOnline failed: http error")
	}
	rawResponse := lowhttp.GetHTTPPacketBody(rsp.RawPacket)
	if rsp.GetStatusCode() != 200 {
		var responseData map[string]interface{}
		err = json.Unmarshal(rawResponse, &responseData)
		return "", utils.Errorf("GetAIApiKeyByOnline failed: %s %s", utils.MapGetString(responseData, "reason"), utils.MapGetString(responseData, "message"))
	}
	return string(rawResponse), nil
}
