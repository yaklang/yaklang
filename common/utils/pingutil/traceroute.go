package pingutil

import (
	"bytes"
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/netx"
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
	MaxHops      int
	Protocol     string
	WriteTimeOut time.Duration
	ReadTimeOut  time.Duration
	LocalAddr    string
	RetryTimes   int
	UdpPort      int
	FirstTTL     int
}

func TracerouteWithConfig(ctx context.Context, host string, config *TracerouteConfig) (chan []*TracerouteResponse, error) {
	var maxHops = config.MaxHops
	var protocol = config.Protocol
	var writeTimeOut = config.WriteTimeOut
	var readTimeOut = config.ReadTimeOut
	var localAddr = config.LocalAddr
	var retryTimes = config.RetryTimes
	var udpPort = config.UdpPort
	var firstTTL = config.FirstTTL
	ip := netx.LookupFirst(host)
	dstIp := net.ParseIP(ip)
	rsp := make(chan []*TracerouteResponse, 0)
	go func() {
		defer func() {
			close(rsp)
		}()
		for i := firstTTL - 1; i < maxHops; i++ {
			hop := i + 1
			rsps := []*TracerouteResponse{}
			for j := 0; j < retryTimes; j++ {
				select {
				case <-ctx.Done():
					return
				default:
				}
				udpPort++
				res, err := func() (*TracerouteResponse, error) {
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
						icmpMessage.Body.(*icmp.Echo).Seq = i
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
				}()
				if err != nil {
					rsps = append(rsps, &TracerouteResponse{
						IP:     "",
						RTT:    0,
						Hop:    hop,
						Reason: err.Error(),
					})
				} else {
					rsps = append(rsps, res)
				}
			}
			rsp <- rsps
			for _, rsp := range rsps {
				if rsp.IP == ip {
					return
				}
			}
		}
	}()
	return rsp, nil
}
func Traceroute(ctx context.Context, host string) (chan []*TracerouteResponse, error) {
	return TracerouteWithConfig(ctx, host, &TracerouteConfig{
		MaxHops:      30,
		Protocol:     "udp",
		WriteTimeOut: time.Second * 3,
		ReadTimeOut:  time.Second * 3,
		LocalAddr:    "0.0.0.0",
		RetryTimes:   3,
		UdpPort:      33434,
		FirstTTL:     1,
	})
}
