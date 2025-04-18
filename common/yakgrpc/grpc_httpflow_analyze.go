package yakgrpc

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

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
)

type HTTPFlowAnalyzeRequestStream interface {
	Send(response *ypb.AnalyzeHTTPFlowResponse) error
	Context() context.Context
}

type wrapperHTTPFlowAnalyzeStream struct {
	ctx            context.Context
	root           ypb.Yak_AnalyzeHTTPFlowServer
	RequestHandler func(request *ypb.AnalyzeHTTPFlowRequest) bool
	sendMutex      *sync.Mutex
}

func newWrapperHTTPFlowAnalyzeStream(ctx context.Context, stream ypb.Yak_AnalyzeHTTPFlowServer) *wrapperHTTPFlowAnalyzeStream {
	return &wrapperHTTPFlowAnalyzeStream{
		root: stream, ctx: ctx,
		sendMutex: new(sync.Mutex),
	}
}

func (w *wrapperHTTPFlowAnalyzeStream) Send(r *ypb.AnalyzeHTTPFlowResponse) error {
	w.sendMutex.Lock()
	defer w.sendMutex.Unlock()
	return w.root.Send(r)
}

func (w *wrapperHTTPFlowAnalyzeStream) Context() context.Context {
	return w.ctx
}

type AnalyzeHTTPFlowSource string

const (
	AnalyzeHTTPFlowSourceDatabase  = "database"
	AnalyzeHTTPFlowSourceRawPacket = "rawpacket"
)

func (s *Server) AnalyzeHTTPFlow(request *ypb.AnalyzeHTTPFlowRequest, stream ypb.Yak_AnalyzeHTTPFlowServer) error {
	if request == nil {
		return utils.Errorf("AnalyzeHTTPFlow request is nil")
	}
	rules := request.GetReplacers()
	analyzedId := uuid.NewString()

	// 创建一个通道来收集错误
	errChan := make(chan error, 1)

	wrapperStream := newWrapperHTTPFlowAnalyzeStream(stream.Context(), stream)
	client := yaklib.NewVirtualYakitClientWithRuntimeID(func(result *ypb.ExecResult) error {
		result.RuntimeID = analyzedId
		select {
		case <-wrapperStream.Context().Done():
			return wrapperStream.Context().Err()
		default:
			return wrapperStream.Send(&ypb.AnalyzeHTTPFlowResponse{
				ExecResult: result,
			})
		}
	}, analyzedId)

	var opts []HTTPFlowAnalyzeMangerOption
	opts = append(opts, WithRules(rules...))
	if config := request.GetConfig(); config != nil {
		opts = append(opts, WithConcurrency(int(config.GetConcurrency())))
		opts = append(opts, WithDedup(config.GetEnableDeduplicate()))
	}
	if request.GetSource() != nil {
		opts = append(opts, WithDataSource(request.GetSource()))
	}

	manger, err := NewHTTPFlowAnalyzeManger(
		s.GetProfileDatabase(),
		stream.Context(),
		analyzedId,
		client,
		wrapperStream,
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

	go func() {
		err := manger.AnalyzeHTTPFlow(s.GetProjectDatabase())
		if err != nil {
			errChan <- err
		} else {
			errChan <- nil
		}
	}()

	select {
	case err := <-errChan:
		return err
	case <-stream.Context().Done():
		return stream.Context().Err()
	}
}

type HTTPFlowAnalyzeManger struct {
	*mitmReplacer
	analyzeId    string
	client       *yaklib.YakitClient
	stream       HTTPFlowAnalyzeRequestStream
	ctx          context.Context
	source       *ypb.AnalyzedDataSource // 分析流量的数据源
	pluginCaller *yak.MixPluginCaller    // for hot patch code exec
	concurrency  int                     // 并发处理数量
	dedup        bool                    // 是否对单条数据进行去重

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
		m.LoadRules(rules)
	}
}

func WithDedup(dedup bool) HTTPFlowAnalyzeMangerOption {
	return func(m *HTTPFlowAnalyzeManger) {
		m.dedup = dedup
	}
}

func WithDataSource(source *ypb.AnalyzedDataSource) HTTPFlowAnalyzeMangerOption {
	return func(m *HTTPFlowAnalyzeManger) {
		m.source = source
	}
}

