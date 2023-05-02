package yakgrpc

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cybertunnel"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) GetTunnelServerExternalIP(ctx context.Context, p *ypb.GetTunnelServerExternalIPParams) (*ypb.GetTunnelServerExternalIPResponse, error) {
	ip, err := cybertunnel.GetTunnelServerExternalIP(p.GetAddr(), p.GetSecret())
	if err != nil {
		return nil, err
	}
	return &ypb.GetTunnelServerExternalIPResponse{IP: ip.String()}, nil
}

func (s *Server) VerifyTunnelServerDomain(ctx context.Context, p *ypb.VerifyTunnelServerDomainParams) (*ypb.VerifyTunnelServerDomainResponse, error) {
	ip, err := cybertunnel.GetTunnelServerExternalIP(p.GetConnectParams().GetAddr(), p.GetConnectParams().GetSecret())
	if err != nil {
		return nil, err
	}

	ipFirst := utils.GetFirstIPFromHostWithTimeout(
		5*time.Second, p.Domain,
		[]string{ip.String()},
	)

	var reason []string
	if ip.String() != ipFirst {
		reason = append(reason, fmt.Sprintf(
			"dns A for [%v] is %v, tunnel server external ip: %s (ns:%v)",
			p.GetDomain(), ipFirst, ip, ip,
		))
	}

	ipFirst = utils.GetFirstIPFromHostWithTimeout(
		5*time.Second, p.Domain, nil,
	)
	if ip.String() != ipFirst {
		reason = append(reason, fmt.Sprintf(
			"dns A for [%v] is %v, tunnel server external ip: %s (ns:default)",
			p.GetDomain(), ipFirst, ip,
		),
		)
	}

	if len(reason) > 0 {
		return &ypb.VerifyTunnelServerDomainResponse{
			Domain: p.Domain,
			Ok:     false,
			Reason: strings.Join(reason, "\n"),
		}, nil
	}

	return &ypb.VerifyTunnelServerDomainResponse{
		Domain: p.Domain,
		Ok:     true,
		Reason: "",
	}, nil
}

func (s *Server) RequireDNSLogDomain(ctx context.Context, addr *ypb.YakDNSLogBridgeAddr) (*ypb.DNSLogRootDomain, error) {
	domain, token, err := cybertunnel.RequireDNSLogDomain(addr.GetDNSLogAddr())
	if err != nil {
		return nil, err
	}
	return &ypb.DNSLogRootDomain{
		Domain: domain,
		Token:  token,
	}, nil
}

func (s *Server) QueryDNSLogByToken(ctx context.Context, req *ypb.QueryDNSLogByTokenRequest) (*ypb.QueryDNSLogByTokenResponse, error) {
	events, err := cybertunnel.QueryExistedDNSLogEvents(req.GetDNSLogAddr(), req.GetToken())
	if err != nil {
		return nil, err
	}
	rsp := &ypb.QueryDNSLogByTokenResponse{}
	for _, e := range events {
		rsp.Events = append(rsp.Events, &ypb.DNSLogEvent{
			DNSType:    e.Type,
			Token:      e.GetToken(),
			Domain:     e.GetDomain(),
			RemoteAddr: e.RemoteAddr,
			RemoteIP:   e.RemoteIP,
			RemotePort: e.GetRemotePort(),
			Raw:        e.GetRaw(),
			Timestamp:  e.GetTimestamp(),
		})
	}
	return rsp, nil
}

