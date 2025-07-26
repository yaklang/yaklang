package yaklib

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type UploadPayloadsToOnlineRequest struct {
	Content     []byte `json:"content"`
	FileContent []byte `json:"fileContent"`
}

type DownloadBatchPayloadsRequest struct {
	Page    int64  `json:"page"`
	Limit   int64  `json:"limit"`
	OrderBy string `json:"order_by"`
	Order   string `json:"order"`
	Group   string `json:"group"`
	Folder  string `json:"folder"`
}

type OnlinePayload struct {
	ID          int64
	Group       string
	Content     string
	ContentFile []byte
	Folder      string
	HitCount    int64
	IsFile      bool
	Hash        string
}

type OnlinePayloadItem struct {
	PayloadData *OnlinePayload
	Total       int64
}

type OnlineDownloadPayloadStream struct {
	Total     int64
	Page      int64
	PageTotal int64
	Limit     int64
	Chan      chan *OnlinePayloadItem
}

func (s *OnlineClient) UploadPayloadsToOnline(ctx context.Context, token string, data, fileContent []byte) error {
	raw, err := json.Marshal(UploadPayloadsToOnlineRequest{
		Content:     data,
		FileContent: fileContent,
	})
	if err != nil {
		return utils.Errorf("marshal params failed: %s", err)
	}

	rsp, _, err := poc.DoPOST(
		fmt.Sprintf("%v/%v", consts.GetOnlineBaseUrl(), "api/upload/payload"),
		poc.WithReplaceHttpPacketHeader("Authorization", token),
		poc.WithReplaceHttpPacketHeader("Content-Type", "application/json"),
		poc.WithReplaceHttpPacketBody(raw, true),
		poc.WithProxy(consts.GetOnlineBaseUrlProxy()),
		poc.WithSave(false),
	)
	if err != nil {
		return utils.Wrapf(err, "UploadPayloadsToOnline failed: http error")
	}
	rawResponse := lowhttp.GetHTTPPacketBody(rsp.RawPacket)

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
	ctx context.Context, token, group, folder string,
) *OnlineDownloadPayloadStream {
	var ch = make(chan *OnlinePayloadItem, 10)
	var rsp = &OnlineDownloadPayloadStream{
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
				log.Errorf("recover SyntaxFlowRule failed: %s", err)
			}
		}()

		var page = 0
		var retry = 0
		for {
			select {
			case <-ctx.Done():
			default:
			}
			page++

			// 设置超时处理的问题
		RETRYDOWNLOAD:
			payloads, paging, err := s.downloadOnlinePayload(group, folder, page, 30)
			if err != nil {
				retry++
				if retry <= 5 {
					log.Errorf("[RETRYING]: download SyntaxFlowRule failed: %s", err)
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

			if len(payloads) > 0 {
				for _, payload := range payloads {
					select {
					case ch <- &OnlinePayloadItem{
						PayloadData: payload,
						Total:       rsp.Total,
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

func (s *OnlineClient) downloadOnlinePayload(
	group, folder string,
	page int, limit int64,
) ([]*OnlinePayload, *OnlinePaging, error) {
	raw, err := json.Marshal(DownloadBatchPayloadsRequest{
		OrderBy: "",
		Order:   "",
		Page:    int64(page),
		Limit:   limit,
		Group:   group,
		Folder:  folder,
	})
	if err != nil {
		return nil, nil, utils.Errorf("marshal params failed: %s", err)
	}
	rsp, _, err := poc.DoPOST(
		fmt.Sprintf("%v/%v", consts.GetOnlineBaseUrl(), "api/download/payload"),
		//poc.WithReplaceHttpPacketHeader("Authorization", token),
		poc.WithReplaceHttpPacketHeader("Content-Type", "application/json"),
		poc.WithReplaceHttpPacketBody(raw, false),
		poc.WithProxy(consts.GetOnlineBaseUrlProxy()),
		poc.WithSave(false),
	)
	if err != nil {
		return nil, nil, utils.Errorf("SyntaxFlowRule UploadToOnline failed: http error")
	}
	rawResponse := lowhttp.GetHTTPPacketBody(rsp.RawPacket)

	type PayloadDownloadResponse struct {
		Data     []*OnlinePayload `json:"data"`
		Pagemeta *OnlinePaging    `json:"pagemeta"`
	}
	type OnlineErr struct {
		Form   string `json:"form"`
		Reason string `json:"reason"`
		Ok     bool   `json:"ok"`
	}
	var _container PayloadDownloadResponse
	var ret OnlineErr
	err = json.Unmarshal(rawResponse, &_container)
	if err != nil {
		return nil, nil, utils.Errorf("unmarshal payload response failed: %s", err.Error())
	}
	err = json.Unmarshal(rawResponse, &ret)
	if ret.Reason != "" {
		return nil, nil, utils.Errorf("unmarshal payload response failed: %s", ret.Reason)
	}
	return _container.Data, _container.Pagemeta, nil
}

func (s *OnlineClient) SavePayload(db *gorm.DB, payload ...*OnlinePayload) error {
	if db == nil {
		return utils.Error("empty database")
	}
	for _, p := range payload {
		if p.IsFile {
			// 文件需要单独写入

		}
		p := &schema.Payload{
			Group:    p.Group,
			Folder:   &p.Folder,
			Content:  &p.Content,
			HitCount: &p.HitCount,
			IsFile:   &p.IsFile,
			Hash:     p.Hash,
		}

		err := yakit.CreateOrUpdatePayload(db, *p.Content, p.Group, *p.Folder, *p.HitCount, *p.IsFile)
		if err != nil {
			log.Errorf("save [%s] to local failed: %s", p.Group, err)
			return err
		}
	}
	return nil
}
