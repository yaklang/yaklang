package engine

import (
	"fmt"
	"net/netip"
	"regexp/syntax"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/minirehs"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/match"
	"github.com/yaklang/yaklang/common/suricata/pcre"
	"github.com/yaklang/yaklang/common/suricata/rule"
)

type silentMinirehsLogger struct{}

func (silentMinirehsLogger) Infof(string, ...interface{})  {}
func (silentMinirehsLogger) Warnf(string, ...interface{})  {}
func (silentMinirehsLogger) Errorf(string, ...interface{}) {}
func (silentMinirehsLogger) Debugf(string, ...interface{}) {}

// minirehsGroup wraps compiled minirehs Database(s) with deduplicated
// pattern-to-plan mappings and bounded, drainable Scratch pools so worker
// goroutines owned by Scratch are cleanly closed on engine shutdown.
//
// Plain literal patterns compile into litDB (BackendEngine: C99 Teddy SIMD
// prefilter with minimal heap footprint), while PCRE patterns compile into
// reDB (BackendMVS: bit-parallel Glushkov NFA existence scanning). Keeping
// them separate prevents a handful of PCRE-only rules from forcing tens of
// thousands of plain literals to build Glushkov NFAs.
type minirehsGroup struct {
	litDB    minirehs.Database
	litPlans [][]int
	litPool  chan minirehs.Scratch

	reDB    minirehs.Database
	rePlans [][]int
	rePool  chan minirehs.Scratch

	closed atomic.Bool
}

type patternKey struct {
	expr  string
	flags minirehs.Flag
}

type minirehsGroupBuilder struct {
	litPatterns []minirehs.Pattern
	litPlans    [][]int
	litDedup    map[patternKey]minirehs.PatternID

	rePatterns []minirehs.Pattern
	rePlans    [][]int
	reDedup    map[patternKey]minirehs.PatternID
}

func newMinirehsGroupBuilder() *minirehsGroupBuilder {
	return &minirehsGroupBuilder{
		litDedup: make(map[patternKey]minirehs.PatternID),
		reDedup:  make(map[patternKey]minirehs.PatternID),
	}
}

func (b *minirehsGroupBuilder) add(expr string, flags minirehs.Flag, isRegex bool, planIdx int) {
	key := patternKey{expr: expr, flags: flags}
	if isRegex {
		if id, exists := b.reDedup[key]; exists {
			plans := b.rePlans[id]
			if len(plans) == 0 || plans[len(plans)-1] != planIdx {
				b.rePlans[id] = append(plans, planIdx)
			}
			return
		}
		id := minirehs.PatternID(len(b.rePatterns))
		b.reDedup[key] = id
		b.rePatterns = append(b.rePatterns, minirehs.Pattern{ID: id, Expr: expr, Flags: flags})
		b.rePlans = append(b.rePlans, []int{planIdx})
		return
	}
	if id, exists := b.litDedup[key]; exists {
		plans := b.litPlans[id]
		if len(plans) == 0 || plans[len(plans)-1] != planIdx {
			b.litPlans[id] = append(plans, planIdx)
		}
		return
	}
	id := minirehs.PatternID(len(b.litPatterns))
	b.litDedup[key] = id
	b.litPatterns = append(b.litPatterns, minirehs.Pattern{ID: id, Expr: expr, Flags: flags})
	b.litPlans = append(b.litPlans, []int{planIdx})
}

func (b *minirehsGroupBuilder) build() (*minirehsGroup, []int) {
	if b == nil || (len(b.litPatterns) == 0 && len(b.rePatterns) == 0) {
		return nil, nil
	}
	poolSize := runtime.NumCPU()
	if poolSize < 4 {
		poolSize = 4
	}
	if poolSize > 16 {
		poolSize = 16
	}

	grp := &minirehsGroup{}
	var fallback []int

	if len(b.litPatterns) > 0 {
		db, err := minirehs.Compile(
			b.litPatterns,
			minirehs.WithBackend(minirehs.BackendEngine),
			minirehs.WithReportLocation(false),
			minirehs.WithMinLiteralLen(2),
			minirehs.WithLogger(silentMinirehsLogger{}),
		)
		if err != nil {
			for _, plans := range b.litPlans {
				fallback = append(fallback, plans...)
			}
		} else {
			grp.litDB = db
			grp.litPlans = b.litPlans
			grp.litPool = make(chan minirehs.Scratch, poolSize)
		}
	}

	if len(b.rePatterns) > 0 {
		db, err := minirehs.Compile(
			b.rePatterns,
			minirehs.WithBackend(minirehs.BackendMVS),
			minirehs.WithReportLocation(false),
			minirehs.WithMinLiteralLen(2),
			minirehs.WithLogger(silentMinirehsLogger{}),
		)
		if err != nil {
			for _, plans := range b.rePlans {
				fallback = append(fallback, plans...)
			}
		} else {
			grp.reDB = db
			grp.rePlans = b.rePlans
			grp.rePool = make(chan minirehs.Scratch, poolSize)
		}
	}

	if grp.litDB == nil && grp.reDB == nil {
		return nil, fallback
	}
	return grp, fallback
}

func scanMinirehsSubDB(db minirehs.Database, pool chan minirehs.Scratch, planMap [][]int, closed *atomic.Bool, data []byte, collector *candidateCollector) {
	if db == nil {
		return
	}
	var sc minirehs.Scratch
	select {
	case sc = <-pool:
	default:
	}
	if sc == nil {
		var err error
		sc, err = db.NewScratch()
		if err != nil {
			for _, plans := range planMap {
				for _, idx := range plans {
					collector.add(idx)
				}
			}
			return
		}
	}
	defer func() {
		if closed.Load() {
			_ = sc.Close()
			return
		}
		select {
		case pool <- sc:
		default:
			_ = sc.Close()
		}
	}()

	if err := db.Scan(data, sc, func(m minirehs.Match) bool {
		if int(m.ID) < len(planMap) {
			for _, idx := range planMap[m.ID] {
				collector.add(idx)
			}
		}
		return true
	}); err != nil {
		for _, plans := range planMap {
			for _, idx := range plans {
				collector.add(idx)
			}
		}
	}
}

func (g *minirehsGroup) scan(data []byte, collector *candidateCollector) {
	if g == nil || len(data) == 0 || g.closed.Load() {
		return
	}
	if g.litDB != nil {
		scanMinirehsSubDB(g.litDB, g.litPool, g.litPlans, &g.closed, data, collector)
	}
	if g.reDB != nil {
		scanMinirehsSubDB(g.reDB, g.rePool, g.rePlans, &g.closed, data, collector)
	}
}

func closeMinirehsSubDB(db minirehs.Database, pool chan minirehs.Scratch) {
	if pool != nil {
		for {
			select {
			case sc := <-pool:
				if sc != nil {
					_ = sc.Close()
				}
			default:
				goto closeDB
			}
		}
	}
closeDB:
	if db != nil {
		_ = db.Close()
	}
}

