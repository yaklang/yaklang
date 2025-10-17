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

type UploadHotPatchTemplateToOnlineRequest struct {
	Content []byte `json:"content"`
}

type DownloadHotPatchTemplateRequest struct {
	Name         string `json:"name"`
	TemplateType string `json:"type"`
}

type HotPatchTemplate struct {
	Name         string `json:"name"`
	Content      string `json:"content"`
	TemplateType string `json:"type"`
}

type ResponseErr struct {
	Form   string `json:"form"`
	Reason string `json:"reason"`
	Ok     bool   `json:"ok"`
}

func (s *OnlineClient) UploadHotPatchTemplateToOnline(ctx context.Context, token string, data []byte) error {
	raw, err := json.Marshal(UploadHotPatchTemplateToOnlineRequest{
		Content: data,
	})
	if err != nil {
		return utils.Errorf("marshal params failed: %s", err)
	}

	rsp, _, err := poc.DoPOST(
		fmt.Sprintf("%v/%v", consts.GetOnlineBaseUrl(), "api/hot/patch/template"),
		poc.WithReplaceHttpPacketHeader("Authorization", token),
		poc.WithReplaceHttpPacketHeader("Content-Type", "application/json"),
		poc.WithReplaceHttpPacketBody(raw, true),
		poc.WithProxy(consts.GetOnlineBaseUrlProxy()),
		poc.WithSave(false),
	)
	if err != nil {
		return utils.Wrapf(err, "UploadHotPatchTemplateToOnline failed: http error")
	}
	rawResponse := lowhttp.GetHTTPPacketBody(rsp.RawPacket)

	if err != nil {
		return utils.Errorf("read body failed: %s", err)
	}
	var responseData map[string]interface{}
	err = json.Unmarshal(rawResponse, &responseData)
	if err != nil {
		return utils.Errorf("unmarshal HotPatchTemplate to online response failed: %s", err)
	}
	if utils.MapGetString(responseData, "message") != "" || utils.MapGetString(responseData, "reason") != "" {
		return utils.Errorf(" %s %s", utils.MapGetString(responseData, "reason"), utils.MapGetString(responseData, "message"))
	}
	return nil
}

func (s *OnlineClient) DownloadHotPatchTemplate(
	token, name, templateType string,
) (*HotPatchTemplate, error) {

	raw, err := json.Marshal(DownloadHotPatchTemplateRequest{
		Name:         name,
		TemplateType: templateType,
	})
	if err != nil {
		return nil, utils.Errorf("marshal params failed: %s", err)
	}

	rsp, _, err := poc.DoPOST(
		fmt.Sprintf("%v/%v", consts.GetOnlineBaseUrl(), "api/hot/patch/template/download"),
		poc.WithReplaceHttpPacketHeader("Authorization", token),
		poc.WithReplaceHttpPacketHeader("Content-Type", "application/json"),
		poc.WithReplaceHttpPacketBody(raw, true),
		poc.WithProxy(consts.GetOnlineBaseUrlProxy()),
		poc.WithSave(false),
	)
	if err != nil {
		return nil, utils.Wrapf(err, "DownloadHotPatchTemplate failed: http error")
	}
	rawResponse := lowhttp.GetHTTPPacketBody(rsp.RawPacket)
	if err != nil {
		return nil, utils.Errorf("read body failed: %s", err)
	}
	var container *HotPatchTemplate
	var ret ResponseErr
	err = json.Unmarshal(rawResponse, &container)
	if err != nil {
		return nil, utils.Errorf("unmarshal plugin response failed: %s", err.Error())
	}
	err = json.Unmarshal(rawResponse, &ret)
	if ret.Reason != "" {
		return nil, utils.Errorf("unmarshal plugin response failed: %s", ret.Reason)
	}
	return container, nil
}
