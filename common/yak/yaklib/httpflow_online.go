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
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
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

type DownloadHTTPFlowOnlineRequest struct {
	Token string `json:"token"`
	Page  int    `json:"page"`
	Limit int    `json:"limit"`
}

type OnlineHTTPFlowItem struct {
	Hash         string `json:"hash"`
	IssueType    string `json:"issueType"`
	Severity     string `json:"severity"`
	Status       string `json:"status"`
	StatusReason string `json:"statusReason"`
}

type HTTPFlowDownloadResponse struct {
	Data     []*OnlineHTTPFlowItem `json:"data"`
	Pagemeta *OnlinePaging         `json:"pagemeta"`
}

type HTTPFlowDownloadItem struct {
	Flow  *OnlineHTTPFlowItem
	Total int64
}

type HTTPFlowDownloadStream struct {
	Total     int64
	Page      int64
	PageTotal int64
	Limit     int64
	Chan      chan *HTTPFlowDownloadItem
}

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

func (s *OnlineClient) DownloadOnlineHTTPFlows(ctx context.Context, token string) *HTTPFlowDownloadStream {
	var ch = make(chan *HTTPFlowDownloadItem, 10)
	var rsp = &HTTPFlowDownloadStream{
		Total:     0,
		Page:      0,
		PageTotal: 0,
		Limit:     0,
		Chan:      ch,
	}
	go func() {
		defer close(ch)
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("recover download online httpflow failed: %s", err)
			}
		}()

		var page = 0
		var retry = 0
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			page++

		RETRYDOWNLOAD:
			flows, paging, err := s.downloadOnlineHTTPFlows(token, page, 30)
			if err != nil {
				retry++
				if retry <= 5 {
					log.Errorf("[RETRYING]: download online httpflow failed: %s", err)
					goto RETRYDOWNLOAD
				} else {
					break
				}
			} else {
				retry = 0
			}

			if paging != nil && rsp.Total <= 0 {
				rsp.Page = int64(paging.Page)
				rsp.Limit = int64(paging.Limit)
				rsp.PageTotal = int64(paging.TotalPage)
				rsp.Total = int64(paging.Total)
			}

			if len(flows) > 0 {
				for _, flow := range flows {
					select {
					case ch <- &HTTPFlowDownloadItem{
						Flow:  flow,
						Total: rsp.Total,
					}:
					case <-ctx.Done():
						return
					}
				}
			} else {
				break
			}
		}
	}()
	return rsp
}

func (s *OnlineClient) downloadOnlineHTTPFlows(token string, page int, limit int) ([]*OnlineHTTPFlowItem, *OnlinePaging, error) {
	raw, err := json.Marshal(DownloadHTTPFlowOnlineRequest{
		Token: token,
		Page:  page,
		Limit: limit,
	})
	if err != nil {
		return nil, nil, utils.Errorf("marshal params failed: %s", err)
	}

	rsp, _, err := poc.DoPOST(
		fmt.Sprintf("%v/%v", consts.GetOnlineBaseUrl(), "api/httpflow"),
		poc.WithReplaceHttpPacketHeader("Authorization", token),
		poc.WithReplaceHttpPacketHeader("Content-Type", "application/json"),
		poc.WithReplaceHttpPacketBody(raw, true),
		poc.WithProxy(consts.GetOnlineBaseUrlProxy()),
	)
	if err != nil {
		return nil, nil, utils.Wrapf(err, "downloadOnlineHTTPFlows failed: http error")
	}
	rawResponse := lowhttp.GetHTTPPacketBody(rsp.RawPacket)

	if rsp.GetStatusCode() != 200 {
		var responseData map[string]interface{}
		err = json.Unmarshal(rawResponse, &responseData)
		return nil, nil, utils.Errorf("unmarshal httpflow response failed: %s %s", utils.MapGetString(responseData, "reason"), utils.MapGetString(responseData, "message"))
	}

	var _container HTTPFlowDownloadResponse
	err = json.Unmarshal(rawResponse, &_container)
	if err != nil {
		return nil, nil, utils.Errorf("unmarshal httpflow response failed: %s", err.Error())
	}
	return _container.Data, _container.Pagemeta, nil
}

type setHTTPFlowTagsRequest struct {
	Hash         string `json:"hash"`
	SetIssueType string `json:"setIssueType"`
	SetSeverity  string `json:"setSeverity"`
	SetStatus    string `json:"setStatus"`
	StatusReason string `json:"statusReason"`
}

func (s *OnlineClient) SetHTTPFlowTagsToOnline(ctx context.Context, token string, hashes []string, setIssueType, setSeverity, setStatus, statusReason string) error {
	if token == "" {
		return utils.Errorf("token is empty")
	}
	if len(hashes) == 0 {
		return nil
	}

	raw, err := json.Marshal(setHTTPFlowTagsRequest{
		Hash:         strings.Join(hashes, ","),
		SetIssueType: setIssueType,
		SetSeverity:  setSeverity,
		SetStatus:    setStatus,
		StatusReason: statusReason,
	})
	if err != nil {
		return utils.Errorf("marshal set httpflow tags request failed: %s", err)
	}

	rsp, _, err := poc.DoPOST(
		fmt.Sprintf("%v/%v", consts.GetOnlineBaseUrl(), "api/set/httpflow/tags"),
		poc.WithReplaceHttpPacketHeader("Authorization", token),
		poc.WithReplaceHttpPacketHeader("Content-Type", "application/json"),
		poc.WithReplaceHttpPacketBody(raw, true),
		poc.WithProxy(consts.GetOnlineBaseUrlProxy()),
		poc.WithSave(false),
	)
	if err != nil {
		return utils.Errorf("set httpflow tags to online failed: %s", err)
	}

	if rsp.GetStatusCode() != 200 {
		rawResponse := lowhttp.GetHTTPPacketBody(rsp.RawPacket)
		var errData map[string]interface{}
		_ = json.Unmarshal(rawResponse, &errData)
		return utils.Errorf("set httpflow tags to online error: %s%s", utils.MapGetString(errData, "reason"), utils.MapGetString(errData, "message"))
	}

	rawResponse := lowhttp.GetHTTPPacketBody(rsp.RawPacket)
	var responseData map[string]interface{}
	if err := json.Unmarshal(rawResponse, &responseData); err != nil {
		return utils.Errorf("unmarshal set httpflow tags response failed: %s", err)
	}
	if msg := utils.MapGetString(responseData, "message"); msg != "" {
		return utils.Errorf("set httpflow tags to online error: %s %s", utils.MapGetString(responseData, "reason"), msg)
	}
	return nil
}