func (g *minirehsGroup) close() {
	if g == nil || !g.closed.CompareAndSwap(false, true) {
		return
	}
	closeMinirehsSubDB(g.litDB, g.litPool)
	closeMinirehsSubDB(g.reDB, g.rePool)
}

type bufferGroup struct {
	mod   modifier.Modifier
	group *minirehsGroup
}

type httpStreamLane struct {
	hasPlans       bool
	reqBuffers     []bufferGroup
	respBuffers    []bufferGroup
	sharedBuffers  []bufferGroup
	reqHeaderLines *minirehsGroup
	resHeaderLines *minirehsGroup
	alwaysReq      []int
	alwaysResp     []int
	alwaysBoth     []int
}

func (l *httpStreamLane) close() {
	for _, bg := range l.reqBuffers {
		bg.group.close()
	}
	for _, bg := range l.respBuffers {
		bg.group.close()
	}
	for _, bg := range l.sharedBuffers {
		bg.group.close()
	}
	l.reqHeaderLines.close()
	l.resHeaderLines.close()
}

type tlsStreamLane struct {
	hasPlans   bool
	buffers    []bufferGroup
	certsGroup *minirehsGroup
	alwaysEval []int
}

func (l *tlsStreamLane) close() {
	for _, bg := range l.buffers {
		bg.group.close()
	}
	l.certsGroup.close()
}

type singleBufferLane struct {
	hasPlans   bool
	group      *minirehsGroup
	binaryAC   *rawByteAC
	alwaysEval []int
	alwaysIdx  ipFilteredSet
	// Optional direction split for streamTCP fallback rules.
	alwaysToServer ipFilteredSet
	alwaysToClient ipFilteredSet
	alwaysAnyDir   ipFilteredSet
	hasDirSplit    bool
}

func (l *singleBufferLane) close() {
	l.group.close()
}

type dnsPacketLane struct {
	hasPlans   bool
	queryGroup *minirehsGroup
	rawGroup   *minirehsGroup
	alwaysEval []int
}

func (l *dnsPacketLane) close() {
	l.queryGroup.close()
	l.rawGroup.close()
}

type candidateCollector struct {
	seen    []uint32
	gen     uint32
	indices []int
}

func (c *candidateCollector) reset(planCount int) {
	if len(c.seen) < planCount {
		c.seen = make([]uint32, planCount)
		c.gen = 1
	} else {
		c.gen++
		if c.gen == 0 {
			clear(c.seen)
			c.gen = 1
		}
	}
	c.indices = c.indices[:0]
}

func (c *candidateCollector) add(idx int) {
	if idx < 0 || idx >= len(c.seen) {
		return
	}
	if c.seen[idx] != c.gen {
		c.seen[idx] = c.gen
		c.indices = append(c.indices, idx)
	}
}

func (c *candidateCollector) addSlice(indices []int) {
	for _, idx := range indices {
		c.add(idx)
	}
}

func (c *candidateCollector) sorted() []int {
	if len(c.indices) > 1 {
		slices.Sort(c.indices)
	}
	return c.indices
}

// enginePrefilter partitions compiled rules by view, protocol, and sticky
// buffer and compiles unified minirehs MVS prefilter databases for high-speed
// candidate selection with zero false negatives.
type enginePrefilter struct {
	plans     []*CompiledRulePlan
	planCount int

	streamHTTP  httpStreamLane
	streamTLS   tlsStreamLane
	streamTCP   singleBufferLane
	streamHTTP2 []int
	// tcpFileDataPlans tracks alert tcp rules using file.data/file.magic whose
	// indexed prefilter literal lives in file.data, so compressed HTTP-over-TCP
	// streams can still trigger them without false negatives.
	tcpFileDataPlans []int

	pktDNS    dnsPacketLane
	pktTCP    singleBufferLane
	pktUDP    singleBufferLane
	pktICMP   singleBufferLane
	pktICMPv6 singleBufferLane
	pktIP     singleBufferLane
	pktOther  []int

	allPacketPlans []int
	allStreamPlans []int

	collectorPool sync.Pool
}

var httpRequestOnlyModifiers = map[modifier.Modifier]bool{
	modifier.HTTPUri:           true,
	modifier.HTTPUriRaw:        true,
	modifier.HTTPMethod:        true,
	modifier.HTTPHost:          true,
	modifier.HTTPHostRaw:       true,
	modifier.HTTPUserAgent:     true,
	modifier.HTTPRequestBody:   true,
	modifier.HTTPRequestLine:   true,
	modifier.HTTPReferer:       true,
	modifier.HTTPAccept:        true,
	modifier.HTTPAcceptEnc:     true,
	modifier.HTTPAcceptLang:    true,
	modifier.HTTPRequestHeader: true,
}

var httpResponseOnlyModifiers = map[modifier.Modifier]bool{
	modifier.HTTPStatCode:       true,
	modifier.HTTPStatMsg:        true,
	modifier.HTTPResponseBody:   true,
	modifier.HTTPResponseLine:   true,
	modifier.HTTPServer:         true,
	modifier.HTTPLocation:       true,
	modifier.HTTPResponseHeader: true,
}

var httpAllowedBuffers = map[modifier.Modifier]bool{
	modifier.Default:            true,
	modifier.HTTPUri:            true,
	modifier.HTTPUriRaw:         true,
	modifier.HTTPMethod:         true,
	modifier.HTTPHost:           true,
	modifier.HTTPHostRaw:        true,
	modifier.HTTPUserAgent:      true,
	modifier.HTTPHeader:         true,
	modifier.HTTPHeaderRaw:      true,
	modifier.HTTPHeaderNames:    true,
	modifier.HTTPRequestHeader:  true,
	modifier.HTTPResponseHeader: true,
	modifier.HTTPCookie:         true,
	modifier.HTTPRequestBody:    true,
	modifier.HTTPResponseBody:   true,
	modifier.FileData:           true,
	modifier.HTTPStatCode:       true,
	modifier.HTTPStatMsg:        true,
	modifier.HTTPRequestLine:    true,
	modifier.HTTPResponseLine:   true,
	modifier.HTTPReferer:        true,
	modifier.HTTPServer:         true,
	modifier.HTTPLocation:       true,
	modifier.HTTPContentType:    true,
	modifier.HTTPContentLen:     true,
	modifier.HTTPStart:          true,
	modifier.HTTPProtocol:       true,
	modifier.HTTPAccept:         true,
	modifier.HTTPAcceptEnc:      true,
	modifier.HTTPAcceptLang:     true,
	modifier.HTTPConnection:     true,
}

var tlsAllowedBuffers = map[modifier.Modifier]bool{
	modifier.Default:            true,
	modifier.TLSSNI:             true,
	modifier.TLSCertSubject:     true,
	modifier.TLSCertIssuer:      true,
	modifier.TLSCertSerial:      true,
	modifier.TLSCertFingerprint: true,
	modifier.TLSCerts:           true,
	modifier.TLSRandom:          true,
	modifier.TLSRandomTime:      true,
	modifier.TLSRandomBytes:     true,
	modifier.JA3Hash:            true,
	modifier.JA3String:          true,
	modifier.JA3SHash:           true,
	modifier.JA3SString:         true,
}

