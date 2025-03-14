package yakgrpc

import (
	"context"
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/model"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type HTTPFlowAnalyzeManger struct {
	*mitmReplacer
}

func NewDefaultHTTPFlowAnalyzeManger(gorm *gorm.DB) *HTTPFlowAnalyzeManger {
	return &HTTPFlowAnalyzeManger{
		mitmReplacer: NewMITMReplacerFromDB(gorm),
	}
}

func (m *HTTPFlowAnalyzeManger) AnalyzeHTTPFlow(db *gorm.DB, ctx context.Context) (string, error) {
	if !m.haveRules() {
		return "", utils.Error("analyze rules is empty")
	}
	var (
		extractedResults []*schema.ExtractedData
		analyzeResults   []*schema.AnalyzedHTTPFlow
	)
	analyzeId := uuid.NewString()
	for _, rule := range m.rules {
		re, err := rule.compile()
		if err != nil {
			continue
		}
		pattern := re.String()
		analyzeResult := &schema.AnalyzedHTTPFlow{
			ResultId:        analyzeId,
			Rule:            rule.GetRule(),
			RuleVerboseName: rule.GetVerboseName(),
		}
		if rule.EnableForRequest {
			for flow := range yakit.QueryHTTPFlowsByRegexRequest(db, ctx, pattern, rule.EffectiveURL) {
				fullReq, err := model.ToHTTPFlowGRPCModelFull(flow)
				if err != nil {
					continue
				}
				_, matcheds, err := rule.MatchPacket(fullReq.Request, true)
				if err != nil {
					log.Infof("match packet failed: %s", err)
					continue
				}
				for _, matched := range matcheds {
					extractedResults = append(
						extractedResults,
						yakit.ExtractedDataFromHTTPFlow(
							flow.HiddenIndex,
							rule.VerboseName,
							matched,
							pattern,
						),
					)
				}
				analyzeResult.HTTPFlows = append(analyzeResult.HTTPFlows, flow)
			}
		}
		if rule.EnableForResponse {
			for flow := range yakit.QueryHTTPFlowsByRegexResponse(db, ctx, pattern, rule.EffectiveURL) {
				fullRsp, err := model.ToHTTPFlowGRPCModelFull(flow)
				if err != nil {
					continue
				}
				_, matcheds, err := rule.MatchPacket(fullRsp.Response, false)
				if err != nil {
					log.Infof("match packet failed: %s", err)
					continue
				}
				for _, matched := range matcheds {
					extractedResults = append(
						extractedResults,
						yakit.ExtractedDataFromHTTPFlow(
							flow.HiddenIndex,
							rule.VerboseName,
							matched,
							pattern,
						),
					)
				}
				analyzeResult.HTTPFlows = append(analyzeResult.HTTPFlows, flow)
			}
		}
		// handle color and tag
		err = yakit.HandleAnalyzedHTTPFlowsColorAndTag(
			db,
			analyzeResult.HTTPFlows,
			rule.GetColor(),
			rule.GetExtraTag()...,
		)
		if err != nil {
			log.Infof("handle analyzed http flows color and tag failed: %s", err)
		}
		analyzeResults = append(analyzeResults, analyzeResult)
	}
	// save analyzeResults
	var errs error
	for _, a := range analyzeResults {
		err := db.Save(&a).Error
		errs = utils.JoinErrors(errs, err)
	}
	// save extractedResults
	for _, e := range extractedResults {
		err := yakit.CreateOrUpdateExtractedDataEx(-1, e)
		if err != nil {
			log.Infof("create or update extracted data failed: %s", err)
		}
	}
	return analyzeId, errs
}

func (s *Server) AnalyzeHTTPFlow(ctx context.Context, req *ypb.AnalyzeHTTPFlowRequest) (*ypb.AnalyzeHTTPFlowResponse, error) {
	analyzed := NewDefaultHTTPFlowAnalyzeManger(s.GetProfileDatabase())
	analyzedId, err := analyzed.AnalyzeHTTPFlow(s.GetProjectDatabase(), ctx)
	if err != nil {
		return nil, err
	}
	return &ypb.AnalyzeHTTPFlowResponse{
		AnalyzeId: analyzedId,
	}, nil
}

func (s *Server) QueryAnalyzedHTTPFlowRule(ctx context.Context, req *ypb.QueryAnalyzedHTTPFlowRuleRequest) (*ypb.QueryAnalyzedHTTPFlowRuleResponse, error) {
	p, data, err := yakit.QueryAnalyzedHTTPFlowRule(s.GetProjectDatabase(), req)
	if err != nil {
		return nil, err
	}
	rsp := &ypb.QueryAnalyzedHTTPFlowRuleResponse{
		Pagination: req.GetPagination(),
		Total:      int64(p.TotalRecord),
	}
	for _, d := range data {
		rsp.Data = append(rsp.Data, d.ToGRPCModel())
	}
	return rsp, nil
}
