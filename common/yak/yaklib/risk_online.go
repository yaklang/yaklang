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

type QueryUploadRiskOnlineRequest struct {
	ProjectName         string `json:"projectName"`
	Content             []byte `json:"content"`
	ExternalModule      string `json:"externalModule"`
	ExternalProjectCode string `json:"externalProjectCode"`
}

type UploadOnlineRequest struct {
	Content []byte `json:"content"`
}

func (s *OnlineClient) UploadToOnline(ctx context.Context,
	token string, raw []byte, urlStr string) error {
	rsp, _, err := poc.DoPOST(
		fmt.Sprintf("%v/%v", consts.GetOnlineBaseUrl(), urlStr),
		poc.WithReplaceHttpPacketHeader("Authorization", token),
		poc.WithReplaceHttpPacketHeader("Content-Type", "application/json"),
		poc.WithReplaceHttpPacketBody(raw, false),
		poc.WithProxy(consts.GetOnlineBaseUrlProxy()),
		poc.WithSave(false),
	)
	if err != nil {
		return utils.Wrapf(err, "UploadToOnline failed: http error")
	}
	rawResponse := lowhttp.GetHTTPPacketBody(rsp.RawPacket)
	var responseData map[string]interface{}
	err = json.Unmarshal(rawResponse, &responseData)
	if err != nil {
		return utils.Errorf("unmarshal to online response failed: %s", err)
	}
	if utils.MapGetString(responseData, "message") != "" || utils.MapGetString(responseData, "reason") != "" {
		return utils.Errorf("%s %s", utils.MapGetString(responseData, "reason"), utils.MapGetString(responseData, "message"))
	}
	return nil
}