var tcpStreamAllowedBuffers = map[modifier.Modifier]bool{
	modifier.Default:  true,
	modifier.FileData: true,
}

var defaultOnlyAllowedBuffers = map[modifier.Modifier]bool{
	modifier.Default: true,
}

var dnsAllowedBuffers = map[modifier.Modifier]bool{
	modifier.DNSQuery: true,
	modifier.Default:  true,
}

func buildEnginePrefilter(plans []*CompiledRulePlan) *enginePrefilter {
	pf := &enginePrefilter{
		plans:     plans,
		planCount: len(plans),
	}
	pf.collectorPool.New = func() any {
		return &candidateCollector{
			seen:    make([]uint32, len(plans)),
			indices: make([]int, 0, 32),
		}
	}

	httpBuilders := make(map[modifier.Modifier]*minirehsGroupBuilder)
	tlsBuilders := make(map[modifier.Modifier]*minirehsGroupBuilder)
	tcpStreamBuilder := newMinirehsGroupBuilder()
	tcpStreamBinBuilder := newRawByteACBuilder()
	dnsQueryBuilder := newMinirehsGroupBuilder()
	dnsRawBuilder := newMinirehsGroupBuilder()
	pktTCPBuilder := newMinirehsGroupBuilder()
	pktTCPBinBuilder := newRawByteACBuilder()
	pktUDPBuilder := newMinirehsGroupBuilder()
	pktUDPBinBuilder := newRawByteACBuilder()
	pktICMPBuilder := newMinirehsGroupBuilder()
	pktICMPBinBuilder := newRawByteACBuilder()
	pktICMPv6Builder := newMinirehsGroupBuilder()
	pktICMPv6BinBuilder := newRawByteACBuilder()
	pktIPBuilder := newMinirehsGroupBuilder()
	pktIPBinBuilder := newRawByteACBuilder()

	getBuilder := func(m map[modifier.Modifier]*minirehsGroupBuilder, mod modifier.Modifier) *minirehsGroupBuilder {
		b := m[mod]
		if b == nil {
			b = newMinirehsGroupBuilder()
			m[mod] = b
		}
		return b
	}

	for idx, plan := range plans {
		if plan == nil || plan.Rule == nil {
			continue
		}
		if plan.acceptsStream {
			pf.allStreamPlans = append(pf.allStreamPlans, idx)
			switch {
			case plan.isHTTP:
				pf.streamHTTP.hasPlans = true
				if mod, expr, flags, isRegex, ok := extractPrefilterPattern(plan.Rule, httpAllowedBuffers, false); ok {
					getBuilder(httpBuilders, mod).add(expr, flags, isRegex, idx)
				} else {
					addHTTPAlwaysEval(&pf.streamHTTP, plan, idx)
				}
			case plan.isTLS:
				pf.streamTLS.hasPlans = true
				if mod, expr, flags, isRegex, ok := extractPrefilterPattern(plan.Rule, tlsAllowedBuffers, true); ok {
					getBuilder(tlsBuilders, mod).add(expr, flags, isRegex, idx)
				} else {
					pf.streamTLS.alwaysEval = append(pf.streamTLS.alwaysEval, idx)
				}
			case plan.isHTTP2:
				pf.streamHTTP2 = append(pf.streamHTTP2, idx)
			default:
				pf.streamTCP.hasPlans = true
				allowed := tcpStreamAllowedBuffers
				// SMTP file_data decodes MIME/base64 and SSH buffers strip banners,
				// so only index modifier.Default on raw stream bytes for those protocols.
				if plan.Rule.Protocol == "smtp" || rule.UsesSSHBuffers(plan.Rule) {
					allowed = defaultOnlyAllowedBuffers
				}
				if rule.UsesFileData(plan.Rule) {
					pf.tcpFileDataPlans = append(pf.tcpFileDataPlans, idx)
				}
				if _, expr, flags, isRegex, ok := extractPrefilterPattern(plan.Rule, allowed, false); ok {
					tcpStreamBuilder.add(expr, flags, isRegex, idx)
				} else if rawLit, caseless, ok := extractBinaryLiteral(plan.Rule, allowed); ok {
					tcpStreamBinBuilder.add(rawLit, caseless, idx)
				} else {
					pf.streamTCP.alwaysEval = append(pf.streamTCP.alwaysEval, idx)
				}
			}
		}

		if plan.acceptsPacket {
			pf.allPacketPlans = append(pf.allPacketPlans, idx)
			switch plan.Rule.Protocol {
			case "dns":
				pf.pktDNS.hasPlans = true
				if mod, expr, flags, isRegex, ok := extractPrefilterPattern(plan.Rule, dnsAllowedBuffers, false); ok {
					if mod == modifier.DNSQuery {
						dnsQueryBuilder.add(expr, flags, isRegex, idx)
					} else {
						dnsRawBuilder.add(expr, flags, isRegex, idx)
					}
				} else {
					pf.pktDNS.alwaysEval = append(pf.pktDNS.alwaysEval, idx)
				}
			case "tcp", "tcp-pkt", "tcp-stream", "http", "http1", "http2", "tls", "smtp", "ftp", "ftp-data", "smb", "ssh", "dcerpc", "rdp", "telnet":
				pf.pktTCP.hasPlans = true
				// Packet-view HTTP/TLS/SSH/SMTP-file_data rules parse structured
				// buffers rather than raw TCP payload; keep them in alwaysEval.
				if plan.isHTTP || plan.isHTTP2 || plan.isTLS || plan.Rule.Protocol == "smtp" || rule.UsesSSHBuffers(plan.Rule) {
					pf.pktTCP.alwaysEval = append(pf.pktTCP.alwaysEval, idx)
				} else if _, expr, flags, isRegex, ok := extractPrefilterPattern(plan.Rule, defaultOnlyAllowedBuffers, false); ok {
					pktTCPBuilder.add(expr, flags, isRegex, idx)
				} else if rawLit, caseless, ok := extractBinaryLiteral(plan.Rule, defaultOnlyAllowedBuffers); ok {
					pktTCPBinBuilder.add(rawLit, caseless, idx)
				} else {
					pf.pktTCP.alwaysEval = append(pf.pktTCP.alwaysEval, idx)
				}
			case "udp", "ike", "bittorrent-dht", "snmp":
				pf.pktUDP.hasPlans = true
				if _, expr, flags, isRegex, ok := extractPrefilterPattern(plan.Rule, defaultOnlyAllowedBuffers, false); ok {
					pktUDPBuilder.add(expr, flags, isRegex, idx)
				} else if rawLit, caseless, ok := extractBinaryLiteral(plan.Rule, defaultOnlyAllowedBuffers); ok {
					pktUDPBinBuilder.add(rawLit, caseless, idx)
				} else {
					pf.pktUDP.alwaysEval = append(pf.pktUDP.alwaysEval, idx)
				}
			case "icmp":
				pf.pktICMP.hasPlans = true
				if _, expr, flags, isRegex, ok := extractPrefilterPattern(plan.Rule, defaultOnlyAllowedBuffers, false); ok {
					pktICMPBuilder.add(expr, flags, isRegex, idx)
				} else if rawLit, caseless, ok := extractBinaryLiteral(plan.Rule, defaultOnlyAllowedBuffers); ok {
					pktICMPBinBuilder.add(rawLit, caseless, idx)
				} else {
					pf.pktICMP.alwaysEval = append(pf.pktICMP.alwaysEval, idx)
				}
			case "icmpv6":
				pf.pktICMPv6.hasPlans = true
				if _, expr, flags, isRegex, ok := extractPrefilterPattern(plan.Rule, defaultOnlyAllowedBuffers, false); ok {
					pktICMPv6Builder.add(expr, flags, isRegex, idx)
				} else if rawLit, caseless, ok := extractBinaryLiteral(plan.Rule, defaultOnlyAllowedBuffers); ok {
					pktICMPv6BinBuilder.add(rawLit, caseless, idx)
				} else {
					pf.pktICMPv6.alwaysEval = append(pf.pktICMPv6.alwaysEval, idx)
				}
			case "ip":
				pf.pktIP.hasPlans = true
				if _, expr, flags, isRegex, ok := extractPrefilterPattern(plan.Rule, defaultOnlyAllowedBuffers, false); ok {
					pktIPBuilder.add(expr, flags, isRegex, idx)
				} else if rawLit, caseless, ok := extractBinaryLiteral(plan.Rule, defaultOnlyAllowedBuffers); ok {
					pktIPBinBuilder.add(rawLit, caseless, idx)
				} else {
					pf.pktIP.alwaysEval = append(pf.pktIP.alwaysEval, idx)
				}
			default:
				pf.pktOther = append(pf.pktOther, idx)
			}
		}
	}

	// Finalize HTTP buffer groups in deterministic modifier order.
	httpMods := make([]modifier.Modifier, 0, len(httpBuilders))
	for mod := range httpBuilders {
		httpMods = append(httpMods, mod)
	}
	slices.Sort(httpMods)
	for _, mod := range httpMods {
		grp, fallback := httpBuilders[mod].build()
		for _, idx := range fallback {
			addHTTPAlwaysEval(&pf.streamHTTP, plans[idx], idx)
		}
		if grp == nil {
			continue
		}
		switch {
		case mod == modifier.HTTPRequestHeader:
			pf.streamHTTP.reqHeaderLines = grp
		case mod == modifier.HTTPResponseHeader:
			pf.streamHTTP.resHeaderLines = grp
		case httpRequestOnlyModifiers[mod]:
			pf.streamHTTP.reqBuffers = append(pf.streamHTTP.reqBuffers, bufferGroup{mod: mod, group: grp})
		case httpResponseOnlyModifiers[mod]:
			pf.streamHTTP.respBuffers = append(pf.streamHTTP.respBuffers, bufferGroup{mod: mod, group: grp})
		default:
			pf.streamHTTP.sharedBuffers = append(pf.streamHTTP.sharedBuffers, bufferGroup{mod: mod, group: grp})
		}
	}

	// Finalize TLS buffer groups.
	tlsMods := make([]modifier.Modifier, 0, len(tlsBuilders))
	for mod := range tlsBuilders {
		tlsMods = append(tlsMods, mod)
	}
	slices.Sort(tlsMods)
	for _, mod := range tlsMods {
		grp, fallback := tlsBuilders[mod].build()
		pf.streamTLS.alwaysEval = append(pf.streamTLS.alwaysEval, fallback...)
		if grp != nil {
			if mod == modifier.TLSCerts {
				pf.streamTLS.certsGroup = grp
			} else {
				pf.streamTLS.buffers = append(pf.streamTLS.buffers, bufferGroup{mod: mod, group: grp})
			}
		}
	}

	var fallback []int
	pf.streamTCP.group, fallback = tcpStreamBuilder.build()
	pf.streamTCP.binaryAC = tcpStreamBinBuilder.build()
	pf.streamTCP.alwaysEval = append(pf.streamTCP.alwaysEval, fallback...)
	var tcpToSrv, tcpToCli, tcpAnyDir []int
	for _, idx := range pf.streamTCP.alwaysEval {
		f := plans[idx].Flow
		switch {
		case f != nil && f.ToServer && !f.ToClient:
			tcpToSrv = append(tcpToSrv, idx)
		case f != nil && f.ToClient && !f.ToServer:
			tcpToCli = append(tcpToCli, idx)
		default:
			tcpAnyDir = append(tcpAnyDir, idx)
		}
	}
	pf.streamTCP.alwaysToServer = buildIPFilteredSet(plans, tcpToSrv, false)
	pf.streamTCP.alwaysToClient = buildIPFilteredSet(plans, tcpToCli, false)
	pf.streamTCP.alwaysAnyDir = buildIPFilteredSet(plans, tcpAnyDir, false)
	pf.streamTCP.hasDirSplit = true

	pf.pktDNS.queryGroup, fallback = dnsQueryBuilder.build()
	pf.pktDNS.alwaysEval = append(pf.pktDNS.alwaysEval, fallback...)
	pf.pktDNS.rawGroup, fallback = dnsRawBuilder.build()
	pf.pktDNS.alwaysEval = append(pf.pktDNS.alwaysEval, fallback...)

	pf.pktTCP.group, fallback = pktTCPBuilder.build()
	pf.pktTCP.binaryAC = pktTCPBinBuilder.build()
	pf.pktTCP.alwaysEval = append(pf.pktTCP.alwaysEval, fallback...)
	pf.pktTCP.alwaysIdx = buildIPFilteredSet(plans, pf.pktTCP.alwaysEval, true)

	pf.pktUDP.group, fallback = pktUDPBuilder.build()
	pf.pktUDP.binaryAC = pktUDPBinBuilder.build()
	pf.pktUDP.alwaysEval = append(pf.pktUDP.alwaysEval, fallback...)
	pf.pktUDP.alwaysIdx = buildIPFilteredSet(plans, pf.pktUDP.alwaysEval, false)

	pf.pktICMP.group, fallback = pktICMPBuilder.build()
	pf.pktICMP.binaryAC = pktICMPBinBuilder.build()
	pf.pktICMP.alwaysEval = append(pf.pktICMP.alwaysEval, fallback...)
	pf.pktICMP.alwaysIdx = buildIPFilteredSet(plans, pf.pktICMP.alwaysEval, false)

	pf.pktICMPv6.group, fallback = pktICMPv6Builder.build()
	pf.pktICMPv6.binaryAC = pktICMPv6BinBuilder.build()
	pf.pktICMPv6.alwaysEval = append(pf.pktICMPv6.alwaysEval, fallback...)
	pf.pktICMPv6.alwaysIdx = buildIPFilteredSet(plans, pf.pktICMPv6.alwaysEval, false)

	pf.pktIP.group, fallback = pktIPBuilder.build()
	pf.pktIP.binaryAC = pktIPBinBuilder.build()
	pf.pktIP.alwaysEval = append(pf.pktIP.alwaysEval, fallback...)
	pf.pktIP.alwaysIdx = buildIPFilteredSet(plans, pf.pktIP.alwaysEval, false)

	return pf
}

