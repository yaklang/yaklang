package match

import (
	"fmt"
	"sync"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/data"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/data/protocol"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"golang.org/x/exp/slices"
)

// Matcher is the low-level stateless rule evaluator. StatefulEngine embeds it
// per rule and adds flow state, alerts, capture binding and pooling. New code
// should prefer StatefulEngine; Matcher and Group remain for compatibility.
type Matcher struct {
	matcher *matchContext
}

func CompileRule(r *rule.Rule) (*Matcher, error) {
	return compileValidatedRule(r, false)
}

// CompileStatefulRule accepts stateful-required diagnostics while preserving
// fail-closed behavior for unsupported rule semantics.
func CompileStatefulRule(r *rule.Rule) (*Matcher, error) {
	return compileValidatedRule(r, true)
}

func compileValidatedRule(r *rule.Rule, allowStateful bool) (*Matcher, error) {
	diagnostics := rule.ValidateRule(r)
	if allowStateful {
		diagnostics = slices.DeleteFunc(diagnostics, func(diagnostic rule.ValidationDiagnostic) bool {
			return diagnostic.Capability == rule.KeywordStatefulRequired
		})
	}
	if len(diagnostics) > 0 {
		return nil, &RuleCompileError{Diagnostics: diagnostics}
	}
	ctx, err := compile(r)
	if err != nil {
		return nil, err
	}
	return &Matcher{
		matcher: ctx,
	}, nil
}
func New(r *rule.Rule) *Matcher {
	ctx, err := compile(r)
	if err != nil {
		log.Error(err)
		return &Matcher{}
	}
	return &Matcher{matcher: ctx}
}

// Clone returns an independent matcher sharing only the immutable rule and
// workflow. Clones can run concurrently because match state lives in the
// per-clone matchContext.
func (m *Matcher) Clone() *Matcher {
	if m == nil || m.matcher == nil {
		return &Matcher{}
	}
	return &Matcher{matcher: m.matcher.Clone()}
}

// Reset releases per-match packet, payload and buffer references. Call it
// before returning a matcher to a pool so pooled matchers do not retain the
// previous capture's memory.
func (m *Matcher) Reset() {
	if m == nil || m.matcher == nil {
		return
	}
	m.matcher.Tidy()
}

func (m *Matcher) Match(flow []byte) bool {
	if m == nil || m.matcher == nil || len(flow) == 0 {
		return false
	}
	pk := gopacket.NewPacket(flow, layers.LayerTypeEthernet, gopacket.NoCopy)
	return m.MatchPackage(pk)
}

func (m *Matcher) MatchHTTPFlow(flow *HttpFlow) bool {
	if m == nil || m.matcher == nil || flow == nil {
		return false
	}
	for _, packet := range flow.ToRequestPacket() {
		if m.MatchPackage(packet) {
			return true
		}
	}
	return false
}

func (m *Matcher) MatchPackage(pk gopacket.Packet) bool {
	matched, _ := m.MatchPackageWithError(pk)
	return matched
}

func (m *Matcher) MatchPackageWithError(pk gopacket.Packet) (bool, error) {
	if m == nil || m.matcher == nil || pk == nil {
		return false, nil
	}
	err, matched := m.matcher.Match(pk)
	return matched, err
}

// MatchStream evaluates payload rules against ordered reassembled bytes while
// retaining the real packet for header/address/port checks.
func (m *Matcher) MatchStream(pk gopacket.Packet, payload []byte) (bool, error) {
	matched, _, err := m.MatchStreamEvidence(pk, payload)
	return matched, err
}

// MatchStreamEvidence also returns the end offset of the complete signature's
// final payload evidence, allowing callers to suppress replay of old matches.
func (m *Matcher) MatchStreamEvidence(pk gopacket.Packet, payload []byte) (bool, int, error) {
	if m == nil || m.matcher == nil || pk == nil {
		return false, 0, nil
	}
	return m.matcher.MatchPayloadEvidence(pk, payload)
}