func NewHTTPFlowAnalyzeManger(
	MITMReplacerConfigDb *gorm.DB,
	ctx context.Context,
	analyzeId string,
	client *yaklib.YakitClient,
	stream HTTPFlowAnalyzeRequestStream,
	opts ...HTTPFlowAnalyzeMangerOption,
) (*HTTPFlowAnalyzeManger, error) {
	m := &HTTPFlowAnalyzeManger{
		analyzeId:    analyzeId,
		mitmReplacer: NewMITMReplacerFromDB(MITMReplacerConfigDb),
		client:       client,
		stream:       stream,
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

func (m *HTTPFlowAnalyzeManger) AnalyzeHTTPFlow(db *gorm.DB) (errs error) {
	defer func() {
		m.notifyProcess(1)
		if err := recover(); err != nil {
			errs = utils.JoinErrors(errs, utils.Errorf("panic: %s", err))
			return
		}
	}()

	source := m.source
	if source == nil {
		return m.AnalyzeHTTPFlowFromDb(db)
	}

	if source.SourceType == AnalyzeHTTPFlowSourceDatabase {
		return m.AnalyzeHTTPFlowFromDb(db)
	} else if source.SourceType == AnalyzeHTTPFlowSourceRawPacket {
		return m.AnalyzeHTTPFlowFromRawPacket(db)
	} else {
		return utils.Errorf("unknown analyze source type: %s", source.SourceType)
	}
}

func (m *HTTPFlowAnalyzeManger) AnalyzeHTTPFlowFromRawPacket(db *gorm.DB) error {
	if m.source == nil {
		return utils.Errorf("analyze source is nil")
	}
	flow, err := yakit.CreateHTTPFlow(
		yakit.CreateHTTPFlowWithRequestRaw([]byte(m.source.GetRawRequest())),
		yakit.CreateHTTPFlowWithResponseRaw([]byte(m.source.GetRawResponse())),
		yakit.CreateHTTPFlowWithFromPlugin("流量分析"),
	)
	if err != nil {
		return err
	}
	// 存储分析的流量
	err = yakit.SaveHTTPFlow(db, flow)
	if err != nil {
		return err
	}
	// 处理流量
	m.ExecHotPatch(db, flow)
	err = m.ExecReplacerRule(db, flow)
	if err != nil {
		return err
	}
	m.notifyHandleFlowNum()
	m.notifyProcess(1)
	return nil
}

func (m *HTTPFlowAnalyzeManger) AnalyzeHTTPFlowFromDb(db *gorm.DB) (errs error) {
	totalCallBack := func(i int) {
		atomic.AddInt64(&m.allHTTPFlowCount, int64(i))
	}
	query := db
	if m.source != nil {
		query = yakit.FilterHTTPFlow(db, m.source.GetHTTPFlowFilter())
	}
	flowCh := yakit.YieldHTTPFlowsEx(query, m.ctx, totalCallBack)
	errChan := make(chan error, m.concurrency)
	swg := utils.NewSizedWaitGroup(m.concurrency, m.ctx)
	for flow := range flowCh {
		swg.Add(1)
		go func(f *schema.HTTPFlow) {
			defer swg.Done()
			// 处理websocket流量
			if flow.IsWebsocket {
				m.handleWebsocket(db, flow)
			}
			// hot patch
			m.ExecHotPatch(db, f)
			// mitm replace rule
			if err := m.ExecReplacerRule(db, f); err != nil {
				errChan <- err
			}
			// 处理完成后更新计数和进度
			atomic.AddInt64(&m.handledHTTPFlowCount, 1)
			m.notifyProcess(float64(atomic.LoadInt64(&m.handledHTTPFlowCount)) / float64(atomic.LoadInt64(&m.allHTTPFlowCount)))
			m.notifyHandleFlowNum()
		}(flow)
	}
	swg.Wait()
	close(errChan)
	for err := range errChan {
		errs = utils.JoinErrors(errs, err)
	}
	return errs
}

func (m *HTTPFlowAnalyzeManger) handleWebsocket(db *gorm.DB, flow *schema.HTTPFlow) error {
	if !flow.IsWebsocket {
		return nil
	}
	// 处理websocket流量
	subQuery := db
	wsFlows, err := yakit.QueryAllWebsocketFlowByWebsocketHash(subQuery, flow.WebsocketHash)
	if err != nil {
		return err
	}
	handleColorAndTag := func(rule *MITMReplaceRule, wsFlow *schema.WebsocketFlow) {
		err := yakit.HandleAnalyzedHTTPFlowsColorAndTag(
			db,
			flow,
			rule.GetColor(),
			rule.GetExtraTag()...,
		)
		if err != nil {
			log.Infof("handle analyzed http flows color and tag failed: %s", err)
		}
		yakit.HandleAnalyzedWebsocketFlowsColorAndTag(db, wsFlow, rule.GetColor(), rule.GetExtraTag()...)
	}
	for _, wsFlow := range wsFlows {
		for _, rule := range m._mirrorRules {
			if !rule.EnableForRequest && !rule.EnableForResponse {
				continue
			}
			data, err := strconv.Unquote(wsFlow.QuotedData)
			if err != nil {
				log.Errorf("unquote websocket data failed: %v", err)
				continue
			}
			match, err := rule.matchRawSimple([]byte(data))
			if err != nil {
				log.Errorf("match package failed: %v", err)
				continue
			}
			if !match {
				continue
			}
			handleColorAndTag(rule, wsFlow)
		}
	}
	return nil
}

func (m *HTTPFlowAnalyzeManger) ExecHotPatch(db *gorm.DB, flow *schema.HTTPFlow) {
	extract := func(ruleName string, flow *schema.HTTPFlow) {
		yakit.UpdateHTTPFlowTags(db, flow)
		analyzed := &schema.AnalyzedHTTPFlow{
			ResultId:        m.analyzeId,
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

func (m *HTTPFlowAnalyzeManger) ExecReplacerRule(db *gorm.DB, flow *schema.HTTPFlow) error {
	if !m.haveRules() {
		return nil
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
			ResultId:        m.analyzeId,
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
	for _, rule := range m._mirrorRules {
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
