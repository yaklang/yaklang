package yaklib

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io/ioutil"
	"net/http"
	"net/url"
)

type QueryUploadRiskOnlineRequest struct {
	ProjectName string `json:"projectName"`
	Content     []byte `json:"content"`
}

func (s *OnlineClient) UploadRiskToOnlineWithToken(ctx context.Context, req *ypb.UploadRiskToOnlineRequest, risk []byte) error {
	err := s.UploadRiskToOnline(ctx,
		req.Token,
		req.ProjectName,
		risk,
	)
	if err != nil {
		log.Errorf("upload risk to online failed: %s", err.Error())
		return utils.Errorf("upload risk to online failed: %s", err.Error())
	}

	return nil
}

func (s *OnlineClient) UploadRiskToOnline(ctx context.Context,
	token string, projectName string, content []byte) error {
	urlIns, err := url.Parse(s.genUrl("/api/risk/upload"))
	if err != nil {
		return utils.Errorf("parse url-instance failed: %s", err)
	}
	raw, err := json.Marshal(QueryUploadRiskOnlineRequest{
		projectName,
		content,
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
		return utils.Errorf("unmarshal upload risk to online response failed: %s", err)
	}
	if utils.MapGetString(responseData, "message") != "" || utils.MapGetString(responseData, "reason") != "" {
		return utils.Errorf("%s %s", utils.MapGetString(responseData, "reason"), utils.MapGetString(responseData, "message"))
	}
	return nil
}
