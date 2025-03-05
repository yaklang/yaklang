package icmp

import (
	"bytes"
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/sync"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/ipv4"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/ipv6"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/icmp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/waiter"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/time/rate"
	"math/rand/v2"
	"net"
	"net/netip"
	"sync/atomic"
	"time"
)

type Client struct {
	stack        *stack.Stack
	wq           waiter.Queue
	seqGenerator func() uint16
}

func NewClient(
	s *stack.Stack,
) *Client {
	seqInc := new(atomic.Uint32)
	seqInc.Store(rand.Uint32N(100) + 100)
	c := &Client{
		stack: s,
		seqGenerator: func() uint16 {
			return uint16(seqInc.Add(1))
		},
	}
	return c
}

var icmpPayload = []byte("abcdefghijklmnopqrstuvwabcdefghi")

func (c *Client) newICMPv4EchoRequest() header.ICMPv4 {
	buf := make([]byte, header.ICMPv4MinimumSize+32)
	copy(buf[header.ICMPv4MinimumSize:], icmpPayload)

	icmpPacket := header.ICMPv4(buf)
	icmpPacket.SetType(header.ICMPv4Echo)
	icmpPacket.SetSequence(c.seqGenerator())

	return icmpPacket
}

func (c *Client) newICMPv6EchoRequest() []byte {
	buf := make([]byte, header.ICMPv6MinimumSize+32)
	copy(buf[header.ICMPv6MinimumSize:], icmpPayload)

	icmpPacket := header.ICMPv6(buf)
	icmpPacket.SetType(header.ICMPv6EchoRequest)
	icmpPacket.SetSequence(c.seqGenerator())

	return icmpPacket
}

func (c *Client) newICMPEchoRequest(ipv6 bool) []byte {
	if ipv6 {
		return c.newICMPv6EchoRequest()
	} else {
		return c.newICMPv4EchoRequest()
	}
}

type Result struct {
	MessageCode byte
	MessageType byte
	Address     tcpip.Address
	TTL         uint8
	Ok          bool
	RTT         time.Duration
}

type ScanConfig struct {
	LinkAddressResolveTimeout time.Duration // link address resolve just for ping, default 4s

	Timeout    time.Duration // one target timeout, include link address resolve. default pingTimeout + 4s
	RetryTimes int
	Concurrent int
}

type ScanConfigOpt func(*ScanConfig)

func WithLinkResolveTimeout(timeout time.Duration) ScanConfigOpt {
	return func(c *ScanConfig) {
		c.LinkAddressResolveTimeout = timeout
	}
}

func WithTimeout(timeout time.Duration) ScanConfigOpt {
	return func(c *ScanConfig) {
		c.Timeout = timeout
	}
}

func WithRetryTimes(times int) ScanConfigOpt {
	return func(c *ScanConfig) {
		c.RetryTimes = times
	}
}

func WithConcurrent(concurrent int) ScanConfigOpt {
	return func(c *ScanConfig) {
		c.Concurrent = concurrent
	}
}

func (c *Client) PingScan(ctx context.Context, target string, opts ...ScanConfigOpt) (chan *Result, error) {
	config := &ScanConfig{
		LinkAddressResolveTimeout: 2 * time.Second,
		RetryTimes:                0,
		Concurrent:                128,
	}
	for _, opt := range opts {
		opt(config)
	}

	if config.Timeout == 0 {
		config.Timeout = config.LinkAddressResolveTimeout + time.Second*4
	}

	res := make(chan *Result, 100)
	go func() {
		defer close(res)
		targetList := utils.ParseStringToHosts(target)
		pingLimiter := rate.NewLimiter(rate.Limit(config.Concurrent), 1)
		wg := new(sync.WaitGroup)
		for _, t := range targetList {
			waitErr := pingLimiter.Wait(ctx)
			if waitErr != nil {
				log.Errorf("ping limiter wait fail: %v", waitErr)
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i <= config.RetryTimes; i++ {
					subCtx, _ := context.WithTimeout(ctx, config.Timeout)
					v4, err := c.Ping(subCtx, t, config.LinkAddressResolveTimeout)
					if err != nil {
						//log.Errorf("ping %s fail: %v", t, err)
						continue
					}
					res <- v4
					return
				}
			}()
		}
		wg.Wait()
	}()

	return res, nil
}

