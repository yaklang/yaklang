package match

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

/*

matcher := suricata.NewGroupMatcher(
	match.WithOnMatchedCallback(match => {
		// do something
	}),
)
matcher.LoadRulesWithQuery("baidu.com")~

pcapx.Sniff("", pcapx.pcap_everyFrame(data => {
	matcher.FeedFrame(data)
}))

*/

// Group is a group of rules
type Group struct {
	pkCache *utils.Cache[gopacket.Packet]
	loader  SuricataRuleLoaderType

	HTTPMatcher     []*sync.Pool
	OrdinaryMatcher []*sync.Pool

	// context
	ctx    context.Context
	cancel context.CancelFunc

	frameChan   chan gopacket.Packet
	httpRequest chan *HttpFlow

	consumeOnce *sync.Once

	onMatchedCallback func(packet gopacket.Packet, match *rule.Rule)

	// control waitgroup
	wg *sync.WaitGroup
}

type SuricataRuleLoaderType func(query string) (chan *rule.Rule, error)

var defaultSuricataRuleLoader SuricataRuleLoaderType = nil

func RegisterSuricataRuleLoader(h SuricataRuleLoaderType) {
	defaultSuricataRuleLoader = h
}

func NewGroup(opt ...GroupOption) *Group {
	gopacketCache := utils.NewTTLCache[gopacket.Packet](30 * time.Second)

	ctx, cancel := context.WithCancel(context.Background())

	group := &Group{
		pkCache: gopacketCache,
		loader:  defaultSuricataRuleLoader,

		// internal fields
		frameChan:   make(chan gopacket.Packet, 50000),
		httpRequest: make(chan *HttpFlow, 50000),
		consumeOnce: new(sync.Once),
		ctx:         ctx,
		cancel:      cancel,
		onMatchedCallback: func(packet gopacket.Packet, match *rule.Rule) {
			log.Infof("matched: %v", match.Raw)
		},
		wg: new(sync.WaitGroup),
	}
	for _, i := range opt {
		i(group)
	}
	group.consumeMain()
	return group
}

func (g *Group) SetLoader(loader SuricataRuleLoaderType) {
	g.loader = loader
}

func (g *Group) LoadRule(r *rule.Rule) {
	matcher := New(r)
	switch r.Protocol {
	case "http":
		g.HTTPMatcher = append(g.HTTPMatcher, &sync.Pool{New: func() any {
			return &Matcher{
				matcher: matcher.matcher.Clone(),
			}
		}})
	}
	g.OrdinaryMatcher = append(g.OrdinaryMatcher, &sync.Pool{New: func() any {
		return &Matcher{
			matcher: matcher.matcher.Clone(),
		}
	}})
}

func (g *Group) LoadRules(r ...*rule.Rule) {
	for _, v := range r {
		g.LoadRule(v)
	}
}

func (g *Group) LoadRulesWithQuery(query string) error {
	if g.loader == nil {
		return utils.Error("no SuricataRuleLoader set yet")
	}

	res, err := g.loader(query)
	if err != nil {
		return err
	}
	var count int
	for r := range res {
		count++
		log.Infof("load rule: %v", r.Message)
		g.LoadRule(r)
	}
	if count > 0 {
		log.Infof("load %d rules", count)
	}
	return nil
}

func (g *Group) unSerializingFrameWithoutCache(loopbackFirst bool, raw []byte) (gopacket.Packet, error) {
	order := make([]gopacket.Decoder, 2)
	if loopbackFirst {
		order[0] = layers.LayerTypeLoopback
		order[1] = layers.LayerTypeEthernet
	} else {
		order[0] = layers.LayerTypeEthernet
		order[1] = layers.LayerTypeLoopback
	}
	err := make([]error, 0, 2)
	for _, decoder := range order {
		pk := gopacket.NewPacket(raw, decoder, gopacket.NoCopy)
		if pk.LinkLayer() != nil {
			if pk.LinkLayer().LayerType() == decoder {
				// fetch ethernet
				return pk, nil
			}
		}
		if pk.ErrorLayer() != nil {
			err = append(err, pk.ErrorLayer().Error())
		}
	}
	if len(err) > 0 {
		return nil, utils.Errorf("decode packet failed: %v", err)
	}
	return nil, utils.Errorf("unknown error for parsing: %v", strconv.Quote(string(raw)))
}