func addHTTPAlwaysEval(lane *httpStreamLane, plan *CompiledRulePlan, idx int) {
	reqOnly, respOnly := classifyHTTPDirection(plan)
	switch {
	case reqOnly && !respOnly:
		lane.alwaysReq = append(lane.alwaysReq, idx)
	case respOnly && !reqOnly:
		lane.alwaysResp = append(lane.alwaysResp, idx)
	default:
		lane.alwaysBoth = append(lane.alwaysBoth, idx)
	}
}

func classifyHTTPDirection(plan *CompiledRulePlan) (reqOnly, respOnly bool) {
	if plan == nil || plan.Rule == nil || plan.Rule.ContentRuleConfig == nil {
		return false, false
	}
	cfg := plan.Rule.ContentRuleConfig
	if cfg.HTTPConfig != nil {
		reqOnly = true
	}
	for _, content := range cfg.ContentRules {
		if content == nil {
			continue
		}
		if httpRequestOnlyModifiers[content.Modifier] {
			reqOnly = true
		}
		if httpResponseOnlyModifiers[content.Modifier] {
			respOnly = true
		}
	}
	return reqOnly, respOnly
}

func (pf *enginePrefilter) close() {
	if pf == nil {
		return
	}
	pf.streamHTTP.close()
	pf.streamTLS.close()
	pf.streamTCP.close()
	pf.pktDNS.close()
	pf.pktTCP.close()
	pf.pktUDP.close()
	pf.pktICMP.close()
	pf.pktICMPv6.close()
	pf.pktIP.close()
}

