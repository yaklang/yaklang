package yakgrpc

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/chaosmaker"
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"sync/atomic"
	"time"
)

func (s *Server) ImportChaosMakerRules(ctx context.Context, req *ypb.ImportChaosMakerRulesRequest) (*ypb.Empty, error) {
	content := req.GetContent()
	if strings.TrimSpace(content) == "" {
		return nil, utils.Error("empty content")
	}

	var rules []*rule.Storage
	switch strings.ToLower(req.GetRuleType()) {
	case "suricata":
		log.Infof("start to load suricata rules from content-len: %v", utils.ByteSize(uint64(len(req.GetContent()))))
		rules = chaosmaker.ParseRuleFromRawSuricataRules(req.GetContent())
	case "http-request":
		rules = chaosmaker.ParseRuleFromHTTPRequestRawJSON(req.GetContent())
	}
	log.Infof("load suricata rules finished! fetch rule: %v", len(rules))
	for _, i := range rules {
		rule.UpsertRule(consts.GetGormProfileDatabase(), i.CalcHash(), i)
	}
	return &ypb.Empty{}, nil
}

func (s *Server) QueryChaosMakerRule(ctx context.Context, req *ypb.QueryChaosMakerRuleRequest) (*ypb.QueryChaosMakerRuleResponse, error) {
	p, res, err := rule.QueryRule(consts.GetGormProfileDatabase(), req)
	if err != nil {
		return nil, utils.Errorf("QueryRule failed: %s", err)
	}
	return &ypb.QueryChaosMakerRuleResponse{
		Pagination: req.GetPagination(),
		Total:      int64(p.TotalRecord),
		Data: funk.Map(res, func(i *rule.Storage) *ypb.ChaosMakerRule {
			return i.ToGPRCModel()
		}).([]*ypb.ChaosMakerRule),
	}, nil
}

func (s *Server) DeleteChaosMakerRuleByID(ctx context.Context, req *ypb.DeleteChaosMakerRuleByIDRequest) (*ypb.Empty, error) {
	err := rule.DeleteSuricataRuleByID(consts.GetGormProfileDatabase(), req.GetId())
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) ExecuteChaosMakerRule(req *ypb.ExecuteChaosMakerRuleRequest, stream ypb.Yak_ExecuteChaosMakerRuleServer) error {
	sendLog := func(level, msg string, items ...interface{}) {
		stream.Send(yaklib.NewYakitLogExecResult("info", msg, items...))
	}
	groups := req.GetGroups()
	if len(groups) == 0 {
		sendLog("日志", "没有指定分组，将使用全部数据")
	}

	delayer, err := utils.NewDelayWaiter(req.GetTrafficDelayMinSeconds(), req.GetTrafficDelayMaxSeconds())
	if err != nil {
		return utils.Errorf("create delayer failed: %s", err)
	}

	var trafficCounter int64 = 0
	var addTrafficCounter = func() {
		atomic.AddInt64(&trafficCounter, 1)
	}
	var start = time.Now()
	sendLogger := yaklib.NewVirtualYakitClient(func(i interface{}) error {
		if i == nil {
			return nil
		}
		if ret, ok := i.(*yaklib.YakitLog); ok {
			raw, _ := yaklib.YakitMessageGenerator(ret)
			if raw != nil {
				if err := stream.Send(&ypb.ExecResult{
					IsMessage: true,
					Message:   raw,
				}); err != nil {
					return err
				}
			}
		}
		return nil
	})
	go func() {
		for {
			select {
			case <-stream.Context().Done():
				return
			default:
				sendLogger.Output(&yaklib.YakitStatusCard{
					Id:   "已运行",
					Data: fmt.Sprintf("%ds", int64(time.Now().Sub(start).Seconds())),
				})
				sendLogger.Output(&yaklib.YakitStatusCard{
					Id:   "模拟攻击事件",
					Data: fmt.Sprintf("%d", trafficCounter),
				})
				time.Sleep(500 * time.Millisecond)
			}
		}
	}()

	var handle = func() {
		concurrent := req.GetConcurrent()
		if concurrent <= 0 {
			concurrent = 30
		}
		swg := utils.NewSizedWaitGroup(int(concurrent))
		if len(req.GetGroups()) > 0 {
			for _, group := range req.GetGroups() {
				generator := chaosmaker.NewChaosMaker()
				sendLog("info", "开始加载模拟场景，关键字: %v, 协议: %v", group.Keywords, group.Protocols)
				for rule := range chaosmaker.YieldRulesByKeywords(group.Keywords, group.Protocols...) {
					generator.FeedRule(rule)
				}
				sendLog("info", "模拟场景加载完成，共加载规则: %v", len(generator.ChaosRules))
				for traffic := range generator.Generate() {
					addTrafficCounter()
					traffic := traffic
					swg.Add()
					go func() {
						defer swg.Done()
						pcapx.InjectChaosTraffic(traffic)
						delayer.Wait()
						for _, r := range req.GetExtraOverrideDestinationAddress() {
							pcapx.InjectChaosTraffic(traffic, pcapx.WithRemoteAddress(r))
							delayer.Wait()
						}
					}()
				}
			}
		} else {
			generator := chaosmaker.NewChaosMaker()
			sendLog("info", "开始加载全部模拟攻击剧本")
			for rule := range rule.YieldRules(consts.GetGormProfileDatabase(), stream.Context()) {
				generator.FeedRule(rule)
			}
			sendLog("info", "模拟场景加载完成，共加载规则: %v", len(generator.ChaosRules))
			for traffic := range generator.Generate() {
				traffic := traffic
				swg.Add()
				addTrafficCounter()
				go func() {
					defer swg.Done()
					pcapx.InjectChaosTraffic(traffic)
					delayer.Wait()
					for _, r := range req.GetExtraOverrideDestinationAddress() {
						pcapx.InjectChaosTraffic(traffic, pcapx.WithRemoteAddress(r))
						delayer.Wait()
					}
				}()
			}
		}
		swg.Wait()
		sendLog("info", "本地模拟攻击剧本执行完成")
	}

	if req.GetExtraRepeat() >= 0 {
		for _index := 0; _index < int(req.GetExtraRepeat())+1; _index++ {
			sendLog("info", "开始进行第%v次攻击模拟", _index)
			handle()
		}
	} else {
		count := 0
		for {
			select {
			case <-stream.Context().Done():
				return nil
			default:
			}
			count++
			sendLog("info", "开始进行第%v次攻击模拟", count)
			handle()
		}
	}
	return nil
}