func (g *Group) unSerializingFrame(loopback bool, raw []byte) (gopacket.Packet, error) {
	sha256 := codec.Sha256(raw)
	if pk, ok := g.pkCache.Get(sha256); ok {
		return pk, nil
	}
	packet, err := g.unSerializingFrameWithoutCache(loopback, raw)
	if err != nil {
		return nil, err
	}
	g.pkCache.Set(sha256, packet)
	return packet, nil
}

func (g *Group) feedPacket(pk gopacket.Packet) {
	g.wg.Add(1)
	select {
	case g.frameChan <- pk:
	case <-g.ctx.Done():
		g.wg.Done()
	default:
		go func() {
			select {
			case g.frameChan <- pk:
			case <-g.ctx.Done():
				g.wg.Done()
			}
		}()
	}
}

func (g *Group) feedHTTPFlow(flow *HttpFlow) {
	if flow == nil {
		return
	}
	g.wg.Add(1)
	select {
	case g.httpRequest <- flow:
	case <-g.ctx.Done():
		g.wg.Done()
	default:
		go func() {
			select {
			case g.httpRequest <- flow:
			case <-g.ctx.Done():
				g.wg.Done()
			}
		}()
	}
}

func (g *Group) Wait() {
	g.wg.Wait()
}

func (g *Group) FeedFrame(raw []byte) {
	pk, err := g.unSerializingFrame(false, raw)
	if err != nil {
		log.Errorf("unserializing frame failed: %v", err)
		return
	}
	g.feedPacket(pk)
}

func (g *Group) FeedHTTPRequestBytes(reqBytes []byte) {
	g.feedHTTPFlow(&HttpFlow{
		Req: reqBytes,
	})
}

func (g *Group) FeedHTTPResponseBytes(rsp []byte) {
	g.feedHTTPFlow(&HttpFlow{
		Rsp: rsp,
	})
}

func (g *Group) FeedHTTPFlowBytesWithTrafficFlow(flow *pcaputil.TrafficFlow, req, rsp []byte) {
	g.feedHTTPFlow(&HttpFlow{
		TrafficFlow: flow,
		Rsp:         rsp,
		Req:         req,
	})
}
func (g *Group) FeedHTTPFlowBytes(req, rsp []byte) {
	g.feedHTTPFlow(&HttpFlow{
		Rsp: rsp,
		Req: req,
	})
}

func (g *Group) FeedHTTPFlow(src, dst string, srcPort, dstPort int, req *http.Request, rsp *http.Response) {
	flow := &HttpFlow{
		ReqInstance: req,
		Src:         src,
		SrcPort:     srcPort,
		Dst:         dst,
		DstPort:     dstPort,
	}
	if req == nil && rsp != nil {
		flow.Rsp, _ = utils.DumpHTTPResponse(rsp, true)
		if rsp.Request != nil {
			flow.Req, _ = utils.DumpHTTPRequest(rsp.Request, true)
		}
	} else {
		if ret := httpctx.GetBareRequestBytes(req); len(ret) > 0 {
			flow.Req = ret
		}
		if ret := httpctx.GetBareResponseBytes(req); len(ret) > 0 {
			flow.Rsp = ret
		}
	}

	// TODO: 这里需要考虑一下，如果httpRequest chan满了，那么就会阻塞，这样会导致整个程序阻塞
	g.feedHTTPFlow(flow)
}

func (g *Group) consumeMain() {
	g.consumeOnce.Do(func() {
		go func() {
			defer func() {
				utils.TryCloseChannel(g.frameChan)
				utils.TryCloseChannel(g.httpRequest)
			}()
			for {
				select {
				case packetFrame := <-g.frameChan:
					for _, matcherpool := range g.OrdinaryMatcher {
						matcher := matcherpool.Get().(*Matcher)
						if matcher.MatchPackage(packetFrame) {
							g.onMatchedCallback(packetFrame, matcher.matcher.Rule)
						}
						matcherpool.Put(matcher)
					}
					g.wg.Done()
				case httpFlowInstance := <-g.httpRequest:
					_ = httpFlowInstance
					pkgs := httpFlowInstance.ToRequestPacket()
					if len(pkgs) <= 0 {
						g.wg.Done()
						continue
					}
					for _, matcherpool := range g.HTTPMatcher {
						for _, pkg := range pkgs {
							matcher := matcherpool.Get().(*Matcher)
							if matcher.MatchPackage(pkg) {
								g.onMatchedCallback(pkg, matcher.matcher.Rule)
							}
							matcherpool.Put(matcher)
						}
					}
					g.wg.Done()
				case <-g.ctx.Done():
					return
				}
			}
		}()
	})
}
