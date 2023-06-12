package yaklib

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io/ioutil"
	"net/url"
)

type HttpFlowShareRequest struct {
	ExpiredTime           int64     `json:"expired_time"`
	Module        		  string    `json:"module"`
	ShareContent          string    `json:"share_content"`
	Pwd       			  bool    `json:"pwd"`
	LimitNum              int64     `json:"limit_num"`
	Token            	  string    `json:"token"`
}

type HttpFlowShare struct {
	ShareId string        	`json:"share_id"`
	ExtractCode string		`json:"extract_code"`
}

func (s *OnlineClient) HttpFlowShareWithToken(ctx context.Context, token string, expiredTime int64, module string, shareContent string, pwd bool, limitNum int64) (*HttpFlowShare, error) {
	res, err := s.HttpFlowShare(ctx,
		token,
		expiredTime,
		module,
		shareContent,
		pwd,
		limitNum,
	)
	if err != nil {
		log.Errorf("httpFlow share failed: %s", err.Error())
		return nil, utils.Errorf("httpFlow share failed: %s", err.Error())
	}

	return res, nil
}

func (s *OnlineClient) HttpFlowShare(ctx context.Context,
	token string, expiredTime int64, module string, shareContent string, pwd bool, limitNum int64) (*HttpFlowShare, error) {
	urlIns, err := url.Parse(s.genUrl("/api/module/share"))
	if err != nil {
		return nil, utils.Errorf("parse url-instance failed: %s", err)
	}
	raw, err := json.Marshal(HttpFlowShareRequest{
		ExpiredTime:  expiredTime,
		Module:       module,
		ShareContent: shareContent,
		Pwd:          pwd,
		LimitNum:     limitNum,
		Token:        token,
	})
	if err != nil {
		return nil, utils.Errorf("marshal params failed: %s", err)
	}

	rsp, err := s.client.Post(urlIns.String(), "application/json", bytes.NewBuffer(raw))
	if err != nil {
		return nil, utils.Errorf("HTTP Post %v failed: %v params:%s", urlIns.String(), err, string(raw))
	}
	rawResponse, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return nil, utils.Errorf("read body failed: %s", err)
	}

	type HttpFlowShareResponse struct {
		Data     *HttpFlowShare `json:"data"`
	}
	var _container HttpFlowShareResponse
	var responseData map[string]interface{}
	err = json.Unmarshal(rawResponse, &responseData)
	if err != nil {
		return nil, utils.Errorf("unmarshal plugin response failed: %s", err)
	}
	if utils.MapGetString(responseData, "reason") != "" {
		return nil, utils.Errorf("httpFlow share failed: %s", utils.MapGetString(responseData, "reason") )
	}
	_container.Data.ShareId = utils.MapGetString(responseData, "share_id")
	_container.Data.ExtractCode = utils.MapGetString(responseData, "extract_code")
	return _container.Data,  nil

}
