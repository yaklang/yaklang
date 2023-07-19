package dnslogbrokers

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"sync"
	"time"
)

var brokers = new(sync.Map)

func register(name string, i DNSLogBroker) {
	_, ok := brokers.Load(name)
	if ok {
		log.Errorf("broker: %v is existed", name)
		return
	}
	brokers.Store(name, i)
}

func Get(name string) (DNSLogBroker, error) {
	raw, ok := brokers.Load(name)
	if !ok {
		return nil, utils.Errorf("dnsbroker [%v] no existed", name)
	}
	ins, ok := raw.(DNSLogBroker)
	if !ok {
		spew.Dump(raw)
		return nil, utils.Errorf("BUG: dnsbroker internal error, not DNSLogBroker Interface")
	}
	return ins, nil
}

type DNSLogBroker interface {
	Require(timeout time.Duration, proxy ...string) (domain, token string, err error)
	GetResult() ([]*tpb.DNSLogEvent, error)
}
