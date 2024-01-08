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
	"syscall"
	"time"
)

type TracerouteResponse struct {
	IPs    []string
	RTT    int64
	Reason string
	Hop    int
}

func traceroute(ctx context.Context, host string) (chan []*TracerouteResponse, error) {
	const maxHops = 30
	const protocol = "icmp"
	const writeTimeOut = time.Second * 3
	const readTimeOut = time.Second * 3
	const localAddr = "0.0.0.0"
	ip := netx.LookupFirst(host)
	dstIp := net.ParseIP(ip)
	rsp := make(chan []*TracerouteResponse, 0)
	//randDstPort := rand.Intn(10000) + 40000 // 40000-50000
	randDstPort := 33434
	go func() {
		defer func() {
			close(rsp)
		}()
		for i := 0; i < maxHops; i++ {
			hop := i + 1
			rsps := []*TracerouteResponse{}
			for j := 0; j < 3; j++ {
				select {
				case <-ctx.Done():
					return
				default:
				}
				randDstPort++
				res, err := func() (*TracerouteResponse, error) {
					switch protocol {
					case "udp":
						conn, err := net.ListenPacket("ip4:icmp", localAddr)
						if err != nil {
							return nil, err
						}
						defer conn.Close()
						dialer := net.Dialer{
							Timeout: time.Second,
							Control: func(network, address string, c syscall.RawConn) error {
								return c.Control(func(fd uintptr) {
									syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_TTL, hop)
								})
							},
						}

						udpCon, err := dialer.Dial("udp", fmt.Sprintf("%s:%d", ip, randDstPort))
						if err != nil {
							return nil, err
						}
						defer udpCon.Close()
						start := time.Now()
						err = udpCon.SetReadDeadline(time.Now().Add(writeTimeOut))
						if err != nil {
							return nil, err
						}
						_, err = udpCon.Write(bytes.Repeat([]byte{0}, 24))
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
							IPs: []string{addr.String()},
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
							IPs: []string{addr.String()},
							RTT: rtt,
							Hop: hop,
						}, nil
					}
				}()
				if err != nil {
					rsps = append(rsps, &TracerouteResponse{
						IPs:    []string{},
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
				if utils.StringArrayContains(rsp.IPs, ip) {
					return
				}
			}
		}
	}()
	return rsp, nil
}
