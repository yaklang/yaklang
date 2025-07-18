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

type UploadPayloadsToOnlineRequest struct {
	Content []byte `json:"content"`
}

type DownloadBatchPayloadsRequest struct {
	Group string `json:"group"`
}

func (s *OnlineClient) UploadPayloadsToOnline(ctx context.Context, token string, data []byte) error {
	urlIns, err := url.Parse(s.genUrl("/api/upload/payloads"))
	if err != nil {
		return utils.Errorf("parse url-instance failed: %s", err)
	}
	raw, err := json.Marshal(UploadPayloadsToOnlineRequest{
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
		return utils.Errorf("unmarshal payload to online response failed: %s", err)
	}
	if utils.MapGetString(responseData, "message") != "" || utils.MapGetString(responseData, "reason") != "" {
		return utils.Errorf(" %s %s", utils.MapGetString(responseData, "reason"), utils.MapGetString(responseData, "message"))
	}
	return nil
}

func (s *OnlineClient) DownloadBatchPayloads(
	group string,
) (*Payload, error) {
	urlIns, err := url.Parse(s.genUrl("/api/payloads/download"))
	if err != nil {
		return nil, utils.Errorf("parse url-instance failed: %s", err)
	}

	raw, err := json.Marshal(DownloadBatchPayloadsRequest{
		Group: group,
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
	var container *Payload
	var ret ResponseErr
	err = json.Unmarshal(rawResponse, &container)
	if err != nil {
		return nil, utils.Errorf("unmarshal payload response failed: %s", err.Error())
	}
	err = json.Unmarshal(rawResponse, &ret)
	if ret.Reason != "" {
		return nil, utils.Errorf("unmarshal payload response failed: %s", ret.Reason)
	}
	return container, nil
}
