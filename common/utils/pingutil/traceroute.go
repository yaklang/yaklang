package pingutil

import (
	"bytes"
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"net"
	"os"
	"time"
)

type TracerouteResponse struct {
	IP     string
	RTT    int64
	Reason string
	Hop    int
}
type TracerouteConfig struct {
	Ctx          context.Context
	MaxHops      int
	Protocol     string
	WriteTimeOut time.Duration
	ReadTimeOut  time.Duration
	LocalAddr    string
	RetryTimes   int
	UdpPort      int
	FirstTTL     int
	Sender       func(host string, hop int) (*TracerouteResponse, error)
}
type TracerouteConfigOption func(*TracerouteConfig)

func WithSender(f func(config *TracerouteConfig, host string, hop int) (*TracerouteResponse, error)) TracerouteConfigOption {
	return func(cfg *TracerouteConfig) {
		cfg.Sender = func(host string, hop int) (*TracerouteResponse, error) {
			return f(cfg, host, hop)
		}
	}
}

func WithCtx(ctx context.Context) TracerouteConfigOption {
	return func(cfg *TracerouteConfig) {
		cfg.Ctx = ctx
	}
}
func WithMaxHops(hops int) TracerouteConfigOption {
	return func(cfg *TracerouteConfig) {
		cfg.MaxHops = hops
	}
}

func WithProtocol(protocol string) TracerouteConfigOption {
	return func(cfg *TracerouteConfig) {
		cfg.Protocol = protocol
	}
}

func WithWriteTimeout(timeout float64) TracerouteConfigOption {
	return func(cfg *TracerouteConfig) {
		cfg.WriteTimeOut = utils.FloatSecondDuration(timeout)
	}
}

func WithReadTimeout(timeout float64) TracerouteConfigOption {
	return func(cfg *TracerouteConfig) {
		cfg.ReadTimeOut = utils.FloatSecondDuration(timeout)
	}
}

func WithLocalAddr(addr string) TracerouteConfigOption {
	return func(cfg *TracerouteConfig) {
		cfg.LocalAddr = addr
	}
}

func WithRetryTimes(times int) TracerouteConfigOption {
	return func(cfg *TracerouteConfig) {
		cfg.RetryTimes = times
	}
}

func WithUdpPort(port int) TracerouteConfigOption {
	return func(cfg *TracerouteConfig) {
		cfg.UdpPort = port
	}
}

func WithFirstTTL(ttl int) TracerouteConfigOption {
	return func(cfg *TracerouteConfig) {
		cfg.FirstTTL = ttl
	}
}
func NewTracerouteConfig(opts ...TracerouteConfigOption) *TracerouteConfig {
	opts = append([]TracerouteConfigOption{WithSender(func(config *TracerouteConfig, host string, hop int) (*TracerouteResponse, error) {
		var protocol = config.Protocol
		var writeTimeOut = config.WriteTimeOut
		var readTimeOut = config.ReadTimeOut
		var localAddr = config.LocalAddr
		var udpPort = config.UdpPort
		ip := netx.LookupFirst(host)
		dstIp := net.ParseIP(ip)
		switch protocol {
		case "udp":
			conn, err := net.ListenPacket("ip4:icmp", localAddr)
			if err != nil {
				return nil, err
			}
			defer conn.Close()

			udpCon, err := net.ListenPacket("udp4", fmt.Sprintf("%v:%v", localAddr, 0))
			if err != nil {
				return nil, err
			}
			defer udpCon.Close()
			ipConn := ipv4.NewPacketConn(udpCon)
			ipConn.SetWriteDeadline(time.Now().Add(writeTimeOut))
			ipConn.SetTTL(hop)

			start := time.Now()
			dst, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%v:%v", ip, udpPort))
			if err != nil {
				return nil, err
			}
			_, err = ipConn.WriteTo(bytes.Repeat([]byte{0}, 24), nil, dst)
			if err != nil {
				return nil, err
			}
			buff := make([]byte, 512)
			conn.SetReadDeadline(time.Now().Add(readTimeOut))
			_, addr, err := conn.ReadFrom(buff)
			if err != nil {
				return nil, err
			}
			rtt := time.Since(start).Milliseconds()
			return &TracerouteResponse{
				IP:  addr.String(),
				RTT: rtt,
				Hop: hop,
			}, nil
		default:
			icmpMessage := icmp.Message{
				Type: ipv4.ICMPTypeEcho, Code: 0,
				Body: &icmp.Echo{
					ID:   os.Getpid() & 0xffff,
					Data: []byte("HELLO-R-U-THERE"),
				},
			}
			icmpMessage.Body.(*icmp.Echo).Seq = hop
			mb, err := icmpMessage.Marshal(nil)
			if err != nil {
				return nil, err
			}
			conn, err := icmp.ListenPacket("ip4:icmp", localAddr)
			if err != nil {
				return nil, err
			}
			defer conn.Close()
			start := time.Now()
			conn.SetWriteDeadline(time.Now().Add(writeTimeOut))
			conn.IPv4PacketConn().SetTTL(hop)
			if _, err := conn.WriteTo(mb, &net.IPAddr{
				IP: dstIp,
			}); err != nil {
				return nil, err
			}
			conn.SetReadDeadline(time.Now().Add(readTimeOut))
			rb := make([]byte, 512)
			n, addr, err := conn.ReadFrom(rb)
			if err != nil {
				return nil, err
			}
			_, err = icmp.ParseMessage(1, rb[:n])
			if err != nil {
				return nil, err
			}
			rtt := time.Since(start).Milliseconds()
			return &TracerouteResponse{
				IP:  addr.String(),
				RTT: rtt,
				Hop: hop,
			}, nil
		}
	})}, opts...)
	cfg := &TracerouteConfig{
		MaxHops:      30,
		Protocol:     "icmp",
		WriteTimeOut: time.Second * 3,
		ReadTimeOut:  time.Second * 3,
		LocalAddr:    "0.0.0.0",
		RetryTimes:   3,
		UdpPort:      33434,
		FirstTTL:     1,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

func Traceroute(host string, opts ...TracerouteConfigOption) (chan *TracerouteResponse, error) {
	config := NewTracerouteConfig(opts...)
	var ctx = config.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	var maxHops = config.MaxHops
	var retryTimes = config.RetryTimes
	var udpPort = config.UdpPort
	var firstTTL = config.FirstTTL
	ip := netx.LookupFirst(host)
	rsp := make(chan *TracerouteResponse, 0)
	go func() {
		defer func() {
			close(rsp)
		}()
		for i := firstTTL - 1; i < maxHops; i++ {
			hop := i + 1
			//rsps := *TracerouteResponse{}
			for j := 0; j < retryTimes; j++ {
				select {
				case <-ctx.Done():
					return
				default:
				}
				udpPort++
				res, err := config.Sender(ip, hop)
				if err != nil {
					rsp <- &TracerouteResponse{
						IP:     "",
						RTT:    0,
						Hop:    hop,
						Reason: err.Error(),
					}
				} else {
					rsp <- res
					if res.IP == ip {
						return
					}
				}
			}
		}
	}()
	return rsp, nil
}
