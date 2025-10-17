package yaklib

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"sync"
)

type QueryHTTPFlowOnlineRequest struct {
	ProjectName         string `json:"projectName"`
	Content             []byte `json:"content"`
	ProjectDescription  string `json:"projectDescription"`
	ExternalModule      string `json:"externalModule"`
	ExternalProjectCode string `json:"externalProjectCode"`
}

var (
	syncMu   sync.Mutex
	syncing  bool
	syncTask string // "auto" or "manual"
)

func StartSync(taskType string) (bool, string) {
	syncMu.Lock()
	defer syncMu.Unlock()
	if syncing {
		return false, syncTask
	}
	syncing = true
	syncTask = taskType
	return true, syncTask
}

func EndSync() {
	syncMu.Lock()
	defer syncMu.Unlock()
	syncing = false
	syncTask = ""
}

func (s *OnlineClient) UploadHTTPFlowToOnline(ctx context.Context, params *ypb.HTTPFlowsToOnlineRequest, content []byte) error {
	raw, err := json.Marshal(QueryHTTPFlowOnlineRequest{
		Content:             content,
		ProjectName:         params.ProjectName,
		ProjectDescription:  params.ProjectDescription,
		ExternalModule:      params.ExternalModule,
		ExternalProjectCode: params.ExternalProjectCode,
	})
	if err != nil {
		return utils.Errorf("marshal params failed: %s", err)
	}

	rsp, _, err := poc.DoPOST(
		fmt.Sprintf("%v/%v", consts.GetOnlineBaseUrl(), "api/httpflow/upload"),
		poc.WithReplaceHttpPacketHeader("Authorization", params.Token),
		poc.WithReplaceHttpPacketHeader("Content-Type", "application/json"),
		poc.WithReplaceHttpPacketBody(raw, true),
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
		return utils.Errorf("unmarshal httpflow to online response failed: %s", err)
	}
	if utils.MapGetString(responseData, "message") != "" || utils.MapGetString(responseData, "reason") != "" {
		return utils.Errorf("%s %s", utils.MapGetString(responseData, "reason"), utils.MapGetString(responseData, "message"))
	}
	return nil
}