type downloadHTTPFlowRequest struct {
	Page    int64  `json:"page"`
	Limit   int64  `json:"limit"`
	Order   string `json:"order"`
	OrderBy string `json:"order_by"`
}

type DownloadHTTPFlowItem struct {
	Hash          string `json:"hash"`
	URL           string `json:"url"`
	Path          string `json:"path"`
	Method        string `json:"method"`
	IsHTTPS       bool   `json:"isHTTPS"`
	StatusCode    int64  `json:"statusCode"`
	ContentType   string `json:"contentType"`
	SourceType    string `json:"sourceType"`
	Request       string `json:"request"`
	Response      string `json:"response"`
	BodyLength    int64  `json:"bodyLength"`
	HTMLTitle     string `json:"htmlTitle"`
	IPAddress     string `json:"ipAddress"`
	HostPort      string `json:"hostPort"`
	Host          string `json:"host"`
	Tags          string `json:"tags"`
	FromPlugin    string `json:"fromPlugin"`
	HiddenIndex   string `json:"hiddenIndex"`
	IsWebsocket   bool   `json:"isWebsocket"`
	WebsocketHash string `json:"websocketHash"`
	ProjectName   string `json:"projectName"`
	UserName      string `json:"userName"`
	IssueType     string `json:"issueType"`
	Severity      string `json:"severity"`
	Status        string `json:"status"`
	StatusReason  string `json:"statusReason"`
}

type downloadHTTPFlowResponse struct {
	Pagemeta *OnlinePaging           `json:"pagemeta"`
	Data     []*DownloadHTTPFlowItem `json:"data"`
}

// DownloadMyHTTPFlowStreamItem 流式下载的单条结果，携带当前条数据和总数（用于进度计算）。
type DownloadHTTPFlowStreamItem struct {
	Flow  *DownloadHTTPFlowItem
	Total int64
}

func (s *OnlineClient) DownloadHTTPFlows(ctx context.Context, token string) (chan *DownloadHTTPFlowStreamItem, error) {
	if token == "" {
		return nil, utils.Errorf("token is empty")
	}
	ch := make(chan *DownloadHTTPFlowStreamItem, 10)
	go func() {
		defer close(ch)
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("recover download  httpflow failed: %s", err)
			}
		}()

		const limit int64 = 30
		var (
			retry int
			total int64
			page  int64 = 1
		)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

		RETRY:
			items, paging, err := s.downloadHTTPFlowPage(token, page, limit)
			if err != nil {
				retry++
				if retry <= 5 {
					log.Errorf("[RETRYING]: download  httpflow page %d failed: %s", page, err)
					goto RETRY
				} else {
					log.Errorf("download  httpflow page %d failed after retries: %s", page, err)
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
					case ch <- &DownloadHTTPFlowStreamItem{
						Flow:  item,
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

func (s *OnlineClient) downloadHTTPFlowPage(token string, page, limit int64) ([]*DownloadHTTPFlowItem, *OnlinePaging, error) {
	raw, err := json.Marshal(downloadHTTPFlowRequest{
		Page:    page,
		Limit:   limit,
		Order:   "desc",
		OrderBy: "updated_at",
	})
	if err != nil {
		return nil, nil, utils.Errorf("marshal download httpflow request failed: %s", err)
	}

	rsp, _, err := poc.DoPOST(
		fmt.Sprintf("%v/%v", consts.GetOnlineBaseUrl(), "api/httpflow/download"),
		poc.WithReplaceHttpPacketHeader("Authorization", token),
		poc.WithReplaceHttpPacketHeader("Content-Type", "application/json"),
		poc.WithReplaceHttpPacketBody(raw, true),
		poc.WithProxy(consts.GetOnlineBaseUrlProxy()),
		poc.WithSave(false),
	)
	if err != nil {
		return nil, nil, utils.Errorf("download my httpflow failed: %s", err)
	}

	rawResponse := lowhttp.GetHTTPPacketBody(rsp.RawPacket)
	if rsp.GetStatusCode() != 200 {
		var errData map[string]interface{}
		_ = json.Unmarshal(rawResponse, &errData)
		return nil, nil, utils.Errorf("download httpflow error: %s%s", utils.MapGetString(errData, "reason"), utils.MapGetString(errData, "message"))
	}

	var container downloadHTTPFlowResponse
	if err := json.Unmarshal(rawResponse, &container); err != nil {
		return nil, nil, utils.Errorf("unmarshal download httpflow response failed: %s", err)
	}
	return container.Data, container.Pagemeta, nil
}
