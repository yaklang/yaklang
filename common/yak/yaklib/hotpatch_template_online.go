package yaklib

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/yaklang/yaklang/common/utils"
	"io/ioutil"
	"net/http"
	"net/url"
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
	urlIns, err := url.Parse(s.genUrl("/api/hot/patch/template"))
	if err != nil {
		return utils.Errorf("parse url-instance failed: %s", err)
	}
	raw, err := json.Marshal(UploadHotPatchTemplateToOnlineRequest{
		Content: data,
	})
	if err != nil {
		return utils.Errorf("marshal params failed: %s", err)
	}

	req, err := http.NewRequest("POST", urlIns.String(), bytes.NewBuffer(raw))
	if err != nil {
		return utils.Errorf(err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	rsp, err := s.client.Do(req)
	if err != nil {
		return utils.Errorf(" HTTP Post %v failed: %v ", urlIns.String(), err)
	}

	defer rsp.Body.Close()

	rawResponse, err := ioutil.ReadAll(rsp.Body)
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
	name, templateType string,
) (*HotPatchTemplate, error) {
	urlIns, err := url.Parse(s.genUrl("/api/hot/patch/template/download"))
	if err != nil {
		return nil, utils.Errorf("parse url-instance failed: %s", err)
	}

	raw, err := json.Marshal(DownloadHotPatchTemplateRequest{
		Name:         name,
		TemplateType: templateType,
	})
	if err != nil {
		return nil, utils.Errorf("marshal params failed: %s", err)
	}
	rsp, err := s.client.Post(urlIns.String(), "application/json", bytes.NewBuffer(raw))
	if err != nil {
		return nil, utils.Errorf("HTTP Post %v failed: %v ", urlIns.String(), err)
	}

	defer rsp.Body.Close()

	rawResponse, err := ioutil.ReadAll(rsp.Body)
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
