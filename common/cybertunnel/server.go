package cybertunnel

import (
	"context"
	"yaklang/common/cybertunnel/tpb"
	"yaklang/common/log"
	"yaklang/common/utils"
	"sync"
	"time"
)

var startTunnelServerOnceForRandomPort = new(sync.Once)
var startTunnelServerOnceForICMPLength = new(sync.Once)
var randomPortTrigger *RandomPortTrigger
var icmpTrigger *ICMPTrigger

type TunnelServer struct {
	tpb.TunnelServer

	ExternalIP string

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

	return nil
}

func NewTunnelServer() (*TunnelServer, error) {
	s := &TunnelServer{}
	return s, nil
}
