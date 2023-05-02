package yaklib

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/url"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/yakgrpc/yakit"
)

type QueryUploadRiskOnlineRequest struct {
	Token           string `json:"token"`
	RiskHash        string `json:"risk_hash"`
	IP              string `json:"ip"`
	IPInteger       int64  `json:"ip_integer"`
	Url             string `json:"url"`
	Port            int    `json:"port"`
	Host            string `json:"host"`
	Title           string `json:"title"`
	TitleVerbose    string `json:"title_verbose"`
	RiskType        string `json:"risk_type"`
	RiskTypeVerbose string `json:"risk_type_verbose"`
	Parameter       string `json:"parameter"`
	Payload         string `json:"payload"`
	Details         string `json:"details"`
	FromYakScript   string `json:"from_yak_script"`
	WaitingVerified bool   `json:"waiting_verified"`
	ReverseToken    string `json:"reverse_token"`
	Severity        string `json:"severity"`
	Request         string `json:"request"`
	Response        string `json:"response"`
	RuntimeId       string `json:"runtime_id"`
	CVE             string `json:"cve"`
	Description     string `json:"description"`
	Solution        string `json:"solution"`
	RiskCreatedAt   int64  `json:"risk_created_at"`
}

func (s *OnlineClient) UploadRiskToOnlineWithToken(ctx context.Context, token string, risk *yakit.Risk) error {
	err := s.UploadRiskToOnline(ctx,
		token,
		risk.Hash,
		risk.IP,
		risk.IPInteger,
		risk.Url,
		risk.Port,
		risk.Host,
		risk.Title,
		risk.TitleVerbose,
		risk.RiskType,
		risk.RiskTypeVerbose,
		risk.Parameter,
		risk.Payload,
		risk.Details,
		risk.FromYakScript,
		risk.WaitingVerified,
		risk.ReverseToken,
		risk.Severity,
		risk.QuotedRequest,
		risk.QuotedResponse,
		risk.RuntimeId,
		risk.CVE,
		risk.Description,
		risk.Solution,
		risk.CreatedAt.Unix(),
	)
	if err != nil {
		log.Errorf("upload risk to online failed: %s", err.Error())
		return utils.Errorf("upload risk to online failed: %s", err.Error())
	}

	return nil
}

func (s *OnlineClient) UploadRiskToOnline(ctx context.Context,
	token string, hash string, ip string, ipInteger int64, Url string, port int,
	host string, title string, titleVerbose string, riskType string, riskTypeVerbose string, parameter string,
	payload string, details string, fromYakScript string, waitingVerified bool, reverseToken string, severity string,
	request string, response string, runtimeId string, cve string, description string, solution string, riskCreatedAt int64) error {
	urlIns, err := url.Parse(s.genUrl("/api/risk/upload"))
	if err != nil {
		return utils.Errorf("parse url-instance failed: %s", err)
	}
	raw, err := json.Marshal(QueryUploadRiskOnlineRequest{
		Token:           token,
		RiskHash:        hash,
		IP:              ip,
		IPInteger:       ipInteger,
		Url:             Url,
		Port:            port,
		Host:            host,
		Title:           title,
		TitleVerbose:    titleVerbose,
		RiskType:        riskType,
		RiskTypeVerbose: riskTypeVerbose,
		Parameter:       utils.EscapeInvalidUTF8Byte([]byte(parameter)),
		Payload:         payload,
		Details:         details,
		FromYakScript:   fromYakScript,
		WaitingVerified: waitingVerified,
		ReverseToken:    reverseToken,
		Severity:        severity,
		Request:         request,
		Response:        response,
		RuntimeId:       runtimeId,
		CVE:             cve,
		Description:     description,
		Solution:        solution,
		RiskCreatedAt:   riskCreatedAt,
	})
	if err != nil {
		return utils.Errorf("marshal params failed: %s", err)
	}

	rsp, err := s.client.Post(urlIns.String(), "application/json", bytes.NewBuffer(raw))
	if err != nil {
		return utils.Errorf("HTTP Post %v failed: %v params:%s", urlIns.String(), err, string(raw))
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
	if !utils.MapGetBool(responseData, "ok") {
		return utils.Errorf("upload risk to online failed: %s", utils.MapGetString(responseData, "reason"))
	}
	return nil
}
