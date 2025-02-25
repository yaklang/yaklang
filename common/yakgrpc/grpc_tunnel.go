package yakgrpc

import (
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cybertunnel"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"math/rand"
	"strings"
	"time"
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
	ipFirst := netx.LookupFirst(p.Domain, netx.WithTimeout(5*time.Second), netx.WithDNSServers(ip.String()))
	var reason []string
	if ip.String() != ipFirst {
		reason = append(reason, fmt.Sprintf(
			"dns A for [%v] is %v, tunnel server external ip: %s (ns:%v)",
			p.GetDomain(), ipFirst, ip, ip,
		))
	}

	ipFirst = netx.LookupFirst(p.Domain, netx.WithTimeout(5*time.Second))
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

func (s *Server) RequireDNSLogDomain(ctx context.Context, params *ypb.YakDNSLogBridgeAddr) (*ypb.DNSLogRootDomain, error) {
	if params.GetUseLocal() {
		domain, token, _, err := cybertunnel.RequireDNSLogDomainByLocal(params.GetDNSMode())
		if err != nil {
			return nil, err
		}
		return &ypb.DNSLogRootDomain{
			Domain: domain,
			Token:  token,
		}, nil
	} else {
		domain, token, _, err := cybertunnel.RequireDNSLogDomainByRemote(params.GetDNSLogAddr(), params.GetDNSMode())
		if err != nil {
			return nil, err
		}
		return &ypb.DNSLogRootDomain{
			Domain: domain,
			Token:  token,
		}, nil
	}
}

func (s *Server) RequireDNSLogDomainByScript(ctx context.Context, req *ypb.RequireDNSLogDomainByScriptRequest) (*ypb.DNSLogRootDomain, error) {
	if req.GetScriptName() != "" {
		script, err := yakit.GetYakScriptByName(s.GetProfileDatabase(), req.GetScriptName())
		if err != nil {
			return nil, err
		}

		engine, err := yak.NewScriptEngine(1000).ExecuteEx(script.Content, map[string]interface{}{
			"YAK_FILENAME": req.GetScriptName(),
		})
		if err != nil {
			return nil, utils.Errorf("execute file %s code failed: %s", req.GetScriptName(), err.Error())
		}
		result, err := engine.CallYakFunction(context.Background(), "requireDomain", []interface{}{})
		if err != nil {
			return nil, utils.Errorf("import %v' s handle failed: %s", req.GetScriptName(), err)
		}
		var domain, token string
		domain = utils.InterfaceToStringSlice(result)[0]
		token = utils.InterfaceToStringSlice(result)[1]
		return &ypb.DNSLogRootDomain{
			Domain: domain,
			Token:  token,
		}, nil

	} else {
		return nil, utils.Error("script name is empty")
	}
}

func (s *Server) QuerySupportedDnsLogPlatforms(ctx context.Context, req *ypb.Empty) (*ypb.QuerySupportedDnsLogPlatformsResponse, error) {
	platforms := cybertunnel.GetSupportDNSLogBrokersName()
	if len(platforms) > 0 {
		return &ypb.QuerySupportedDnsLogPlatformsResponse{
			Platforms: platforms,
		}, nil
	}
	return nil, fmt.Errorf("no supported dns log platforms")
}

func (s *Server) QueryDNSLogByToken(ctx context.Context, req *ypb.QueryDNSLogByTokenRequest) (*ypb.QueryDNSLogByTokenResponse, error) {
	var events []*tpb.DNSLogEvent
	var err error
	if req.GetUseLocal() {
		events, err = cybertunnel.QueryExistedDNSLogEventsByLocal(req.GetToken(), req.GetDNSMode())
	} else {
		events, err = cybertunnel.QueryExistedDNSLogEvents(req.GetDNSLogAddr(), req.GetToken(), req.GetDNSMode())
	}
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

func (s *Server) QueryDNSLogTokenByScript(ctx context.Context, req *ypb.RequireDNSLogDomainByScriptRequest) (*ypb.QueryDNSLogByTokenResponse, error) {
	var events []*tpb.DNSLogEvent
	if req.GetScriptName() != "" {
		script, err := yakit.GetYakScriptByName(s.GetProfileDatabase(), req.GetScriptName())
		if err != nil {
			return nil, err
		}

		engine, err := yak.NewScriptEngine(1000).ExecuteEx(script.Content, map[string]interface{}{
			"YAK_FILENAME": req.GetScriptName(),
		})
		if err != nil {
			return nil, utils.Errorf("execute file %s code failed: %s", req.GetScriptName(), err.Error())
		}
		result, err := engine.CallYakFunction(context.Background(), "getResults", []interface{}{req.GetToken()})
		if err != nil {
			return nil, utils.Errorf("import %v' s handle failed: %s", req.GetScriptName(), err)
		}
		for _, v := range utils.InterfaceToSliceInterface(result) {
			event := utils.InterfaceToMapInterface(v)
			var raw = []byte(spew.Sdump(event))

			e := &tpb.DNSLogEvent{
				Type:       utils.MapGetString(event, "Type"),
				Token:      utils.MapGetString(event, "Token"),
				Domain:     utils.MapGetString(event, "Domain"),
				RemoteAddr: utils.MapGetString(event, "RemoteAddr"),
				RemoteIP:   utils.MapGetString(event, "RemoteIP"),
				Raw:        raw,
				Timestamp:  int64(utils.MapGetInt(event, "Timestamp")),
			}
			events = append(events, e)
		}
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