func (pf *enginePrefilter) acquireCollector() *candidateCollector {
	c, _ := pf.collectorPool.Get().(*candidateCollector)
	if c == nil {
		c = &candidateCollector{
			seen:    make([]uint32, pf.planCount),
			indices: make([]int, 0, 32),
		}
	}
	c.reset(pf.planCount)
	return c
}

func (pf *enginePrefilter) releaseCollector(c *candidateCollector) {
	if c == nil {
		return
	}
	pf.collectorPool.Put(c)
}

type packetHeaderMeta struct {
	srcAddr  netip.Addr
	dstAddr  netip.Addr
	srcPort  int
	dstPort  int
	hasAddr  bool
	hasPorts bool
}

func extractPacketHeaderMeta(pk gopacket.Packet) packetHeaderMeta {
	var meta packetHeaderMeta
	if pk == nil {
		return meta
	}
	if nw := pk.NetworkLayer(); nw != nil {
		switch network := nw.(type) {
		case *layers.IPv4:
			if s := network.SrcIP.To4(); len(s) == 4 {
				meta.srcAddr = netip.AddrFrom4([4]byte{s[0], s[1], s[2], s[3]})
			}
			if d := network.DstIP.To4(); len(d) == 4 {
				meta.dstAddr = netip.AddrFrom4([4]byte{d[0], d[1], d[2], d[3]})
			}
			meta.hasAddr = meta.srcAddr.IsValid() && meta.dstAddr.IsValid()
		case *layers.IPv6:
			if s := network.SrcIP.To16(); len(s) == 16 {
				meta.srcAddr = netip.AddrFrom16([16]byte(s)).Unmap()
			}
			if d := network.DstIP.To16(); len(d) == 16 {
				meta.dstAddr = netip.AddrFrom16([16]byte(d)).Unmap()
			}
			meta.hasAddr = meta.srcAddr.IsValid() && meta.dstAddr.IsValid()
		}
	}
	if tl := pk.TransportLayer(); tl != nil {
		switch t := tl.(type) {
		case *layers.TCP:
			meta.srcPort = int(t.SrcPort)
			meta.dstPort = int(t.DstPort)
			meta.hasPorts = true
		case *layers.UDP:
			meta.srcPort = int(t.SrcPort)
			meta.dstPort = int(t.DstPort)
			meta.hasPorts = true
		}
	}
	return meta
}

func matchRuleHeaderFast(r *rule.Rule, meta packetHeaderMeta) bool {
	if r == nil || !meta.hasAddr || r.SourceAddress == nil || r.DestinationAddress == nil {
		return true
	}
	checkPorts := meta.hasPorts && r.Protocol != "icmp" && r.Protocol != "icmpv6" &&
		((r.SourcePort != nil && !r.SourcePort.Any) || (r.DestinationPort != nil && !r.DestinationPort.Any))

	fwd := r.SourceAddress.MatchAddr(meta.srcAddr) && r.DestinationAddress.MatchAddr(meta.dstAddr)
	if fwd && checkPorts {
		fwd = (r.SourcePort == nil || r.SourcePort.Match(meta.srcPort)) &&
			(r.DestinationPort == nil || r.DestinationPort.Match(meta.dstPort))
	}
	if fwd {
		return true
	}
	if r.Direction == "<>" {
		rev := r.SourceAddress.MatchAddr(meta.dstAddr) && r.DestinationAddress.MatchAddr(meta.srcAddr)
		if rev && checkPorts {
			rev = (r.SourcePort == nil || r.SourcePort.Match(meta.dstPort)) &&
				(r.DestinationPort == nil || r.DestinationPort.Match(meta.srcPort))
		}
		return rev
	}
	return false
}

func (pf *enginePrefilter) addFiltered(collector *candidateCollector, indices []int, meta packetHeaderMeta) {
	if !meta.hasAddr {
		collector.addSlice(indices)
		return
	}
	for _, idx := range indices {
		if idx >= 0 && idx < len(pf.plans) && matchRuleHeaderFast(pf.plans[idx].Rule, meta) {
			collector.add(idx)
		}
	}
}

