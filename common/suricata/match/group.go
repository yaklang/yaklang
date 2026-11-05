package match

import (
	"context"
	"errors"
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

var (
	ErrGroupClosed    = errors.New("suricata match group is closed")
	ErrGroupQueueFull = errors.New("suricata match group queue is full")
)

// RuleLoadResult describes the compilation result for one rule.
type RuleLoadResult struct {
	Rule *rule.Rule
	Err  error
}

// Group is a group of rules.
type callbackTask struct {
	packet gopacket.Packet
	rule   *rule.Rule
}

type Group struct {
	pkCache *utils.Cache[gopacket.Packet]

	loaderMu sync.RWMutex
	loader   SuricataRuleLoaderType

	HTTPMatcher     []*sync.Pool
	OrdinaryMatcher []*sync.Pool
	matcherMu       sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc

	frameChan   chan gopacket.Packet
	httpRequest chan *HttpFlow

	consumeOnce   sync.Once
	closeOnce     sync.Once
	consumerDone  chan struct{}
	callbackQueue chan callbackTask
	callbackWG    sync.WaitGroup

	stateMu   sync.Mutex
	closed    bool
	enqueueWG sync.WaitGroup

	onMatchedCallback func(packet gopacket.Packet, match *rule.Rule)

	pendingMu   sync.Mutex
	pendingCond *sync.Cond
	pending     int
}

type SuricataRuleLoaderType func(query string) (chan *rule.Rule, error)

var defaultSuricataRuleLoader SuricataRuleLoaderType = nil

func RegisterSuricataRuleLoader(h SuricataRuleLoaderType) {
	defaultSuricataRuleLoader = h
}

func NewGroup(opt ...GroupOption) *Group {
	return newGroup(50000, opt...)
}

// newGroup allows package tests to exercise backpressure without allocating the
// production-sized queues.
func newGroup(queueSize int, opt ...GroupOption) *Group {
	if queueSize < 0 {
		queueSize = 0
	}
	ctx, cancel := context.WithCancel(context.Background())
	group := &Group{
		pkCache:       utils.NewTTLCache[gopacket.Packet](30 * time.Second),
		loader:        defaultSuricataRuleLoader,
		frameChan:     make(chan gopacket.Packet, queueSize),
		httpRequest:   make(chan *HttpFlow, queueSize),
		ctx:           ctx,
		cancel:        cancel,
		consumerDone:  make(chan struct{}),
		callbackQueue: make(chan callbackTask, max(1, queueSize)),
		onMatchedCallback: func(packet gopacket.Packet, match *rule.Rule) {
			log.Infof("matched: %v", match.Raw)
		},
	}
	group.pendingCond = sync.NewCond(&group.pendingMu)
	for _, i := range opt {
		i(group)
	}
	group.startCallbackWorkers(4)
	group.consumeMain()
	return group
}

func (g *Group) SetLoader(loader SuricataRuleLoaderType) {
	g.loaderMu.Lock()
	g.loader = loader
	g.loaderMu.Unlock()
}

// LoadRule preserves the original logging-only API.
func (g *Group) LoadRule(r *rule.Rule) {
	if err := g.LoadRuleWithError(r); err != nil {
		log.Errorf("compile rule failed: %v", err)
	}
}

// LoadRuleWithError compiles and installs one rule, returning any compilation error.
func (g *Group) LoadRuleWithError(r *rule.Rule) error {
	if r == nil {
		return utils.Error("cannot load nil rule")
	}
	matcher, err := CompileRule(r)
	if err != nil {
		return err
	}

	ordinaryPool := newMatcherPool(matcher)
	var httpPool *sync.Pool
	if r.Protocol == "http" {
		httpPool = newMatcherPool(matcher)
	}

	g.matcherMu.Lock()
	if httpPool != nil {
		g.HTTPMatcher = append(g.HTTPMatcher, httpPool)
	}
	g.OrdinaryMatcher = append(g.OrdinaryMatcher, ordinaryPool)
	g.matcherMu.Unlock()
	return nil
}

func newMatcherPool(matcher *Matcher) *sync.Pool {
	return &sync.Pool{New: func() any {
		return &Matcher{matcher: matcher.matcher.Clone()}
	}}
}

// LoadRules preserves the original logging-only API.
func (g *Group) LoadRules(r ...*rule.Rule) {
	for _, result := range g.LoadRulesWithResults(r...) {
		if result.Err != nil {
			log.Errorf("compile rule failed: %v", result.Err)
		}
	}
}

// LoadRulesWithResults compiles every rule and reports each result. A bad rule
// does not prevent later rules from being installed.
func (g *Group) LoadRulesWithResults(r ...*rule.Rule) []RuleLoadResult {
	results := make([]RuleLoadResult, 0, len(r))
	for _, currentRule := range r {
		results = append(results, RuleLoadResult{
			Rule: currentRule,
			Err:  g.LoadRuleWithError(currentRule),
		})
	}
	return results
}

func (g *Group) LoadRulesWithQuery(query string) error {
	g.loaderMu.RLock()
	loader := g.loader
	g.loaderMu.RUnlock()
	if loader == nil {
		return utils.Error("no SuricataRuleLoader set yet")
	}

	res, err := loader(query)
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
	// gopacket.NoCopy retains its input. Own the bytes before the packet can be
	// queued or cached asynchronously.
	ownedRaw := append([]byte(nil), raw...)
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
		pk := gopacket.NewPacket(ownedRaw, decoder, gopacket.NoCopy)
		if pk.LinkLayer() != nil && pk.LinkLayer().LayerType() == decoder {
			return pk, nil
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

func (g *Group) beginEnqueue() error {
	g.stateMu.Lock()
	defer g.stateMu.Unlock()
	if g.closed {
		return ErrGroupClosed
	}
	g.enqueueWG.Add(1)
	g.addPending()
	return nil
}

func (g *Group) addPending() {
	g.pendingMu.Lock()
	g.pending++
	g.pendingMu.Unlock()
}

func (g *Group) donePending() {
	g.pendingMu.Lock()
	g.pending--
	if g.pending == 0 {
		g.pendingCond.Broadcast()
	}
	g.pendingMu.Unlock()
}

func (g *Group) enqueuePacket(ctx context.Context, pk gopacket.Packet, try bool) error {
	accepted := false
	defer func() {
		if !accepted {
			g.donePending()
		}
	}()

	if try {
		select {
		case <-g.ctx.Done():
			return ErrGroupClosed
		default:
		}
		select {
		case g.frameChan <- pk:
			accepted = true
			return nil
		case <-g.ctx.Done():
			return ErrGroupClosed
		default:
			return ErrGroupQueueFull
		}
	}

	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case g.frameChan <- pk:
		accepted = true
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-g.ctx.Done():
		return ErrGroupClosed
	}
}

func (g *Group) enqueueHTTPFlow(ctx context.Context, flow *HttpFlow, try bool) error {
	if flow == nil {
		g.donePending()
		return utils.Error("cannot feed nil HTTP flow")
	}
	accepted := false
	defer func() {
		if !accepted {
			g.donePending()
		}
	}()

	if try {
		select {
		case <-g.ctx.Done():
			return ErrGroupClosed
		default:
		}
		select {
		case g.httpRequest <- flow:
			accepted = true
			return nil
		case <-g.ctx.Done():
			return ErrGroupClosed
		default:
			return ErrGroupQueueFull
		}
	}

	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case g.httpRequest <- flow:
		accepted = true
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-g.ctx.Done():
		return ErrGroupClosed
	}
}

// Wait blocks until all accepted inputs and their match callbacks complete.
// It must not be called from an OnMatched callback because that callback is
// itself part of the pending work Wait observes.
func (g *Group) Wait() {
	g.pendingMu.Lock()
	for g.pending > 0 {
		g.pendingCond.Wait()
	}
	g.pendingMu.Unlock()
}

func (g *Group) FeedFrame(raw []byte) {
	if err := g.FeedFrameContext(context.Background(), raw); err != nil {
		log.Errorf("feed frame failed: %v", err)
	}
}

func (g *Group) TryFeedFrame(raw []byte) error {
	return g.feedFrameContext(context.Background(), raw, true)
}

func (g *Group) FeedFrameContext(ctx context.Context, raw []byte) error {
	return g.feedFrameContext(ctx, raw, false)
}

func (g *Group) feedFrameContext(ctx context.Context, raw []byte, try bool) error {
	if err := g.beginEnqueue(); err != nil {
		return err
	}
	defer g.enqueueWG.Done()

	pk, err := g.unSerializingFrame(false, raw)
	if err != nil {
		g.donePending()
		return utils.Errorf("unserializing frame failed: %v", err)
	}
	return g.enqueuePacket(ctx, pk, try)
}

func (g *Group) FeedHTTPRequestBytes(reqBytes []byte) {
	if err := g.FeedHTTPRequestBytesContext(context.Background(), reqBytes); err != nil {
		log.Errorf("feed HTTP request failed: %v", err)
	}
}

func (g *Group) TryFeedHTTPRequestBytes(reqBytes []byte) error {
	return g.feedHTTPFlowContext(context.Background(), &HttpFlow{Req: append([]byte(nil), reqBytes...)}, true)
}

func (g *Group) FeedHTTPRequestBytesContext(ctx context.Context, reqBytes []byte) error {
	return g.feedHTTPFlowContext(ctx, &HttpFlow{Req: append([]byte(nil), reqBytes...)}, false)
}

func (g *Group) FeedHTTPResponseBytes(rsp []byte) {
	if err := g.FeedHTTPResponseBytesContext(context.Background(), rsp); err != nil {
		log.Errorf("feed HTTP response failed: %v", err)
	}
}

func (g *Group) TryFeedHTTPResponseBytes(rsp []byte) error {
	return g.feedHTTPFlowContext(context.Background(), &HttpFlow{Rsp: append([]byte(nil), rsp...)}, true)
}

func (g *Group) FeedHTTPResponseBytesContext(ctx context.Context, rsp []byte) error {
	return g.feedHTTPFlowContext(ctx, &HttpFlow{Rsp: append([]byte(nil), rsp...)}, false)
}

func (g *Group) FeedHTTPFlowBytesWithTrafficFlow(flow *pcaputil.TrafficFlow, req, rsp []byte) {
	if err := g.FeedHTTPFlowBytesWithTrafficFlowContext(context.Background(), flow, req, rsp); err != nil {
		log.Errorf("feed HTTP flow failed: %v", err)
	}
}

func (g *Group) TryFeedHTTPFlowBytesWithTrafficFlow(flow *pcaputil.TrafficFlow, req, rsp []byte) error {
	return g.feedHTTPFlowContext(context.Background(), &HttpFlow{
		TrafficFlow: flow,
		Req:         append([]byte(nil), req...),
		Rsp:         append([]byte(nil), rsp...),
	}, true)
}

func (g *Group) FeedHTTPFlowBytesWithTrafficFlowContext(ctx context.Context, flow *pcaputil.TrafficFlow, req, rsp []byte) error {
	return g.feedHTTPFlowContext(ctx, &HttpFlow{
		TrafficFlow: flow,
		Req:         append([]byte(nil), req...),
		Rsp:         append([]byte(nil), rsp...),
	}, false)
}

func (g *Group) FeedHTTPFlowBytes(req, rsp []byte) {
	if err := g.FeedHTTPFlowBytesContext(context.Background(), req, rsp); err != nil {
		log.Errorf("feed HTTP flow failed: %v", err)
	}
}

func (g *Group) TryFeedHTTPFlowBytes(req, rsp []byte) error {
	return g.feedHTTPFlowContext(context.Background(), &HttpFlow{
		Req: append([]byte(nil), req...),
		Rsp: append([]byte(nil), rsp...),
	}, true)
}

func (g *Group) FeedHTTPFlowBytesContext(ctx context.Context, req, rsp []byte) error {
	return g.feedHTTPFlowContext(ctx, &HttpFlow{
		Req: append([]byte(nil), req...),
		Rsp: append([]byte(nil), rsp...),
	}, false)
}

func (g *Group) FeedHTTPFlow(src, dst string, srcPort, dstPort int, req *http.Request, rsp *http.Response) {
	if err := g.FeedHTTPFlowContext(context.Background(), src, dst, srcPort, dstPort, req, rsp); err != nil {
		log.Errorf("feed HTTP flow failed: %v", err)
	}
}

func (g *Group) TryFeedHTTPFlow(src, dst string, srcPort, dstPort int, req *http.Request, rsp *http.Response) error {
	return g.feedHTTPFlowInstance(context.Background(), true, src, dst, srcPort, dstPort, req, rsp)
}

func (g *Group) FeedHTTPFlowContext(ctx context.Context, src, dst string, srcPort, dstPort int, req *http.Request, rsp *http.Response) error {
	return g.feedHTTPFlowInstance(ctx, false, src, dst, srcPort, dstPort, req, rsp)
}

func (g *Group) feedHTTPFlowInstance(ctx context.Context, try bool, src, dst string, srcPort, dstPort int, req *http.Request, rsp *http.Response) error {
	flow := &HttpFlow{
		Src:     src,
		SrcPort: srcPort,
		Dst:     dst,
		DstPort: dstPort,
	}
	if req == nil && rsp != nil {
		flow.Rsp, _ = utils.DumpHTTPResponse(rsp, true)
		if rsp.Request != nil {
			flow.Req, _ = utils.DumpHTTPRequest(rsp.Request, true)
		}
	} else if req != nil {
		if ret := httpctx.GetBareRequestBytes(req); len(ret) > 0 {
			flow.Req = append([]byte(nil), ret...)
		} else {
			flow.Req, _ = utils.DumpHTTPRequest(req, true)
		}
		if ret := httpctx.GetBareResponseBytes(req); len(ret) > 0 {
			flow.Rsp = append([]byte(nil), ret...)
		} else if rsp != nil {
			flow.Rsp, _ = utils.DumpHTTPResponse(rsp, true)
		}
	}
	return g.feedHTTPFlowContext(ctx, flow, try)
}

func (g *Group) feedHTTPFlowContext(ctx context.Context, flow *HttpFlow, try bool) error {
	if err := g.beginEnqueue(); err != nil {
		return err
	}
	defer g.enqueueWG.Done()
	return g.enqueueHTTPFlow(ctx, flow, try)
}

// Close rejects new work, releases blocked feeders, drains accepted work, and
// stops the packet cache. It is safe to call multiple times and concurrently.
func (g *Group) Close() {
	g.closeOnce.Do(func() {
		g.stateMu.Lock()
		g.closed = true
		g.cancel()
		g.stateMu.Unlock()

		g.enqueueWG.Wait()
		close(g.frameChan)
		close(g.httpRequest)
		<-g.consumerDone
		close(g.callbackQueue)
		g.pkCache.Close()
	})
}

func (g *Group) snapshotOrdinaryMatchers() []*sync.Pool {
	g.matcherMu.RLock()
	defer g.matcherMu.RUnlock()
	return append([]*sync.Pool(nil), g.OrdinaryMatcher...)
}

func (g *Group) snapshotHTTPMatchers() []*sync.Pool {
	g.matcherMu.RLock()
	defer g.matcherMu.RUnlock()
	return append([]*sync.Pool(nil), g.HTTPMatcher...)
}

func (g *Group) startCallbackWorkers(workers int) {
	for i := 0; i < workers; i++ {
		g.callbackWG.Add(1)
		go func() {
			defer g.callbackWG.Done()
			for task := range g.callbackQueue {
				g.onMatchedCallback(task.packet, task.rule)
				g.donePending()
			}
		}()
	}
}

func (g *Group) dispatchCallback(packet gopacket.Packet, matched *rule.Rule) {
	g.addPending()
	g.callbackQueue <- callbackTask{packet: packet, rule: matched}
}

func (g *Group) consumeMain() {
	g.consumeOnce.Do(func() {
		go func() {
			defer close(g.consumerDone)
			frameChan := g.frameChan
			httpRequest := g.httpRequest
			for frameChan != nil || httpRequest != nil {
				select {
				case packetFrame, ok := <-frameChan:
					if !ok {
						frameChan = nil
						continue
					}
					for _, matcherpool := range g.snapshotOrdinaryMatchers() {
						matcher := matcherpool.Get().(*Matcher)
						if matcher.MatchPackage(packetFrame) {
							g.dispatchCallback(packetFrame, matcher.matcher.Rule)
						}
						matcherpool.Put(matcher)
					}
					g.donePending()
				case httpFlowInstance, ok := <-httpRequest:
					if !ok {
						httpRequest = nil
						continue
					}
					pkgs := httpFlowInstance.ToRequestPacket()
					if len(pkgs) > 0 {
						for _, matcherpool := range g.snapshotHTTPMatchers() {
							for _, pkg := range pkgs {
								matcher := matcherpool.Get().(*Matcher)
								if matcher.MatchPackage(pkg) {
									g.dispatchCallback(pkg, matcher.matcher.Rule)
								}
								matcherpool.Put(matcher)
							}
						}
					}
					g.donePending()
				}
			}
		}()
	})
}
