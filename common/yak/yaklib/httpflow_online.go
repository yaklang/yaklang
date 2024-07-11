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

type QueryHTTPFlowOnlineRequest struct {
	ProjectName        string `json:"projectName"`
	Content            []byte `json:"content"`
	ProjectDescription string `json:"projectDescription"`
}

func (s *OnlineClient) UploadHTTPFlowToOnline(ctx context.Context, token, projectName, projectDescription string, content []byte) error {
	urlIns, err := url.Parse(s.genUrl("/api/httpflow/upload"))
	if err != nil {
		return utils.Errorf("parse url-instance failed: %s", err)
	}
	raw, err := json.Marshal(QueryHTTPFlowOnlineRequest{
		Content:            content,
		ProjectName:        projectName,
		ProjectDescription: projectDescription,
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
		return utils.Errorf("HTTP Post %v failed: %v ", urlIns.String(), err)
	}

	rawResponse, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return utils.Errorf("read body failed: %s", err)
	}
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