func (s *Server) QueryICMPTrigger(ctx context.Context, req *ypb.QueryICMPTriggerRequest) (*ypb.QueryICMPTriggerResponse, error) {
	notf, err := cybertunnel.QueryICMPLengthTriggerNotifications(
		int(req.Length),
		consts.GetDefaultPublicReverseServer(),
		consts.GetDefaultPublicReverseServerPassword(),
		ctx,
	)
	if err != nil {
		return nil, err
	}
	return &ypb.QueryICMPTriggerResponse{Notification: []*ypb.ICMPTriggerNotification{
		{
			Size:                               notf.Size,
			CurrentRemoteAddr:                  notf.CurrentRemoteAddr,
			Histories:                          notf.Histories,
			CurrentRemoteCachedConnectionCount: notf.CurrentRemoteCachedConnectionCount,
			SizedCachedHistoryConnectionCount:  notf.SizeCachedHistoryConnectionCount,
			TriggerTimestamp:                   notf.TriggerTimestamp,
			Timestamp:                          notf.Timestamp,
		},
	}}, nil
}

func (s *Server) SetYakBridgeLogServer(ctx context.Context, l *ypb.YakDNSLogBridgeAddr) (*ypb.Empty, error) {
	consts.SetDefaultPublicReverseServer(l.GetDNSLogAddr())
	consts.SetDefaultPublicReverseServerPassword(l.GetDNSLogAddrSecret())
	return &ypb.Empty{}, nil
}

func (s *Server) GetCurrentYakBridgeLogServer(ctx context.Context, l *ypb.Empty) (*ypb.YakDNSLogBridgeAddr, error) {
	return &ypb.YakDNSLogBridgeAddr{
		DNSLogAddr:       consts.GetDefaultPublicReverseServer(),
		DNSLogAddrSecret: consts.GetDefaultPublicReverseServerPassword(),
	}, nil
}

func (s *Server) RequireICMPRandomLength(ctx context.Context, req *ypb.Empty) (*ypb.RequireICMPRandomLengthResponse, error) {
	counter := 0
	for {
		counter++
		if counter > 5 {
			return nil, utils.Error("cannot fetch available icmp random length")
		}
		length := 100 + rand.Intn(1100)
		rsp, _ := s.QueryICMPTrigger(ctx, &ypb.QueryICMPTriggerRequest{Length: int32(length)})
		if rsp == nil || len(rsp.Notification) <= 0 {
			host, _, _ := utils.ParseStringToHostPort(consts.GetDefaultPublicReverseServer())
			if host == "" {
				host = consts.GetDefaultPublicReverseServer()
			}
			return &ypb.RequireICMPRandomLengthResponse{Length: int32(length), ExternalHost: host}, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (s *Server) RequireRandomPortToken(ctx context.Context, req *ypb.Empty) (*ypb.RandomPortInfo, error) {
	token := utils.RandStringBytes(8)
	rsp, err := cybertunnel.RequirePortByToken(
		token,
		consts.GetDefaultPublicReverseServer(),
		consts.GetDefaultPublicReverseServerPassword(),
		utils.TimeoutContextSeconds(10),
	)
	if err != nil {
		return nil, err
	}
	return &ypb.RandomPortInfo{
		Token: rsp.Token,
		Addr:  utils.HostPort(rsp.ExternalIP, rsp.Port),
		Port:  int32(rsp.GetPort()),
	}, nil
}

func (s *Server) QueryRandomPortTrigger(ctx context.Context, r *ypb.QueryRandomPortTriggerRequest) (*ypb.RandomPortTriggerNotification, error) {
	event, err := cybertunnel.QueryExistedRandomPortTriggerEvents(
		r.GetToken(),
		consts.GetDefaultPublicReverseServer(),
		consts.GetDefaultPublicReverseServerPassword(),
		ctx,
	)
	if err != nil {
		return nil, err
	}
	return &ypb.RandomPortTriggerNotification{
		RemoteAddr:                            event.RemoteAddr,
		RemoteIP:                              event.RemoteIP,
		RemotePort:                            event.RemotePort,
		LocalPort:                             event.LocalPort,
		History:                               event.History,
		CurrentRemoteCachedConnectionCount:    event.CurrentRemoteCachedConnectionCount,
		LocalPortCachedHistoryConnectionCount: event.LocalPortCachedHistoryConnectionCount,
		TriggerTimestamp:                      event.TriggerTimestamp,
		Timestamp:                             event.Timestamp,
	}, nil
}