// collectStreamCandidates runs fast-fail checks, Parse-Once HTTP/TLS parsing,
// and unified minirehs prefilter scans, returning candidate plan indices in
// strictly ascending order along with any shared HTTP/TLS providers.
func (pf *enginePrefilter) collectStreamCandidates(
	collector *candidateCollector,
	pk gopacket.Packet,
	flow *flowRuntime,
	direction int,
	streamPayload []byte,
	httpPayload []byte,
) (*match.HTTPBufferProvider, *match.TLSBufferProvider, packetHeaderMeta, bool, bool) {
	if pk == nil {
		collector.addSlice(pf.allStreamPlans)
		return nil, nil, packetHeaderMeta{}, false, false
	}
	meta := extractPacketHeaderMeta(pk)

	var sharedHTTP *match.HTTPBufferProvider
	var httpFastFailed bool
	if pf.streamHTTP.hasPlans && len(httpPayload) > 0 {
		if !match.LooksLikeHTTP(httpPayload) {
			httpFastFailed = true
		} else {
			sharedHTTP = match.ParseHTTPStream(pk, httpPayload)
		}
		if sharedHTTP != nil {
			if sharedHTTP.IsRequest() {
				for _, bg := range pf.streamHTTP.reqBuffers {
					if buf := sharedHTTP.Get(bg.mod); len(buf) > 0 {
						bg.group.scan(buf, collector)
					}
				}
				if pf.streamHTTP.reqHeaderLines != nil {
					for _, line := range sharedHTTP.GetHeaderLines(modifier.HTTPRequestHeader) {
						pf.streamHTTP.reqHeaderLines.scan(line, collector)
					}
				}
				pf.addFiltered(collector, pf.streamHTTP.alwaysReq, meta)
			} else if sharedHTTP.IsResponse() {
				for _, bg := range pf.streamHTTP.respBuffers {
					if buf := sharedHTTP.Get(bg.mod); len(buf) > 0 {
						bg.group.scan(buf, collector)
					}
				}
				if pf.streamHTTP.resHeaderLines != nil {
					for _, line := range sharedHTTP.GetHeaderLines(modifier.HTTPResponseHeader) {
						pf.streamHTTP.resHeaderLines.scan(line, collector)
					}
				}
				pf.addFiltered(collector, pf.streamHTTP.alwaysResp, meta)
			}
			for _, bg := range pf.streamHTTP.sharedBuffers {
				if buf := sharedHTTP.Get(bg.mod); len(buf) > 0 {
					bg.group.scan(buf, collector)
				}
			}
			pf.addFiltered(collector, pf.streamHTTP.alwaysBoth, meta)
		}
	}

	var sharedTLS *match.TLSBufferProvider
	var tlsFastFailed bool
	if pf.streamTLS.hasPlans && len(streamPayload) > 0 {
		if !match.LooksLikeTLS(streamPayload) {
			tlsFastFailed = true
		} else {
			sharedTLS = match.ParseTLSStream(pk, streamPayload)
		}
		if sharedTLS != nil {
			for _, bg := range pf.streamTLS.buffers {
				if buf := sharedTLS.Get(bg.mod); len(buf) > 0 {
					bg.group.scan(buf, collector)
				}
			}
			if pf.streamTLS.certsGroup != nil {
				for _, cert := range sharedTLS.GetCerts() {
					if len(cert) > 0 {
						pf.streamTLS.certsGroup.scan(cert, collector)
					}
				}
			}
			pf.addFiltered(collector, pf.streamTLS.alwaysEval, meta)
		}
	}

	if len(pf.streamHTTP2) > 0 && match.LooksLikeHTTP2(streamPayload) {
		pf.addFiltered(collector, pf.streamHTTP2, meta)
	}

	if pf.streamTCP.hasPlans {
		if len(streamPayload) > 0 {
			if pf.streamTCP.group != nil {
				pf.streamTCP.group.scan(streamPayload, collector)
			}
			if pf.streamTCP.binaryAC != nil {
				pf.streamTCP.binaryAC.scan(streamPayload, collector)
			}
		}
		if len(pf.tcpFileDataPlans) > 0 && match.LooksLikeHTTP(streamPayload) {
			hdrProbe := streamPayload
			if len(hdrProbe) > 1024 {
				hdrProbe = hdrProbe[:1024]
			}
			if containsFoldASCII(hdrProbe, "content-encoding:") {
				if sharedHTTP != nil {
					if decomp := sharedHTTP.Get(modifier.FileData); len(decomp) > 0 {
						if pf.streamTCP.group != nil {
							pf.streamTCP.group.scan(decomp, collector)
						}
						if pf.streamTCP.binaryAC != nil {
							pf.streamTCP.binaryAC.scan(decomp, collector)
						}
					} else {
						pf.addFiltered(collector, pf.tcpFileDataPlans, meta)
					}
				} else {
					pf.addFiltered(collector, pf.tcpFileDataPlans, meta)
				}
			}
		}
		if pf.streamTCP.hasDirSplit {
			if flow != nil {
				toSrv, known := flow.toServer(direction)
				if known {
					if toSrv {
						pf.streamTCP.alwaysToServer.collect(pf.plans, collector, meta, false, 0)
					} else {
						pf.streamTCP.alwaysToClient.collect(pf.plans, collector, meta, false, 0)
					}
				} else {
					pf.streamTCP.alwaysToServer.collect(pf.plans, collector, meta, false, 0)
					pf.streamTCP.alwaysToClient.collect(pf.plans, collector, meta, false, 0)
				}
			} else {
				pf.streamTCP.alwaysToServer.collect(pf.plans, collector, meta, false, 0)
				pf.streamTCP.alwaysToClient.collect(pf.plans, collector, meta, false, 0)
			}
			pf.streamTCP.alwaysAnyDir.collect(pf.plans, collector, meta, false, 0)
		} else {
			pf.streamTCP.alwaysIdx.collect(pf.plans, collector, meta, false, 0)
		}
	}

	return sharedHTTP, sharedTLS, meta, httpFastFailed, tlsFastFailed
}

