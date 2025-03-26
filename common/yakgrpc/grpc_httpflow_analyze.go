package yakgrpc

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/model"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) AnalyzeHTTPFlow(request *ypb.AnalyzeHTTPFlowRequest, stream ypb.Yak_AnalyzeHTTPFlowServer) error {
	if request == nil {
		return utils.Errorf("AnalyzeHTTPFlow request is nil")
	}
	rules := request.GetReplacers()
	analyzedId := uuid.NewString()
	client := yaklib.NewVirtualYakitClientWithRuntimeID(func(result *ypb.ExecResult) error {
		result.RuntimeID = analyzedId
		return stream.Send(&ypb.AnalyzeHTTPFlowResponse{
			ExecResult: result,
		})
	}, analyzedId)
	manger, err := NewHTTPFlowAnalyzeManger(s.GetProfileDatabase(), stream.Context(), client, stream, rules...)
	if err != nil {
		return err
	}

	if request.GetHotPatchCode() != "" {
		err = manger.pluginCaller.LoadHotPatch(manger.ctx, nil, request.GetHotPatchCode())
		if err != nil {
			return err
		}
	}

	err = manger.AnalyzeHTTPFlow(s.GetProjectDatabase(), analyzedId)
	if err != nil {
		return err
	}
	return nil
}

type HTTPFlowAnalyzeManger struct {
	*mitmReplacer
	client                 *yaklib.YakitClient
	stream                 ypb.Yak_AnalyzeHTTPFlowServer
	ctx                    context.Context
	matchedHTTPFlowCount   int64
	extractedHTTPFlowCount int64
	pluginCaller           *yak.MixPluginCaller // for hot patch code exec

	allHTTPFlowCount     int64
	handledHTTPFlowCount int64
}

func NewHTTPFlowAnalyzeManger(
	gorm *gorm.DB,
	ctx context.Context,
	client *yaklib.YakitClient,
	steam ypb.Yak_AnalyzeHTTPFlowServer,
	rules ...*ypb.MITMContentReplacer,
) (*HTTPFlowAnalyzeManger, error) {
	m := &HTTPFlowAnalyzeManger{
		mitmReplacer: NewMITMReplacerFromDB(gorm),
		client:       client,
		stream:       steam,
		ctx:          ctx,
	}
	m.SetRules(rules...)
	caller, err := yak.NewMixPluginCaller()
	if err != nil {
		return nil, err
	}
	caller.SetDividedContext(true)
	caller.SetConcurrent(20)
	caller.SetLoadPluginTimeout(10)
	caller.SetCallPluginTimeout(consts.GetGlobalCallerCallPluginTimeout())
	m.pluginCaller = caller
	return m, nil
}

func (m *HTTPFlowAnalyzeManger) AnalyzeHTTPFlow(db *gorm.DB, analyzeId string) (errs error) {
	defer func() {
		m.notifyProcess(1)
		if err := recover(); err != nil {
			errs = utils.JoinErrors(errs, utils.Errorf("panic: %s", err))
			return
		}
	}()

	totalCallBack := func(i int) {
		m.allHTTPFlowCount += int64(i)
	}
	flowCh := yakit.YieldHTTPFlowsEx(db, m.ctx, totalCallBack)
	extract := func(ruleName string, flow *schema.HTTPFlow) {
		yakit.UpdateHTTPFlowTags(db, flow)
		analyzed := &schema.AnalyzedHTTPFlow{
			ResultId:        analyzeId,
			Rule:            "热加载规则",
			RuleVerboseName: ruleName,
			HTTPFlowId:      int64(flow.ID),
		}
		err := db.Save(analyzed).Error
		if err != nil {
			log.Infof("save analyze result failed: %s", err)
		}
		m.handledHTTPFlowCount++
		m.matchedHTTPFlowCount++
		m.notifyResult(analyzed)
		m.notifyHandleFlowNum()
		m.notifyMatchedHTTPFlowNum()
	}
	var count float64
	for flow := range flowCh {
		count++
		m.notifyProcess(count / float64(m.allHTTPFlowCount))
		m.notifyHandleFlowNum()
		// hot patch
		if m.pluginCaller != nil {
			m.pluginCaller.CallAnalyzeHTTPFlow(m.ctx, flow, extract)
		}
		// mitm replace rule
		err := m.ExecReplacerRule(db, flow, analyzeId)
		if err != nil {
			errs = utils.JoinErrors(errs, err)
			continue
		}
	}
	return errs
}