// MatchStreamEvidenceWithHTTP evaluates a stream rule using a pre-parsed HTTP
// buffer provider so the engine parses each HTTP message only once per event.
func (m *Matcher) MatchStreamEvidenceWithHTTP(pk gopacket.Packet, payload []byte, shared *HTTPBufferProvider) (bool, int, error) {
	return m.MatchStreamEvidenceWithShared(pk, payload, shared, nil)
}

// MatchStreamEvidenceWithShared evaluates a stream rule using pre-parsed HTTP
// and/or TLS buffer providers so the engine parses each stream once per event.
func (m *Matcher) MatchStreamEvidenceWithShared(pk gopacket.Packet, payload []byte, sharedHTTP *HTTPBufferProvider, sharedTLS *TLSBufferProvider) (bool, int, error) {
	return m.MatchStreamEvidenceWithDCERPC(pk, payload, sharedHTTP, sharedTLS, nil)
}

// MatchStreamEvidenceWithDCERPC evaluates a stream rule using pre-parsed HTTP
// and/or TLS buffer providers and optional flow-level DCE/RPC context bindings
// bridged from pcaputil protocol dissection.
func (m *Matcher) MatchStreamEvidenceWithDCERPC(pk gopacket.Packet, payload []byte, sharedHTTP *HTTPBufferProvider, sharedTLS *TLSBufferProvider, dcerpcBindings map[uint16]DCERPCBinding) (bool, int, error) {
	if m == nil || m.matcher == nil || pk == nil {
		return false, 0, nil
	}
	var innerHTTP *httpProvider
	if sharedHTTP != nil {
		innerHTTP = sharedHTTP.inner
	}
	var innerTLS *tlsProvider
	if sharedTLS != nil {
		innerTLS = sharedTLS.inner
	}
	return m.matcher.matchPayloadEvidenceWithShared(pk, payload, innerHTTP, innerTLS, dcerpcBindings)
}

// MatchPackageWithDCERPC evaluates a packet rule with optional flow-level
// DCE/RPC context bindings.
func (m *Matcher) MatchPackageWithDCERPC(pk gopacket.Packet, dcerpcBindings map[uint16]DCERPCBinding) (bool, error) {
	if m == nil || m.matcher == nil || pk == nil {
		return false, nil
	}
	matched, _, err := m.matcher.matchPayloadEvidenceWithShared(pk, nil, nil, nil, dcerpcBindings)
	return matched, err
}

type matchHandler func(*matchContext) error

type bufferProvider func(modifier modifier.Modifier) []byte

type jumpCursorState struct {
	position int
	active   bool
}

type matchContext struct {
	// matcher itself not designed for multi threads, you'd best clone it before use.
	// lock is used to protect the matcher from being used by multiple goroutines.
	lock sync.Mutex

	rejected bool
	pos      int

	provider bufferProvider
	buffer   map[modifier.Modifier][]byte

	Value map[string]any

	prevMatched  []data.Matched
	prevModifier modifier.Modifier
	jumpCursor   map[modifier.Modifier]jumpCursorState
	variables    map[string]int64

	PK                  gopacket.Packet
	Rule                *rule.Rule
	overridePayload     []byte
	sharedHTTP          *httpProvider
	sharedTLS           *tlsProvider
	dcerpcBindings      map[uint16]DCERPCBinding
	protocolEvidenceEnd int
	fileDataOffset      int

	workflow []matchHandler
}

func (c *matchContext) Clone() *matchContext {
	return &matchContext{
		pos:      -1,
		Rule:     c.Rule,
		workflow: c.workflow,
	}
}

func (c *matchContext) Reject() {
	c.rejected = true
}

func (c *matchContext) Recover() {
	c.rejected = false
}

func (c *matchContext) IsRejected() bool {
	return c.rejected
}

func (c *matchContext) Attach(handler ...matchHandler) {
	c.workflow = append(c.workflow, handler...)
}

