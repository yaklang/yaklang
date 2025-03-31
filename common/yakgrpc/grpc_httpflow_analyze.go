package yakgrpc

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/model"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"sync/atomic"
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

	var opts []HTTPFlowAnalyzeMangerOption
	opts = append(opts, WithRules(rules...))
	if config := request.GetConfig(); config != nil {
		opts = append(opts, WithConcurrency(int(config.GetConcurrency())))
		opts = append(opts, WithDedup(config.GetEnableDeduplicate()))
	}

	manger, err := NewHTTPFlowAnalyzeManger(
		s.GetProfileDatabase(),
		stream.Context(),
		client,
		stream,
		opts...,
	)
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
	client       *yaklib.YakitClient
	stream       ypb.Yak_AnalyzeHTTPFlowServer
	ctx          context.Context
	pluginCaller *yak.MixPluginCaller // for hot patch code exec
	concurrency  int                  // 并发处理数量
	dedup        bool                 // 是否对单条数据进行去重

	matchedHTTPFlowCount   int64
	extractedHTTPFlowCount int64
	allHTTPFlowCount       int64
	handledHTTPFlowCount   int64
}

type HTTPFlowAnalyzeMangerOption func(*HTTPFlowAnalyzeManger)

func WithConcurrency(concurrency int) HTTPFlowAnalyzeMangerOption {
	return func(m *HTTPFlowAnalyzeManger) {
		if concurrency <= 0 {
			concurrency = 10 // 默认并发数
		}
		m.concurrency = concurrency
	}
}

func WithRules(rules ...*ypb.MITMContentReplacer) HTTPFlowAnalyzeMangerOption {
	return func(m *HTTPFlowAnalyzeManger) {
		m.SetRules(rules...)
	}
}

func WithDedup(dedup bool) HTTPFlowAnalyzeMangerOption {
	return func(m *HTTPFlowAnalyzeManger) {
		m.dedup = dedup
	}
}

func NewHTTPFlowAnalyzeManger(
	gorm *gorm.DB,
	ctx context.Context,
	client *yaklib.YakitClient,
	steam ypb.Yak_AnalyzeHTTPFlowServer,
	opts ...HTTPFlowAnalyzeMangerOption,
) (*HTTPFlowAnalyzeManger, error) {
	m := &HTTPFlowAnalyzeManger{
		mitmReplacer: NewMITMReplacerFromDB(gorm),
		client:       client,
		stream:       steam,
		ctx:          ctx,
		concurrency:  10,    // 默认并发数
		dedup:        false, // 默认不去重
	}

	for _, opt := range opts {
		opt(m)
	}

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
		atomic.AddInt64(&m.allHTTPFlowCount, int64(i))
	}
	flowCh := yakit.YieldHTTPFlowsEx(db, m.ctx, totalCallBack)

	errChan := make(chan error, m.concurrency)
	swg := utils.NewSizedWaitGroup(m.concurrency, m.ctx)
	for flow := range flowCh {
		swg.Add(1)
		go func(f *schema.HTTPFlow) {
			defer swg.Done()
			atomic.AddInt64(&m.handledHTTPFlowCount, 1)
			m.notifyProcess(float64(atomic.LoadInt64(&m.handledHTTPFlowCount)) / float64(atomic.LoadInt64(&m.allHTTPFlowCount)))
			m.notifyHandleFlowNum()
			// hot patch
			m.ExecHotPatch(db, analyzeId, f)
			// mitm replace rule
			if err := m.ExecReplacerRule(db, f, analyzeId); err != nil {
				errChan <- err
			}
		}(flow)
	}

	swg.Wait()
	close(errChan)
	for err := range errChan {
		errs = utils.JoinErrors(errs, err)
	}
	return errs
}

func (m *HTTPFlowAnalyzeManger) ExecHotPatch(db *gorm.DB, analyzeId string, flow *schema.HTTPFlow) {
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
		atomic.AddInt64(&m.matchedHTTPFlowCount, 1)
		m.notifyResult(analyzed, nil)
		m.notifyMatchedHTTPFlowNum()
	}

	if m.pluginCaller != nil {
		m.pluginCaller.CallAnalyzeHTTPFlow(m.ctx, flow, extract)
	}
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
		filter := filter.NewFilter()
		for _, matched := range matcheds {
			// 如果开启了去重，检查是否已经存在相同的数据
			if m.dedup {
				if filter.Exist(matched.MatchResult) {
					continue
				}
				filter.Insert(matched.MatchResult)
			}
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
			atomic.AddInt64(&m.extractedHTTPFlowCount, 1)
			m.notifyExtractedHTTPFlowNum()
			extractedData = append(extractedData, *e)
		}
		return extractedData
	}
	getAnalyzedHTTPFlow := func(rule *MITMReplaceRule, flow *schema.HTTPFlow) *schema.AnalyzedHTTPFlow {
		atomic.AddInt64(&m.matchedHTTPFlowCount, 1)
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
				extracts := extractData(pattern, rule, flow, true)
				if len(extracts) == 0 {
					continue
				}
				result := getAnalyzedHTTPFlow(rule, flow)
				result.ExtractedData = extracts
				saveAnalyzedHTTPFlow(result)
				m.notifyResult(result, extracts)
				handleColorAndTag(rule, flow)
			}
		}

		if rule.EnableForResponse {
			match, _ := re.MatchString(flow.Response)
			if match {
				extracts := extractData(pattern, rule, flow, false)
				if len(extracts) == 0 {
					continue
				}
				result := getAnalyzedHTTPFlow(rule, flow)
				result.ExtractedData = extracts
				saveAnalyzedHTTPFlow(result)
				m.notifyResult(result, extracts)
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
	m.client.StatusCard("符合条件数", atomic.LoadInt64(&m.matchedHTTPFlowCount))
}

func (m *HTTPFlowAnalyzeManger) notifyExtractedHTTPFlowNum() {
	m.client.StatusCard("提取数据", atomic.LoadInt64(&m.extractedHTTPFlowCount))
}

func (m *HTTPFlowAnalyzeManger) notifyHandleFlowNum() {
	m.client.StatusCard("已处理数/总数", fmt.Sprintf("%d/%d",
		atomic.LoadInt64(&m.handledHTTPFlowCount),
		atomic.LoadInt64(&m.allHTTPFlowCount)))
}

func (m *HTTPFlowAnalyzeManger) notifyResult(result *schema.AnalyzedHTTPFlow, extractedData []schema.ExtractedData) {
	var builder strings.Builder
	for i, e := range extractedData {
		if i > 0 {
			builder.WriteString(" | ")
		}
		builder.WriteString(e.Data)
	}
	content := builder.String()
	m.stream.Send(&ypb.AnalyzeHTTPFlowResponse{
		RuleData:         result.ToGRPCModel(),
		ExtractedContent: content,
	})
}