func (c *Client) Ping(ctx context.Context, target string, connectTimeout time.Duration) (*Result, error) {
	ip := net.ParseIP(target)
	if ip == nil {
		target = netx.LookupFirst(target)
		ip = net.ParseIP(target)
		if ip == nil {
			return nil, utils.Errorf("parse ip fail")
		}
	}

	var ep tcpip.Endpoint
	var e = new(waiter.Queue)
	var err tcpip.Error
	var remoteAddr tcpip.FullAddress
	var isIpv6 bool
	if ipIns := ip.To4(); ipIns != nil {
		remoteAddr = tcpip.FullAddress{Addr: tcpip.AddrFrom4([4]byte(ipIns))}
		ep, err = icmp.NewProtocol4(c.stack).NewEndpoint(ipv4.ProtocolNumber, e)
	} else if ipIns = ip.To16(); ipIns != nil {
		isIpv6 = true
		remoteAddr = tcpip.FullAddress{Addr: tcpip.AddrFrom16([16]byte(ipIns))}
		ep, err = icmp.NewProtocol6(c.stack).NewEndpoint(ipv6.ProtocolNumber, e)
	} else {
		return nil, utils.Errorf("ip type not support")
	}
	if err != nil {
		return nil, utils.Errorf("icmp new endpoint fail: %v", err)
	}
	defer ep.Close()

	// register write able event
	writeWE, write := waiter.NewChannelEntry(waiter.WritableEvents)
	e.EventRegister(&writeWE)
	defer e.EventUnregister(&writeWE)

	err = ep.Connect(remoteAddr)

	if _, ok := err.(*tcpip.ErrConnectStarted); ok {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(connectTimeout):
			return nil, utils.Errorf("connect timeout")
		case <-write:
		}
	}

	var b bytes.Buffer
	var r bytes.Reader
	echoRequest := c.newICMPEchoRequest(isIpv6)
	r.Reset(echoRequest)

	start := time.Now()
	_, err = ep.Write(&r, tcpip.WriteOptions{
		To: &remoteAddr,
	})
	if err != nil {
		return nil, utils.Errorf("endpoint write echo request fail: %v", err)
	}

	// register read able event
	readWE, read := waiter.NewChannelEntry(waiter.EventIn)
	e.EventRegister(&readWE)
	defer e.EventUnregister(&readWE)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-read:
			RTT := time.Since(start)
			res, err := ep.Read(&b, tcpip.ReadOptions{
				NeedRemoteAddr: true,
			})
			if err != nil {
				if _, ok := err.(*tcpip.ErrWouldBlock); ok {
					continue
				}
				return nil, utils.Errorf("icmp read fail: %v", err)
			}

			ttl := uint8(0)
			if res.ControlMessages.HasTTL {
				ttl = res.ControlMessages.TTL
			}
			v := b.Bytes()
			if isIpv6 {
				icmpPacket := header.ICMPv6(v)
				return &Result{
					MessageCode: byte(icmpPacket.Code()),
					MessageType: byte(icmpPacket.Type()),
					Address:     res.RemoteAddr.Addr,
					TTL:         ttl,
					Ok:          icmpPacket.Type() == header.ICMPv6EchoReply,
					RTT:         RTT,
				}, nil
			} else {
				icmpPacket := header.ICMPv4(v)
				return &Result{
					MessageCode: byte(icmpPacket.Code()),
					MessageType: byte(icmpPacket.Type()),
					Address:     res.RemoteAddr.Addr,
					TTL:         ttl,
					Ok:          icmpPacket.Type() == header.ICMPv4EchoReply,
					RTT:         RTT,
				}, nil
			}

		}
	}
}

//func (c *Client) GetPinger() func(ip string, timeout time.Duration) *pingutil.PingResult {
//	return func(ip string, timeout time.Duration) *pingutil.PingResult {
//		ctx, cancel := context.WithTimeout(context.Background(), timeout+4*time.Second)
//		defer cancel()
//		res, err := c.Ping(ctx, ip, timeout)
//		if err != nil {
//			return &pingutil.PingResult{
//				IP:     ip,
//				Reason: fmt.Sprintf("netstack ping %s fail: %v", ip, err),
//			}
//		}
//		return CreatePingResult(res)
//	}
//}
//

func (c *Client) PingV4(ctx context.Context, target string, timeout time.Duration) (*Result, error) {
	if !utils.IsIPv4(target) {
		target = netx.LookupFirst(target)
	}

	ipv4Ins, parseErr := netip.ParseAddr(target)
	if parseErr != nil {
		return nil, utils.Errorf("parse addr fail: %v", parseErr)
	}
	remoteAddr := tcpip.FullAddress{Addr: tcpip.AddrFrom4(ipv4Ins.As4())}
	e := new(waiter.Queue)
	ep, err := icmp.NewProtocol4(c.stack).NewEndpoint(ipv4.ProtocolNumber, e)
	if err != nil {
		return nil, utils.Errorf("icmp new endpoint fail: %v", err)
	}
	defer ep.Close()

	// register write able event
	writeWE, write := waiter.NewChannelEntry(waiter.WritableEvents)
	e.EventRegister(&writeWE)
	defer e.EventUnregister(&writeWE)

	err = ep.Connect(remoteAddr)
	if _, ok := err.(*tcpip.ErrConnectStarted); ok {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-write:
		}
	}

	var b bytes.Buffer
	var r bytes.Reader
	echoRequest := c.newICMPv4EchoRequest()
	r.Reset(echoRequest)
	_, err = ep.Write(&r, tcpip.WriteOptions{
		To: &remoteAddr,
	})
	if err != nil {
		return nil, utils.Errorf("endpoint write echo request fail: %v", err)
	}

	writeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// register read able event
	readWE, read := waiter.NewChannelEntry(waiter.EventIn)
	e.EventRegister(&readWE)
	defer e.EventUnregister(&readWE)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-writeCtx.Done():
			return nil, utils.Errorf("ping %s timeout", target)
		case <-read:
			res, err := ep.Read(&b, tcpip.ReadOptions{
				NeedRemoteAddr: true,
			})
			if err != nil {
				if _, ok := err.(*tcpip.ErrWouldBlock); ok {
					continue
				}
				return nil, utils.Errorf("icmp read fail: %v", err)
			}
			v := b.Bytes()
			icmpV4Packet := header.ICMPv4(v)
			if icmpV4Packet.Type() == header.ICMPv4Echo {
				return nil, utils.Errorf("icmp type is echo")
			}
			ttl := uint8(0)
			if res.ControlMessages.HasTTL {
				ttl = res.ControlMessages.TTL
			}
			return &Result{
				MessageCode: byte(icmpV4Packet.Code()),
				Address:     res.RemoteAddr.Addr,
				TTL:         ttl,
			}, nil
		}
	}
}
