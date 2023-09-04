package chaosmaker

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net"
	"strconv"
	"strings"
)

type ChaosMaker struct {
	LocalIPAddress string
	ChaosRules     []*rule.Storage
}

func NewChaosMakerWithRules(rules []*rule.Storage) *ChaosMaker {
	return &ChaosMaker{ChaosRules: rules, LocalIPAddress: utils.GetLocalIPAddress()}
}

func NewChaosMaker() *ChaosMaker {
	return &ChaosMaker{LocalIPAddress: utils.GetLocalIPAddress()}
}

func (c *ChaosMaker) FeedRule(a ...*rule.Storage) {
	c.ChaosRules = append(c.ChaosRules, a...)
}

func (c *ChaosMaker) ApplyAll() error {
	for r := range rule.YieldRules(
		consts.GetGormProfileDatabase().Model(&rule.Storage{}),
		context.Background(),
	) {
		c.FeedRule(r)
	}
	return nil
}

func (c *ChaosMaker) Generate() chan []byte {
	fChan := make(chan []byte)
	go func() {
		defer close(fChan)
		for _, r := range c.ChaosRules {
			ch, err := c.generate(r)
			if err != nil {
				log.Errorf("generate traffic failed: %v", err)
				continue
			}
			if ch == nil {
				continue
			}

			for t := range ch {
				fChan <- t
			}
		}
	}()
	return fChan
}

func (c *ChaosMaker) generate(r *rule.Storage) (chan []byte, error) {
	switch strings.ToLower(r.RuleType) {
	case "suricata":
		return c.suricataGenerate(r)
	case "http-request":
		return c.httpreqGenerate(r)
	case "tcp":
		//TODO: 这里现在还没法处理TCP raw数据
		//raw, err := codec.DecodeBase64(r.RawTrafficBeyondIPPacketBase64)
		//if err != nil {
		//	return nil, err
		//}

		ch := make(chan []byte, 1)
		//ch <- &ChaosTraffic{
		//	ChaosRule:    r,
		//	TCPIPPayload: raw,
		//	RawTCP:       true,
		//}
		close(ch)
		return ch, nil
	case "icmp":
		return nil, utils.Error("icmp not implemented")
	default:
		return nil, utils.Errorf("unknown rule type: %s", r.RuleType)
	}
}

func (c *ChaosMaker) httpreqGenerate(r *rule.Storage) (chan []byte, error) {
	raw, err := codec.DecodeBase64(r.RawTrafficBeyondHTTPBase64)
	if err != nil {
		return nil, err
	}
	output := make(chan []byte, 128)
	go func() {
		defer close(output)
		for i := 0; i < 5; i++ {
			flows, err := pcapx.CreateTCPFlowFromPayload(net.JoinHostPort(utils.GetLocalIPAddress(), strconv.Itoa(utils.GetRandomAvailableTCPPort())), net.JoinHostPort(utils.GetRandomIPAddress(), "80"), raw)
			if err != nil {
				log.Warnf("build packet failed: %v", err)
				continue
			}
			for _, pk := range flows {
				output <- pk
			}
		}
	}()
	return output, nil
}

func (c *ChaosMaker) suricataGenerate(originRule *rule.Storage) (chan []byte, error) {
	if originRule == nil {
		return nil, utils.Error("rule is nil")
	}

	rules, err := surirule.Parse(originRule.SuricataRaw)
	if err != nil {
		return nil, utils.Errorf("parse suricata rule failed: %v", err)
	}

	mapRule := fmt.Sprintf("%s-%s", originRule.RuleType, originRule.Protocol)
	handler, ok := chaosMap.Load(mapRule)
	if !ok {
		return nil, utils.Errorf("cannot found protocol %s", mapRule)
	}

	h, ok := handler.(chaosHandler)
	if !ok {
		return nil, utils.Errorf("cannot convert %v to chaosHandler", handler)
	}

	if len(rules) == 1 {
		return h.Generator(c, originRule, rules[0]), nil
	}

	var chans []chan []byte
	for _, r := range rules {
		ch := h.Generator(c, originRule, r)
		if ch == nil {
			log.Errorf("rule: %v's generator is empty!", r.Message)
			continue
		}
		chans = append(chans, ch)
	}

	if len(chans) > 0 {
		return mergeChans(chans...), nil
	}
	return nil, utils.Errorf("no traffic generator found for %d rules", len(rules))
}

func mergeChans[T any](chans ...chan T) chan T {
	merged := make(chan T)
	go func() {
		defer func() {
			close(merged)
		}()
		for _, ch := range chans {
			for t := range ch {
				merged <- t
			}
		}
	}()
	return merged
}
