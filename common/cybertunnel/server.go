package cybertunnel

import (
	"context"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"sync"
	"time"
)

var startTunnelServerOnceForRandomPort = new(sync.Once)
var startTunnelServerOnceForICMPLength = new(sync.Once)
var startTunnelServerOnceForHTTPTrigger = new(sync.Once)
var randomPortTrigger *RandomPortTrigger
var icmpTrigger *ICMPTrigger

type TunnelServer struct {
	tpb.TunnelServer

	ExternalIP   string
	DNSLogDomain []string

	// 二级密码是用来分别
	SecondaryPassword string
}

func (s *TunnelServer) RequireDomain(ctx context.Context, params *tpb.RequireDomainParams) (*tpb.RequireDomainResponse, error) {
	panic("implement me")
}

func (s *TunnelServer) QueryExistedDNSLog(ctx context.Context, params *tpb.QueryExistedDNSLogParams) (*tpb.QueryExistedDNSLogResponse, error) {
	panic("implement me")
}

func (s *TunnelServer) InitialReverseTrigger() error {
	if randomPortTrigger != nil {
		return utils.Errorf("tunnel-server port-trigger started at mem:%p", randomPortTrigger)
	}

	var err error
	randomPortTrigger, err = NewRandomPortTrigger()
	if err != nil {
		return utils.Errorf("create random port trigger failed: %s", err)
	}
	go startTunnelServerOnceForRandomPort.Do(func() {
		for {
			if randomPortTrigger != nil {
				err := randomPortTrigger.Run()
				if err != nil {
					log.Errorf("start port trigger failed: %s", err)
				}
			} else {
				log.Error("port trigger empty")
			}
			time.Sleep(1 * time.Second)
		}
	})

	icmpTrigger, err = NewICMPTrigger()
	if err != nil {
		return utils.Errorf("create icmp length trigger failed: %s", err)
	}
	go startTunnelServerOnceForICMPLength.Do(func() {
		for {
			if icmpTrigger != nil {
				err := icmpTrigger.Run()
				if err != nil {
					log.Errorf("start icmp trigger failed: %s", err)
				} else {
					log.Errorf("icmp length trigger empty")
				}
				time.Sleep(time.Second * 1)
			}
		}
	})

	defaultHTTPTrigger, err = NewHTTPTrigger(s.ExternalIP, s.DNSLogDomain...)
	if err != nil {
		return utils.Errorf("create http trigger failed: %s", err)
	}
	go startTunnelServerOnceForHTTPTrigger.Do(func() {
		for {
			if defaultHTTPTrigger != nil {
				err := defaultHTTPTrigger.Serve()
				if err != nil {
					log.Errorf("start http trigger failed: %s", err)
				} else {
					log.Errorf("http trigger empty")
				}
				time.Sleep(time.Second * 1)
			}
		}
	})
	return nil
}

func NewTunnelServer(dnslogDomain, externalIPConfigged string) (*TunnelServer, error) {
	domainsRaw := strings.Trim(strings.TrimSpace(strings.ToLower(dnslogDomain)), ".")
	var domains []string
	if strings.Contains(domainsRaw, ",") {
		domains = strings.Split(domainsRaw, ",")
	} else {
		domains = []string{domainsRaw}
	}
	domains = lo.Map(domains, func(item string, index int) string {
		return strings.TrimSpace(item)
	})
	s := &TunnelServer{
		ExternalIP:   externalIPConfigged,
		DNSLogDomain: domains,
	}
	if s.ExternalIP == "" {
		i, err := GetExternalIP()
		if err != nil {
			return nil, err
		}
		s.ExternalIP = i.String()
	}
	return s, nil
}
