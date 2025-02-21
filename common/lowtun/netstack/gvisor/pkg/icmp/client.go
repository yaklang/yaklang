package icmp

import (
	"bytes"
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/sync"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/ipv4"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/icmp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/waiter"
	"github.com/yaklang/yaklang/common/netx/dns_lookup"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/time/rate"
	"math/rand/v2"
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

type PingResult struct {
	MessageType header.ICMPv4Type
	MessageCode header.ICMPv4Code
	MessageID   uint16
	Address     tcpip.Address
	TTL         uint8
}

type ScanConfig struct {
	PingTimeout time.Duration // time out just for ping, default 4s
	Timeout     time.Duration // one target timeout, include link address resolve. default pingTimeout + 4s
	RetryTimes  int
	Concurrent  int
}

type ScanConfigOpt func(*ScanConfig)

func WithPingTimeout(timeout time.Duration) ScanConfigOpt {
	return func(c *ScanConfig) {
		c.PingTimeout = timeout
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

func (c *Client) PingScan(ctx context.Context, target string, opts ...ScanConfigOpt) (chan *PingResult, error) {
	config := &ScanConfig{
		PingTimeout: 4 * time.Second,
		RetryTimes:  0,
		Concurrent:  128,
	}
	for _, opt := range opts {
		opt(config)
	}

	if config.Timeout == 0 {
		config.Timeout = config.PingTimeout + time.Second*4
	}

	res := make(chan *PingResult, 100)
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
					v4, err := c.PingV4(subCtx, t, config.PingTimeout)
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

func (c *Client) PingV4(ctx context.Context, target string, timeout time.Duration) (*PingResult, error) {
	if !utils.IsIPv4(target) {
		target = dns_lookup.LookupFirst(target)
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
			return &PingResult{
				MessageType: icmpV4Packet.Type(),
				MessageCode: icmpV4Packet.Code(),
				MessageID:   icmpV4Packet.Ident(),
				Address:     res.RemoteAddr.Addr,
				TTL:         ttl,
			}, nil
		}
	}
}
