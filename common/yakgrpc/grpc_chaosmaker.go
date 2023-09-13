package yakgrpc

import (
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/chaosmaker"
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/workpool"
	"github.com/yaklang/yaklang/common/vulinboxagentclient"
	"github.com/yaklang/yaklang/common/vulinboxagentproto"
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
	var matchCounter int64 = 0
	var addMatchCounter = func() {
		atomic.AddInt64(&matchCounter, 1)
	}

	var start = time.Now()
	sendLogger := yaklib.NewVirtualYakitClient(stream.Send)
	go func() {
		t := time.NewTicker(500 * time.Millisecond)
		for {
			select {
			case <-stream.Context().Done():
				t.Stop()
				return
			case <-t.C:
				sendLogger.Output(&yaklib.YakitStatusCard{
					Id:   "Agent命中流量",
					Data: fmt.Sprintf("%d", matchCounter),
				})
				sendLogger.Output(&yaklib.YakitStatusCard{
					Id:   "已运行",
					Data: fmt.Sprintf("%ds", int64(time.Now().Sub(start).Seconds())),
				})
				sendLogger.Output(&yaklib.YakitStatusCard{
					Id:   "模拟攻击事件",
					Data: fmt.Sprintf("%d", trafficCounter),
				})
			}
		}
	}()

	getRules := func() []*rule.Storage {
		var rules []*rule.Storage
		if len(req.GetGroups()) > 0 {
			for _, group := range req.GetGroups() {
				sendLog("info", "开始加载模拟场景，关键字: %v, 协议: %v", group.Keywords, group.Protocols)
				for r := range chaosmaker.YieldRulesByKeywords(group.Keywords, group.Protocols...) {
					rules = append(rules, r)
				}
			}
		} else {
			sendLog("info", "开始加载全部模拟攻击剧本")
			for r := range rule.YieldRules(consts.GetGormProfileDatabase(), stream.Context()) {
				rules = append(rules, r)
			}
		}
		return rules
	}

	var attackOnce = func(ctx context.Context) {
		concurrent := req.GetConcurrent()
		if concurrent <= 0 {
			concurrent = 30
		}

		sendLog("info", "正在初始化Agent")
		var clients []*vulinboxagentclient.Client
		vulinboxAgentMap.Range(func(key, value any) bool {
			agent, ok := value.(*VulinboxAgentFacade)
			if !ok {
				return true
			}
			clients = append(clients, agent.client)
			return true
		})

		// 暂时只支持 suricata
		rules := getRules()
		var suriraw []string
		for _, r := range rules {
			if r.SuricataRaw != "" {
				suriraw = append(suriraw, r.SuricataRaw)
			}
		}
		for _, client := range clients {
			client.Msg().Send(vulinboxagentproto.NewSubscribeAction("suricata", suriraw))
			client.RegisterDataback("suricata", func(data any) {
				spew.Dump(data)
				addMatchCounter()
			})
		}
		defer func() {
			for _, client := range clients {
				client.Msg().Send(vulinboxagentproto.NewUnsubscribeAction("suricata", suriraw))
			}
		}()

		sendLog("info", "开始执行本地模拟攻击剧本")
		swg := utils.NewSizedWaitGroup(int(concurrent))
		var generator = chaosmaker.NewChaosMaker()

		wp := workpool.New(int(concurrent), func(rec chan []byte) {
			for traffic := range rec {
				pcapx.InjectRaw(traffic)
				for _, r := range req.GetExtraOverrideDestinationAddress() {
					pcapx.InjectRaw(traffic, pcapx.WithRemoteAddress(r))
				}
				swg.Done()
			}
		})
		wp.Start()
		defer wp.Stop()

		generator.FeedRule(rules...)
		generator.SetContext(ctx)

		for pk := range generator.Generate() {
			addTrafficCounter()
			swg.Add()
			wp.AddJob(pk)
		}

		swg.Wait()
		sendLog("info", "本地模拟攻击剧本执行完成")
	}

	if req.GetExtraRepeat() >= 0 {
		select {
		case <-stream.Context().Done():
			return nil
		default:
		}
		for _index := 1; _index <= int(req.GetExtraRepeat()); _index++ {
			sendLog("info", "开始进行第%v次攻击模拟", _index)
			attackOnce(stream.Context())
			delayer.Wait()
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
			attackOnce(stream.Context())
		}
	}
	return nil
}
