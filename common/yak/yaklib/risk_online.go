package yaklib

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
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

type downloadRiskRequest struct {
	Page    int64  `json:"page"`
	Limit   int64  `json:"limit"`
	Order   string `json:"order"`
	OrderBy string `json:"order_by"`
}

type DownloadRiskItem struct {
	Hash            string  `json:"hash"`
	Title           string  `json:"title"`
	TitleVerbose    string  `json:"titleVerbose"`
	Description     string  `json:"description"`
	Solution        string  `json:"solution"`
	RiskType        string  `json:"riskType"`
	RiskTypeVerbose string  `json:"riskTypeVerbose"`
	Severity        string  `json:"severity"`
	Parameter       string  `json:"parameter"`
	Payload         string  `json:"payload"`
	Details         string  `json:"details"`
	Url             string  `json:"url"`
	Host            string  `json:"host"`
	Port            int     `json:"port"`
	IP              string  `json:"ipAddress"`
	SourceType      string  `json:"sourceType"`
	FromYakScript   string  `json:"fromYakScript"`
	Tags            string  `json:"tags"`
	VerifierUid     string  `json:"verifierUid"`
	FixTime         int64   `json:"fixTime"`
	FixSuggestion   string  `json:"fixSuggestion"`
	TagReason    string  `json:"tagReason"`
	IsPotential     bool    `json:"isPotential"`
	CVE             string  `json:"cve"`
	SeverityScore   float64 `json:"severityScore"`
}

type riskDownloadResponse struct {
	Pagemeta *OnlinePaging       `json:"pagemeta"`
	Data     []*DownloadRiskItem `json:"data"`
}

type DownloadRiskStreamItem struct {
	Risk  *DownloadRiskItem
	Total int64
}

func (s *OnlineClient) DownloadRisks(ctx context.Context, token string) (chan *DownloadRiskStreamItem, error) {
	if token == "" {
		return nil, utils.Errorf("token is empty")
	}

	ch := make(chan *DownloadRiskStreamItem, 10)
	go func() {
		defer close(ch)
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("recover download risks failed: %s", err)
			}
		}()

		var page int64 = 1
		const limit int64 = 30
		var retry int
		var total int64
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

		RETRY:
			items, paging, err := s.downloadRiskPage(token, page, limit)
			if err != nil {
				retry++
				if retry <= 5 {
					log.Errorf("[RETRYING]: download risk page %d failed: %s", page, err)
					goto RETRY
				} else {
					log.Errorf("download risk page %d failed after retries: %s", page, err)
					return
				}
			} else {
				retry = 0
			}

			if paging != nil && total <= 0 {
				total = int64(paging.Total)
			}

			if len(items) > 0 {
				for _, item := range items {
					select {
					case ch <- &DownloadRiskStreamItem{
						Risk:  item,
						Total: total,
					}:
					case <-ctx.Done():
						return
					}
				}
			}

			if paging == nil || page >= int64(paging.TotalPage) {
				return
			}
			page++
		}
	}()
	return ch, nil
}

func (s *OnlineClient) downloadRiskPage(token string, page, limit int64) ([]*DownloadRiskItem, *OnlinePaging, error) {
	raw, err := json.Marshal(downloadRiskRequest{
		Page:    page,
		Limit:   limit,
		Order:   "desc",
		OrderBy: "updated_at",
	})
	if err != nil {
		return nil, nil, utils.Errorf("marshal download risk request failed: %s", err)
	}

	rsp, _, err := poc.DoPOST(
		fmt.Sprintf("%v/%v", consts.GetOnlineBaseUrl(), "api/risk/download"),
		poc.WithReplaceHttpPacketHeader("Authorization", token),
		poc.WithReplaceHttpPacketHeader("Content-Type", "application/json"),
		poc.WithReplaceHttpPacketBody(raw, true),
		poc.WithProxy(consts.GetOnlineBaseUrlProxy()),
		poc.WithSave(false),
	)
	if err != nil {
		return nil, nil, utils.Errorf("download risk failed: %s", err)
	}

	rawResponse := lowhttp.GetHTTPPacketBody(rsp.RawPacket)
	if rsp.GetStatusCode() != 200 {
		var errData map[string]interface{}
		_ = json.Unmarshal(rawResponse, &errData)
		return nil, nil, utils.Errorf("download risk error:%s%s", utils.MapGetString(errData, "reason"), utils.MapGetString(errData, "message"))
	}

	var container riskDownloadResponse
	if err := json.Unmarshal(rawResponse, &container); err != nil {
		return nil, nil, utils.Errorf("unmarshal download risk response failed: %s", err)
	}
	return container.Data, container.Pagemeta, nil
}

type setRiskTagsRequest struct {
	Hash            []string `json:"hash"`
	SetTags         []string `json:"setTags"`
	VerifierUid     string   `json:"verifierUid,omitempty"`
	FixTime         int64    `json:"fixTime,omitempty"`
	FixSuggestion   string   `json:"fixSuggestion,omitempty"`
	TagReason    string   `json:"tagReason,omitempty"`
	RiskTypeVerbose string   `json:"riskTypeVerbose,omitempty"`
	Severity        string   `json:"severity,omitempty"`
	SeverityScore   float64  `json:"severityScore,omitempty"`
}

func (s *OnlineClient) SetRiskTagsToOnline(ctx context.Context, token string, hashes []string, setTags, verifierUid, fixSuggestion, tagReason, riskTypeVerbose, severity string, fixTime int64, severityScore float64) error {
	if token == "" {
		return utils.Errorf("token is empty")
	}
	if len(hashes) == 0 {
		return nil
	}

	// tags 竖线分隔 → []string
	var tagsSlice []string
	if setTags != "" {
		tagsSlice = utils.PrettifyListFromStringSplited(setTags, "|")
	}

	raw, err := json.Marshal(setRiskTagsRequest{
		Hash:            hashes,
		SetTags:         tagsSlice,
		VerifierUid:     verifierUid,
		FixTime:         fixTime,
		FixSuggestion:   fixSuggestion,
		TagReason:    tagReason,
		RiskTypeVerbose: riskTypeVerbose,
		Severity:        severity,
		SeverityScore:   severityScore,
	})
	if err != nil {
		return utils.Errorf("marshal set risk tags request failed: %s", err)
	}

	rsp, _, err := poc.DoPOST(
		fmt.Sprintf("%v/%v", consts.GetOnlineBaseUrl(), "api/set/risk/tags"),
		poc.WithReplaceHttpPacketHeader("Authorization", token),
		poc.WithReplaceHttpPacketHeader("Content-Type", "application/json"),
		poc.WithReplaceHttpPacketBody(raw, true),
		poc.WithProxy(consts.GetOnlineBaseUrlProxy()),
		poc.WithSave(false),
	)
	if err != nil {
		return utils.Errorf("set risk tags to online failed: %s", err)
	}
	rawResponse := lowhttp.GetHTTPPacketBody(rsp.RawPacket)
	if rsp.GetStatusCode() != 200 {
		var errData map[string]interface{}
		_ = json.Unmarshal(rawResponse, &errData)
		return utils.Errorf("set risk tags to online error:%s%s", utils.MapGetString(errData, "reason"), utils.MapGetString(errData, "message"))
	}

	return nil
}