// collectPacketCandidates partitions packet-view evaluation by decoded
// transport/application layer and scans payloads through minirehs prefilters.
func (pf *enginePrefilter) collectPacketCandidates(collector *candidateCollector, pk gopacket.Packet) packetHeaderMeta {
	if pk == nil {
		collector.addSlice(pf.allPacketPlans)
		return packetHeaderMeta{}
	}
	meta := extractPacketHeaderMeta(pk)

	if pf.pktDNS.hasPlans {
		if queryName, rawDNS, isResponse, ok := match.ExtractDNSQueryName(pk); ok {
			if !isResponse && len(queryName) > 0 && pf.pktDNS.queryGroup != nil {
				pf.pktDNS.queryGroup.scan(queryName, collector)
			}
			if len(rawDNS) > 0 && pf.pktDNS.rawGroup != nil {
				pf.pktDNS.rawGroup.scan(rawDNS, collector)
			}
			pf.addFiltered(collector, pf.pktDNS.alwaysEval, meta)
		}
	}

	if pf.pktTCP.hasPlans {
		if tcp, ok := pk.Layer(layers.LayerTypeTCP).(*layers.TCP); ok && tcp != nil {
			if len(tcp.Payload) > 0 {
				if pf.pktTCP.group != nil {
					pf.pktTCP.group.scan(tcp.Payload, collector)
				}
				if pf.pktTCP.binaryAC != nil {
					pf.pktTCP.binaryAC.scan(tcp.Payload, collector)
				}
			}
			pf.pktTCP.alwaysIdx.collect(pf.plans, collector, meta, true, tcpLayerFlagMask(tcp))
		}
	}

	if pf.pktUDP.hasPlans {
		if udp, ok := pk.Layer(layers.LayerTypeUDP).(*layers.UDP); ok && udp != nil {
			if len(udp.Payload) > 0 {
				if pf.pktUDP.group != nil {
					pf.pktUDP.group.scan(udp.Payload, collector)
				}
				if pf.pktUDP.binaryAC != nil {
					pf.pktUDP.binaryAC.scan(udp.Payload, collector)
				}
			}
			pf.pktUDP.alwaysIdx.collect(pf.plans, collector, meta, false, 0)
		}
	}

	if pf.pktICMP.hasPlans {
		if icmp, ok := pk.Layer(layers.LayerTypeICMPv4).(*layers.ICMPv4); ok && icmp != nil {
			if len(icmp.Payload) > 0 {
				if pf.pktICMP.group != nil {
					pf.pktICMP.group.scan(icmp.Payload, collector)
				}
				if pf.pktICMP.binaryAC != nil {
					pf.pktICMP.binaryAC.scan(icmp.Payload, collector)
				}
			}
			pf.pktICMP.alwaysIdx.collect(pf.plans, collector, meta, false, 0)
		}
	}

	if pf.pktICMPv6.hasPlans {
		if icmp6, ok := pk.Layer(layers.LayerTypeICMPv6).(*layers.ICMPv6); ok && icmp6 != nil {
			if len(icmp6.Payload) > 0 {
				if pf.pktICMPv6.group != nil {
					pf.pktICMPv6.group.scan(icmp6.Payload, collector)
				}
				if pf.pktICMPv6.binaryAC != nil {
					pf.pktICMPv6.binaryAC.scan(icmp6.Payload, collector)
				}
			}
			pf.pktICMPv6.alwaysIdx.collect(pf.plans, collector, meta, false, 0)
		}
	}

	if pf.pktIP.hasPlans {
		if netLayer := pk.NetworkLayer(); netLayer != nil {
			if payload := netLayer.LayerPayload(); len(payload) > 0 {
				if pf.pktIP.group != nil {
					pf.pktIP.group.scan(payload, collector)
				}
				if pf.pktIP.binaryAC != nil {
					pf.pktIP.binaryAC.scan(payload, collector)
				}
			}
			pf.pktIP.alwaysIdx.collect(pf.plans, collector, meta, false, 0)
		}
	}

	pf.addFiltered(collector, pf.pktOther, meta)
	return meta
}

// extractPrefilterPattern chooses a single sound necessary condition (literal
// or RE2-safe PCRE) for the rule so any input matching the rule is guaranteed
// to match the returned (modifier, expr, flags) pattern.
func extractPrefilterPattern(
	r *rule.Rule,
	allowed map[modifier.Modifier]bool,
	includeTLSConfig bool,
) (modifier.Modifier, string, minirehs.Flag, bool, bool) {
	if r == nil || r.ContentRuleConfig == nil {
		return modifier.Default, "", 0, false, false
	}

	bestScore := -1 << 30
	var bestMod modifier.Modifier
	var bestExpr string
	var bestFlags minirehs.Flag
	found := false

	considerLiteral := func(mod modifier.Modifier, raw []byte, caseless bool, fastPattern bool) {
		if !allowed[mod] || len(raw) < 2 {
			return
		}
		score := len(raw)*100 + bufferSelectivityBonus(mod)
		if fastPattern && len(raw) >= 4 {
			score += 200
		}
		if mod == modifier.HTTPMethod || isCommonHTTPMethodLiteral(raw) {
			score -= 500
		}
		if !found || score > bestScore {
			var flags minirehs.Flag
			exprBytes := raw
			if caseless {
				flags = minirehs.FlagCaseless
				exprBytes = asciiLowerBytes(raw)
			}
			bestScore = score
			bestMod = mod
			bestExpr = escapeASCIILiteralForRE2(exprBytes)
			bestFlags = flags
			found = true
		}
	}

	if includeTLSConfig && r.ContentRuleConfig.TLSConfig != nil {
		cfg := r.ContentRuleConfig.TLSConfig
		if len(cfg.JA3Hash) >= 2 && isSafeASCIISlice([]byte(cfg.JA3Hash), false) {
			considerLiteral(modifier.JA3Hash, []byte(cfg.JA3Hash), true, true)
		}
		if len(cfg.JA3SHash) >= 2 && isSafeASCIISlice([]byte(cfg.JA3SHash), false) {
			considerLiteral(modifier.JA3SHash, []byte(cfg.JA3SHash), true, true)
		}
	}

	for _, content := range r.ContentRuleConfig.ContentRules {
		if content == nil || content.Negative || content.RawBytes || content.PCREParsed != nil || len(content.Content) == 0 {
			continue
		}
		if !allowed[content.Modifier] {
			continue
		}
		caseless, dotPrefix, stripPseudo, mutating := analyzeTransforms(content)
		if mutating {
			continue
		}
		lit := content.Content
		if dotPrefix && len(lit) > 0 && lit[0] == '.' {
			lit = lit[1:]
		}
		run := longestSafeASCIIRun(lit, stripPseudo)
		if len(run) < 2 {
			continue
		}
		considerLiteral(content.Modifier, run, caseless, content.FastPattern)
	}
	if found {
		return bestMod, bestExpr, bestFlags, false, true
	}

	// Secondary fallback: if the rule has no usable literal content, extract a
	// sound non-relative RE2-compatible PCRE expression on an allowed buffer.
	for _, content := range r.ContentRuleConfig.ContentRules {
		if content == nil || content.Negative || content.RawBytes || content.PCREParsed == nil {
			continue
		}
		if !allowed[content.Modifier] {
			continue
		}
		caseless, dotPrefix, stripPseudo, mutating := analyzeTransforms(content)
		if mutating || dotPrefix || stripPseudo {
			continue
		}
		expr, flags, ok := extractRE2CompatiblePCRE(content.PCREParsed, caseless)
		if !ok {
			continue
		}
		return content.Modifier, expr, flags, true, true
	}

	return modifier.Default, "", 0, false, false
}

// extractBinaryLiteral extracts the longest sound raw-byte literal (>= 2 bytes)
// for rules whose positive content contains NUL or non-ASCII bytes (>= 0x80)
// and therefore could not be indexed in UTF-8 minirehs.
func extractBinaryLiteral(r *rule.Rule, allowed map[modifier.Modifier]bool) ([]byte, bool, bool) {
	if r == nil || r.ContentRuleConfig == nil {
		return nil, false, false
	}
	var best []byte
	bestCaseless := false
	for _, content := range r.ContentRuleConfig.ContentRules {
		if content == nil || content.Negative || content.RawBytes || content.PCREParsed != nil || len(content.Content) < 2 {
			continue
		}
		if !allowed[content.Modifier] {
			continue
		}
		caseless, dotPrefix, stripPseudo, mutating := analyzeTransforms(content)
		if mutating || dotPrefix || stripPseudo {
			continue
		}
		if len(content.Content) > len(best) {
			best = content.Content
			bestCaseless = caseless
		}
	}
	if len(best) < 2 {
		return nil, false, false
	}
	return best, bestCaseless, true
}

