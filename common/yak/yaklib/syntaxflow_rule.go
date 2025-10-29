package yaklib

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type DownloadOnlineSyntaxFlowRuleRequest struct {
	AfterId           int64    `json:"after_id"`
	BeforeId          int64    `json:"before_id"`
	Page              int64    `json:"page"`
	Limit             int64    `json:"limit"`
	OrderBy           string   `json:"order_by"`
	Order             string   `json:"order"`
	RuleNames         []string `json:"ruleNames"`
	Language          []string `json:"language"`
	GroupNames        []string `json:"groupNames"`
	Severity          []string `json:"severity"`
	Purpose           []string `json:"purpose"`
	Tag               []string `json:"tag"`
	Keyword           string   `json:"keyword"`
	FilterRuleKind    string   `json:"filterRuleKind"`
	FilterLibRuleKind string   `json:"filterLibRuleKind"`
}

type OnlineSyntaxFlowRule struct {
	RuleName      string                                           `json:"ruleName"`
	RuleId        string                                           `json:"ruleId"`
	Content       string                                           `json:"content"`
	Language      string                                           `json:"language"`
	Type          string                                           `json:"type"`
	Severity      string                                           `json:"severity"`
	Purpose       string                                           `json:"purpose"`
	IsBuildInRule bool                                             `json:"isBuildInRule"`
	Title         string                                           `json:"title"`
	TitleZh       string                                           `json:"titleZh"`
	Description   string                                           `json:"description"`
	Verified      bool                                             `json:"verified"`
	AllowIncluded bool                                             `json:"allowIncluded"`
	Tag           string                                           `json:"tag"`
	AlertDesc     schema.MapEx[string, *schema.SyntaxFlowDescInfo] `json:"alert_desc"`
	CVE           string                                           `json:"cve"`
	CWE           []string                                         `json:"cwe"`
	RiskType      string                                           `json:"risk_type"`
	Hash          string                                           `json:"hash"`
	GroupName     []string                                         `json:"groupName"`
	Version       string                                           `json:"version"`
}

type OnlineSyntaxFlowRuleItem struct {
	Rule  *OnlineSyntaxFlowRule
	Total int64
}

type OnlineDownloadFlowRuleStream struct {
	Total     int64
	Page      int64
	PageTotal int64
	Limit     int64
	Chan      chan *OnlineSyntaxFlowRuleItem
}

func (s *OnlineClient) DownloadOnlineSyntaxFlowRule(
	ctx context.Context,
	token string,
	req *ypb.DownloadSyntaxFlowRuleRequest,
) *OnlineDownloadFlowRuleStream {
	var ch = make(chan *OnlineSyntaxFlowRuleItem, 10)
	var rsp = &OnlineDownloadFlowRuleStream{
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
			rules, paging, err := s.downloadOnlineSyntaxFlowRule(token, req.Filter.AfterId, req.Filter.BeforeId, req.Filter.RuleNames, req.Filter.Language, req.Filter.GroupNames, req.Filter.Severity, req.Filter.Purpose, req.Filter.Tag,
				req.Filter.Keyword, req.Filter.FilterRuleKind, req.Filter.FilterLibRuleKind,
				page, 30)
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

			if len(rules) > 0 {
				for _, rule := range rules {
					select {
					case ch <- &OnlineSyntaxFlowRuleItem{
						Rule:  rule,
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

func (s *OnlineClient) downloadOnlineSyntaxFlowRule(
	token string,
	afterId, beforeId int64,
	ruleNames []string,
	language []string,
	groupNames []string,
	severity []string,
	purpose []string,
	tag []string,
	keyword string,
	filterRuleKind string,
	filterLibRuleKind string,
	page int,
	limit int64,
) ([]*OnlineSyntaxFlowRule, *OnlinePaging, error) {
	raw, err := json.Marshal(DownloadOnlineSyntaxFlowRuleRequest{
		AfterId:           afterId,
		BeforeId:          beforeId,
		OrderBy:           "",
		Order:             "",
		Page:              int64(page),
		Limit:             limit,
		RuleNames:         ruleNames,
		Language:          language,
		GroupNames:        groupNames,
		Severity:          severity,
		Purpose:           purpose,
		Tag:               tag,
		Keyword:           keyword,
		FilterRuleKind:    filterRuleKind,
		FilterLibRuleKind: filterLibRuleKind,
	})
	if err != nil {
		return nil, nil, utils.Errorf("marshal params failed: %s", err)
	}
	rsp, _, err := poc.DoPOST(
		fmt.Sprintf("%v/%v", consts.GetOnlineBaseUrl(), "api/flow/rule"),
		poc.WithReplaceHttpPacketHeader("Authorization", token),
		poc.WithReplaceHttpPacketHeader("Content-Type", "application/json"),
		poc.WithReplaceHttpPacketBody(raw, false),
		poc.WithProxy(consts.GetOnlineBaseUrlProxy()),
		poc.WithSave(false),
	)
	if err != nil {
		return nil, nil, utils.Errorf("SyntaxFlowRule UploadToOnline failed: http error")
	}
	rawResponse := lowhttp.GetHTTPPacketBody(rsp.RawPacket)

	type SyntaxFlowRuleDownloadResponse struct {
		Data     []*OnlineSyntaxFlowRule `json:"data"`
		Pagemeta *OnlinePaging           `json:"pagemeta"`
	}
	type OnlineErr struct {
		Form   string `json:"form"`
		Reason string `json:"reason"`
		Ok     bool   `json:"ok"`
	}
	var _container SyntaxFlowRuleDownloadResponse
	var ret OnlineErr
	err = json.Unmarshal(rawResponse, &_container)
	if err != nil {
		return nil, nil, utils.Errorf("unmarshal SyntaxFlowRule response failed: %s", err.Error())
	}
	err = json.Unmarshal(rawResponse, &ret)
	if ret.Reason != "" {
		return nil, nil, utils.Errorf("unmarshal SyntaxFlowRule response failed: %s", ret.Reason)
	}
	return _container.Data, _container.Pagemeta, nil
}

func (s *OnlineClient) SaveSyntaxFlowRule(db *gorm.DB, rule ...*OnlineSyntaxFlowRule) error {
	if db == nil {
		return utils.Error("empty database")
	}
	for _, i := range rule {
		y := &schema.SyntaxFlowRule{
			IsBuildInRule: i.IsBuildInRule,
			Language:      ssaconfig.Language(i.Language),
			RuleName:      i.RuleName,
			Title:         i.Title,
			TitleZh:       i.TitleZh,
			Description:   i.Description,
			Tag:           i.Tag,
			AlertDesc:     i.AlertDesc,
			CVE:           i.CVE,
			RiskType:      i.RiskType,
			Type:          schema.SyntaxFlowRuleType(i.Type),
			Severity:      schema.SyntaxFlowSeverity(i.Severity),
			Content:       i.Content,
			Purpose:       schema.SyntaxFlowRulePurposeType(i.Purpose),
			Verified:      i.Verified,
			AllowIncluded: i.AllowIncluded,
			Hash:          i.Hash,
			RuleId:        i.RuleId,
			Version:       i.Version,
		}

		err := sfdb.DeleteSyntaxFlowRuleByRuleNameOrRuleId(i.RuleName, i.RuleId)
		if err != nil {
			log.Errorf("save [%s] to local failed: %s", i.RuleName, err)
		}

		_, err = sfdb.CreateOrUpdateRuleWithGroup(y, i.GroupName...)
		if err != nil {
			log.Errorf("save [%s] to local failed: %s", i.RuleName, err)
			return err
		}
	}
	return nil
}