func (c *matchContext) Insert(handler ...matchHandler) {
	if c.pos+1 < len(c.workflow) {
		c.workflow = append(c.workflow[:c.pos+1], append(handler, c.workflow[c.pos+1:]...)...)
	} else {
		c.workflow = append(c.workflow, handler...)
	}
}

func (c *matchContext) Next() error {
	c.pos++
	defer func() {
		c.pos--
	}()
	if c.rejected || c.pos >= len(c.workflow) {
		return nil
	}
	if err := c.workflow[c.pos](c); err != nil {
		return err
	}
	return c.Next()
}

func (c *matchContext) Must(ok bool) bool {
	if !ok {
		c.Reject()
	}
	return ok
}

func (c *matchContext) Tidy() {
	if c.Value == nil {
		c.Value = make(map[string]any, 4)
	} else {
		clear(c.Value)
	}
	c.PK = nil
	c.overridePayload = nil
	c.sharedHTTP = nil
	c.sharedTLS = nil
	c.dcerpcBindings = nil
	c.protocolEvidenceEnd = 0
	c.fileDataOffset = 0
	c.provider = nil
	if c.buffer == nil {
		c.buffer = make(map[modifier.Modifier][]byte, 4)
	} else {
		clear(c.buffer)
	}
	c.pos = -1
	c.rejected = false
	c.prevMatched = nil
	c.prevModifier = modifier.Default
	if c.jumpCursor != nil {
		clear(c.jumpCursor)
	}
	if c.variables != nil {
		clear(c.variables)
	}
}

func compile(r *rule.Rule) (*matchContext, error) {
	c := &matchContext{
		Rule: r,
		pos:  -1,
	}

	if err := matchMutex(c); err != nil {
		return c, fmt.Errorf("match mutex failed: %s", err.Error())
	}

	return c, nil
}

func (c *matchContext) match() (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("match flow failed: %v", e)
		}
	}()
	err = c.Next()
	if err != nil {
		return err
	}
	return nil
}
func (c *matchContext) Match(pk gopacket.Packet) (error, bool) {
	return c.matchPacket(pk, nil)
}

func (c *matchContext) MatchPayload(pk gopacket.Packet, payload []byte) (bool, error) {
	matched, _, err := c.MatchPayloadEvidence(pk, payload)
	return matched, err
}

func (c *matchContext) MatchPayloadEvidence(pk gopacket.Packet, payload []byte) (bool, int, error) {
	return c.matchPayloadEvidenceWithShared(pk, payload, nil, nil, nil)
}

func (c *matchContext) matchPayloadEvidenceWithShared(pk gopacket.Packet, payload []byte, sharedHTTP *httpProvider, sharedTLS *tlsProvider, dcerpcBindings map[uint16]DCERPCBinding) (bool, int, error) {
	c.lock.Lock()
	c.Tidy()
	defer c.lock.Unlock()
	c.PK = pk
	c.overridePayload = payload
	c.sharedHTTP = sharedHTTP
	c.sharedTLS = sharedTLS
	c.dcerpcBindings = dcerpcBindings
	if err := c.match(); err != nil {
		return false, 0, fmt.Errorf("match failed: %w", err)
	}
	if c.rejected {
		return false, 0, nil
	}
	end := c.protocolEvidenceEnd
	baseOffset := 0
	if c.prevModifier == modifier.FileData || c.prevModifier == modifier.FileMagic {
		baseOffset = c.fileDataOffset
	}
	for _, evidence := range c.prevMatched {
		if current := baseOffset + evidence.Pos + evidence.Len; current > end {
			end = current
		}
	}
	return true, end, nil
}

func (c *matchContext) matchPacket(pk gopacket.Packet, overridePayload []byte) (error, bool) {
	c.lock.Lock()
	c.Tidy()
	defer c.lock.Unlock()
	c.PK = pk
	c.overridePayload = overridePayload
	err := c.match()
	if err != nil {
		return fmt.Errorf("match failed: %w", err), false
	}
	return nil, !c.rejected
}