func analyzeTransforms(content *rule.ContentRule) (caseless, dotPrefix, stripPseudo, mutating bool) {
	caseless = content.Nocase
	dotPrefix = content.DotPrefix
	for _, t := range content.Transforms {
		switch t {
		case "to_lowercase", "to_uppercase", "header_lowercase":
			caseless = true
		case "dotprefix":
			dotPrefix = true
		case "strip_pseudo_headers":
			stripPseudo = true
		default:
			// url_decode, strip_whitespace, to_sha1, to_sha256, to_md5, or any
			// future byte-mutating transform cannot be prefiltered on raw bytes.
			mutating = true
		}
	}
	return caseless, dotPrefix, stripPseudo, mutating
}

// longestSafeASCIIRun returns the longest contiguous sub-slice whose bytes are
// in [0x01, 0x7F] (UTF-8 rune-identical to raw bytes). When forbidNewlines is
// true (e.g. strip_pseudo_headers), CR and LF break runs so a literal cannot
// span across a stripped header line.
func longestSafeASCIIRun(data []byte, forbidNewlines bool) []byte {
	var best []byte
	start := -1
	for i, b := range data {
		safe := b >= 0x01 && b <= 0x7F && (!forbidNewlines || (b != '\r' && b != '\n'))
		if safe {
			if start < 0 {
				start = i
			}
		} else {
			if start >= 0 {
				if i-start > len(best) {
					best = data[start:i]
				}
				start = -1
			}
		}
	}
	if start >= 0 && len(data)-start > len(best) {
		best = data[start:]
	}
	return best
}

func isSafeASCIISlice(data []byte, forbidNewlines bool) bool {
	return len(longestSafeASCIIRun(data, forbidNewlines)) == len(data)
}

func asciiLowerBytes(data []byte) []byte {
	out := make([]byte, len(data))
	for i, b := range data {
		if b >= 'A' && b <= 'Z' {
			out[i] = b + ('a' - 'A')
		} else {
			out[i] = b
		}
	}
	return out
}

func escapeASCIILiteralForRE2(data []byte) string {
	var sb strings.Builder
	sb.Grow(len(data) * 2)
	for _, b := range data {
		switch b {
		case '\\', '.', '+', '*', '?', '(', ')', '|', '[', ']', '{', '}', '^', '$':
			sb.WriteByte('\\')
			sb.WriteByte(b)
		default:
			if b >= 0x20 && b <= 0x7E {
				sb.WriteByte(b)
			} else {
				fmt.Fprintf(&sb, "\\x%02x", b)
			}
		}
	}
	return sb.String()
}

func extractRE2CompatiblePCRE(p *pcre.PCRE, extraCaseless bool) (string, minirehs.Flag, bool) {
	expr, ignoreCase, multiline, singleline, compatible := p.PrefilterSpec()
	if !compatible {
		return "", 0, false
	}
	// Reject non-ASCII PCRE patterns because Go RE2 operates on UTF-8 runes
	// rather than raw Latin-1/binary bytes (\x80..\xff).
	if !isSafeASCIISlice([]byte(expr), false) {
		return "", 0, false
	}
	// Avoid hex/octal escapes or PCRE constructs whose end-of-subject or inline
	// flag semantics diverge between PCRE2 and Go RE2 ($ before trailing \n,
	// \Z, inline (?...) groups).
	if hasHighByteEscape(expr) || strings.Contains(expr, "(?") || strings.Contains(expr, `\Z`) || hasUnescapedDollar(expr) {
		return "", 0, false
	}
	var flags minirehs.Flag
	if extraCaseless || ignoreCase {
		flags |= minirehs.FlagCaseless
	}
	if singleline {
		flags |= minirehs.FlagDotAll
	}
	if multiline {
		flags |= minirehs.FlagMultiline
	}
	if _, err := syntax.Parse(expr, syntax.Perl); err != nil {
		return "", 0, false
	}
	return expr, flags, true
}

func hasHighByteEscape(pattern string) bool {
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '\\' {
			continue
		}
		if i+1 >= len(pattern) {
			break
		}
		next := pattern[i+1]
		switch {
		case next == 'x':
			if i+3 < len(pattern) {
				hi := pattern[i+2]
				if hi >= '8' && hi <= '9' || hi >= 'a' && hi <= 'f' || hi >= 'A' && hi <= 'F' {
					return true
				}
			}
			if i+2 < len(pattern) && pattern[i+2] == '{' {
				return true
			}
		case next >= '0' && next <= '7':
			return true
		case next == '\\':
			i++
		}
	}
	return false
}

func hasUnescapedDollar(pattern string) bool {
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '\\' {
			i++
			continue
		}
		if pattern[i] == '$' {
			return true
		}
	}
	return false
}

func bufferSelectivityBonus(mod modifier.Modifier) int {
	switch mod {
	case modifier.HTTPUri, modifier.HTTPUriRaw, modifier.HTTPHost, modifier.HTTPHostRaw,
		modifier.HTTPUserAgent, modifier.HTTPRequestBody, modifier.HTTPResponseBody,
		modifier.FileData, modifier.HTTPCookie, modifier.HTTPReferer,
		modifier.TLSSNI, modifier.TLSCertSubject, modifier.TLSCertIssuer,
		modifier.TLSCertSerial, modifier.TLSCertFingerprint,
		modifier.JA3Hash, modifier.JA3SHash, modifier.DNSQuery:
		return 150
	case modifier.HTTPHeader, modifier.HTTPHeaderRaw, modifier.HTTPHeaderNames,
		modifier.HTTPRequestHeader, modifier.HTTPResponseHeader,
		modifier.HTTPServer, modifier.HTTPLocation, modifier.HTTPContentType:
		return 100
	case modifier.HTTPMethod, modifier.HTTPStatCode, modifier.HTTPProtocol:
		return -200
	default:
		return 0
	}
}

func isCommonHTTPMethodLiteral(b []byte) bool {
	switch string(b) {
	case "GET", "POST", "HEAD", "PUT", "HTTP/1.1", "HTTP/1.0", "200", "OK":
		return true
	}
	return false
}

// containsFoldASCII reports whether haystack contains the lowercase ASCII
// needle without allocating.
func containsFoldASCII(haystack []byte, needleLower string) bool {
	n := len(needleLower)
	if n == 0 || len(haystack) < n {
		return false
	}
	limit := len(haystack) - n
	first := needleLower[0]
	for i := 0; i <= limit; i++ {
		c := haystack[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != first {
			continue
		}
		matched := true
		for j := 1; j < n; j++ {
			b := haystack[i+j]
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if b != needleLower[j] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}