func (m *HTTPFlowAnalyzeManger) ExecReplacerRule(db *gorm.DB, flow *schema.HTTPFlow, analyzeId string) error {
	if !m.haveRules() {
		return utils.Error("analyze rules is empty")
	}
	extractData := func(pattern string, rule *MITMReplaceRule, flow *schema.HTTPFlow, isReq bool) []schema.ExtractedData {
		modelFull, err := model.ToHTTPFlowGRPCModelFull(flow)
		if err != nil {
			return nil
		}
		var packet []byte
		if isReq {
			packet = modelFull.Request
		} else {
			packet = modelFull.Response
		}
		_, matcheds, err := rule.MatchPacket(packet, isReq)
		if err != nil {
			log.Infof("match packet failed: %s", err)
			return nil
		}

		var extractedData []schema.ExtractedData
		for _, matched := range matcheds {
			e := yakit.ExtractedDataFromHTTPFlow(
				flow.HiddenIndex,
				rule.VerboseName,
				matched,
				pattern,
			)
			// save extracted data
			err = yakit.CreateOrUpdateExtractedDataEx(-1, e)
			if err != nil {
				log.Infof("create or update extracted data failed: %s", err)
				continue
			}
			m.extractedHTTPFlowCount++
			m.notifyExtractedHTTPFlowNum()
			extractedData = append(extractedData, *e)
		}
		return extractedData
	}
	getAnalyzedHTTPFlow := func(rule *MITMReplaceRule, flow *schema.HTTPFlow) *schema.AnalyzedHTTPFlow {
		m.matchedHTTPFlowCount++
		m.notifyMatchedHTTPFlowNum()
		analyzeResult := &schema.AnalyzedHTTPFlow{
			ResultId:        analyzeId,
			Rule:            rule.GetRule(),
			RuleVerboseName: rule.VerboseName,
		}
		analyzeResult.HTTPFlowId = int64(flow.ID)
		return analyzeResult
	}
	saveAnalyzedHTTPFlow := func(result *schema.AnalyzedHTTPFlow) {
		err := db.Save(result).Error
		if err != nil {
			log.Infof("save analyze result failed: %s", err)
		}
		m.handledHTTPFlowCount++
		m.notifyHandleFlowNum()
		m.notifyResult(result)
	}
	handleColorAndTag := func(rule *MITMReplaceRule, flow *schema.HTTPFlow) {
		err := yakit.HandleAnalyzedHTTPFlowsColorAndTag(
			db,
			flow,
			rule.GetColor(),
			rule.GetExtraTag()...,
		)
		if err != nil {
			log.Infof("handle analyzed http flows color and tag failed: %s", err)
		}
	}
	for _, rule := range m.rules {
		re, err := rule.compile()
		if err != nil {
			continue
		}
		pattern := re.String()
		if rule.EffectiveURL != "" {
			yakRegexp := regexp_utils.DefaultYakRegexpManager.GetYakRegexp(rule.EffectiveURL)
			matchString, _ := yakRegexp.MatchString(flow.Url)
			if !matchString {
				continue
			}
		}

		if rule.EnableForRequest {
			match, _ := re.MatchString(flow.Request)
			if match {
				result := getAnalyzedHTTPFlow(rule, flow)
				if result == nil {
					continue
				}
				extracts := extractData(pattern, rule, flow, true)
				result.ExtractedData = extracts
				saveAnalyzedHTTPFlow(result)
				handleColorAndTag(rule, flow)
			}
		}

		if rule.EnableForResponse {
			match, _ := re.MatchString(flow.Response)
			if match {
				result := getAnalyzedHTTPFlow(rule, flow)
				if result == nil {
					continue
				}
				extracts := extractData(pattern, rule, flow, false)
				result.ExtractedData = extracts
				saveAnalyzedHTTPFlow(result)
				handleColorAndTag(rule, flow)
			}
		}
	}
	return nil
}

func (m *HTTPFlowAnalyzeManger) notifyProcess(process float64) {
	m.client.YakitSetProgress(process)
}

func (m *HTTPFlowAnalyzeManger) notifyMatchedHTTPFlowNum() {
	m.client.StatusCard("符合条件数", m.matchedHTTPFlowCount)
}

func (m *HTTPFlowAnalyzeManger) notifyExtractedHTTPFlowNum() {
	m.client.StatusCard("提取数据", m.extractedHTTPFlowCount)
}

func (m *HTTPFlowAnalyzeManger) notifyHandleFlowNum() {
	m.client.StatusCard("已处理数/总数", fmt.Sprintf("%d/%d", m.handledHTTPFlowCount, m.allHTTPFlowCount))
}

func (m *HTTPFlowAnalyzeManger) notifyResult(result *schema.AnalyzedHTTPFlow) {
	m.stream.Send(&ypb.AnalyzeHTTPFlowResponse{
		RuleData: result.ToGRPCModel(),
	})
}