func matchMutex(c *matchContext) error {
	switch c.Rule.Protocol {
	case protocol.IP:
		c.Attach(ipMatcher, ipCfgMatch, ipParser)
		if c.Rule.SourcePort != nil && !c.Rule.SourcePort.Any ||
			c.Rule.DestinationPort != nil && !c.Rule.DestinationPort.Any {
			c.Attach(portMatcher)
		}
		if err := attachFastPattern(c); err != nil {
			return err
		}
		return attachPayloadMatcher(c)
	case protocol.DNS:
		c.Attach(ipMatcher, ipCfgMatch, portMatcher, dnsParser)
		if err := attachFastPattern(c); err != nil {
			return err
		}
		c.Attach(dnsMatcher)
		return attachPayloadMatcher(c)
	case protocol.HTTP, protocol.HTTP1:
		c.Attach(ipMatcher, ipCfgMatch, portMatcher, httpParser)
		if c.Rule.ContentRuleConfig != nil && c.Rule.ContentRuleConfig.HTTP2Config != nil {
			c.Attach(http2Matcher)
		}
		if err := attachFastPattern(c); err != nil {
			return err
		}
		attachHTTPMatcher(c)
		return attachPayloadMatcher(c)
	case protocol.HTTP2:
		c.Attach(ipMatcher, ipCfgMatch, portMatcher, tcpParser, http2Matcher)
		if err := attachFastPattern(c); err != nil {
			return err
		}
		c.Attach(tcpCfgMatch)
		return attachPayloadMatcher(c)
	case protocol.TCP, protocol.TCPPacket:
		if rule.UsesSSHBuffers(c.Rule) {
			c.Attach(ipMatcher, ipCfgMatch, portMatcher, sshProtoParser)
			if err := attachFastPattern(c); err != nil {
				return err
			}
			c.Attach(tcpCfgMatch)
			return attachPayloadMatcher(c)
		}
		if rule.UsesHTTPBuffers(c.Rule) {
			// A tcp rule that selects HTTP sticky buffers carries HTTP even
			// when the header protocol is not literally "http".
			c.Attach(ipMatcher, ipCfgMatch, portMatcher, httpParser)
			c.Attach(tcpCfgMatch)
			if err := attachFastPattern(c); err != nil {
				return err
			}
			attachHTTPMatcher(c)
			return attachPayloadMatcher(c)
		}
		if rule.UsesTLSBuffers(c.Rule) {
			c.Attach(ipMatcher, ipCfgMatch, portMatcher, tlsParser)
			c.Attach(tcpCfgMatch)
			if err := attachFastPattern(c); err != nil {
				return err
			}
			return attachPayloadMatcher(c)
		}
		c.Attach(ipMatcher, ipCfgMatch, portMatcher, tcpParser)
		if rule.UsesDCERPC(c.Rule) {
			c.Attach(dcerpcMatcher)
		}
		if c.Rule.ContentRuleConfig != nil && c.Rule.ContentRuleConfig.HTTP2Config != nil {
			c.Attach(http2Matcher)
		}
		if c.Rule.ContentRuleConfig != nil && c.Rule.ContentRuleConfig.SNMPConfig != nil {
			c.Attach(snmpMatcher)
		}
		if err := attachFastPattern(c); err != nil {
			return err
		}
		c.Attach(tcpCfgMatch)
		return attachPayloadMatcher(c)
	case protocol.SMTP:
		parser := tcpParser
		if rule.UsesFileData(c.Rule) {
			parser = smtpParser
		}
		c.Attach(ipMatcher, ipCfgMatch, portMatcher, parser)
		if err := attachFastPattern(c); err != nil {
			return err
		}
		c.Attach(tcpCfgMatch)
		return attachPayloadMatcher(c)
	case protocol.FTP, protocol.FTPData, protocol.SMB,
		protocol.SSH, protocol.RDP, protocol.DCERPC, protocol.Telnet, protocol.TCPStream:
		// Application protocols that ride on TCP share the TCP header/payload
		// pipeline. SSH additionally exposes its version banner as a buffer.
		parser := tcpParser
		if protocol.SSH == c.Rule.Protocol && rule.UsesSSHBuffers(c.Rule) {
			parser = sshProtoParser
		}
		c.Attach(ipMatcher, ipCfgMatch, portMatcher, parser)
		if rule.UsesDCERPC(c.Rule) {
			c.Attach(dcerpcMatcher)
		}
		if err := attachFastPattern(c); err != nil {
			return err
		}
		c.Attach(tcpCfgMatch)
		return attachPayloadMatcher(c)
	case protocol.SNMP:
		c.Attach(ipMatcher, ipCfgMatch, portMatcher, snmpParser, snmpMatcher)
		if err := attachFastPattern(c); err != nil {
			return err
		}
		return attachPayloadMatcher(c)
	case protocol.SIP:
		c.Attach(ipMatcher, ipCfgMatch, portMatcher, tcpOrUDPParser)
		if err := attachFastPattern(c); err != nil {
			return err
		}
		return attachPayloadMatcher(c)
	case protocol.UDP, protocol.IKE, protocol.BitTorrentDHT:
		c.Attach(ipMatcher, ipCfgMatch, portMatcher, udpParser)
		if rule.UsesDCERPC(c.Rule) {
			c.Attach(dcerpcMatcher)
		}
		if c.Rule.ContentRuleConfig != nil && c.Rule.ContentRuleConfig.SNMPConfig != nil {
			c.Attach(snmpMatcher)
		}
		if err := attachFastPattern(c); err != nil {
			return err
		}
		return attachPayloadMatcher(c)
	case protocol.ICMP:
		c.Attach(ipMatcher, ipCfgMatch, icmpParser)
		if err := attachFastPattern(c); err != nil {
			return err
		}
		c.Attach(icmpCfgMatch)
		return attachPayloadMatcher(c)
	case protocol.ICMPv6:
		c.Attach(ipMatcher, ipCfgMatch, icmpv6Parser)
		if err := attachFastPattern(c); err != nil {
			return err
		}
		c.Attach(icmpv6CfgMatch)
		return attachPayloadMatcher(c)
	case protocol.TLS:
		c.Attach(ipMatcher, ipCfgMatch, portMatcher, tlsParser)
		if c.Rule.ContentRuleConfig.TLSConfig != nil {
			c.Attach(tlsMatcher)
		}
		if err := attachFastPattern(c); err != nil {
			return err
		}
		return attachPayloadMatcher(c)
	default:
		return fmt.Errorf("unsupported protocol: %s", c.Rule.Protocol)
	}
}

func (c *matchContext) SetBufferProvider(p func(modifier modifier.Modifier) []byte) {
	c.provider = p
}

func (c *matchContext) GetBuffer(modifier modifier.Modifier) []byte {
	if _, ok := c.buffer[modifier]; !ok {
		c.buffer[modifier] = c.provider(modifier)
	}
	return c.buffer[modifier]
}

func (c *matchContext) SetBuffer(modifier modifier.Modifier, buf []byte) {
	c.buffer[modifier] = buf
}

func (c *matchContext) takeJumpCursor(mdf modifier.Modifier) (int, bool) {
	state, ok := c.jumpCursor[mdf]
	if !ok || !state.active {
		return 0, false
	}
	delete(c.jumpCursor, mdf)
	return state.position, true
}

// GetPrevMatched return true if the previous motch for current modifier existed.
func (c *matchContext) GetPrevMatched(mdf modifier.Modifier) ([]data.Matched, bool) {
	if c.prevModifier == mdf {
		return c.prevMatched, true
	}
	return nil, false
}

func (c *matchContext) SetPrevMatched(mdf modifier.Modifier, matched []data.Matched) {
	c.prevModifier = mdf
	c.prevMatched = matched
}
